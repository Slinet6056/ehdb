package crawler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

// TorrentImporter imports torrents from all galleries
type TorrentImporter struct {
	client     *Client
	cfg        *config.CrawlerConfig
	logger     *zap.Logger
	retryTimes int
}

// NewTorrentImporter creates a new torrent importer
func NewTorrentImporter(cfg *config.CrawlerConfig, logger *zap.Logger) (*TorrentImporter, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &TorrentImporter{
		client:     client,
		cfg:        cfg,
		logger:     logger,
		retryTimes: cfg.RetryTimes,
	}, nil
}

// ImportAll imports torrents from all galleries (heavy operation)
func (ti *TorrentImporter) ImportAll(ctx context.Context) error {
	ti.logger.Warn("starting torrent import - this may take a long time")

	pool := database.GetPool()

	// Get all galleries without root_gid and not removed
	query := `
		SELECT gid, token, posted
		FROM gallery
		WHERE root_gid IS NULL AND removed = false
		ORDER BY gid ASC
	`

	ti.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query)),
	)

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("query galleries: %w", err)
	}
	defer rows.Close()

	var galleries []struct {
		Gid    int
		Token  string
		Posted time.Time
	}

	for rows.Next() {
		var g struct {
			Gid    int
			Token  string
			Posted time.Time
		}
		if err := rows.Scan(&g.Gid, &g.Token, &g.Posted); err != nil {
			ti.logger.Warn("failed to scan gallery", zap.Error(err))
			continue
		}
		galleries = append(galleries, g)
	}

	ti.logger.Info("found galleries to process", zap.Int("count", len(galleries)))

	// Process each gallery
	processed := 0
	succeeded := 0
	newTorrents := 0

	for _, g := range galleries {
		count, err := Retry(RetryConfig{
			MaxRetries:     ti.retryTimes,
			Logger:         ti.logger,
			WaitForIPUnban: ti.cfg.WaitForIPUnban,
		}, func() (int, error) {
			return ti.processGallery(ctx, g.Gid, g.Token, g.Posted)
		})
		if err != nil {
			ti.logger.Error("failed to process gallery", zap.Int("gid", g.Gid), zap.Error(err))
		} else {
			succeeded++
			newTorrents += count
		}

		processed++
		if processed%100 == 0 {
			ti.logger.Info("progress",
				zap.Int("processed", processed),
				zap.Int("succeeded", succeeded),
				zap.Int("new_torrents", newTorrents),
				zap.Int("total", len(galleries)),
			)
		}

		// Rate limiting
		time.Sleep(2 * time.Second)
	}

	ti.logger.Info("torrent import completed",
		zap.Int("processed", processed),
		zap.Int("succeeded", succeeded),
		zap.Int("new_torrents", newTorrents),
	)
	return nil
}

// processGallery processes a single gallery, returns count of new torrents
func (ti *TorrentImporter) processGallery(ctx context.Context, gid int, token string, posted time.Time) (int, error) {
	ti.logger.Debug("processing gallery", zap.Int("gid", gid))

	// Fetch torrent page directly
	url := fmt.Sprintf("https://%s/gallerytorrents.php?gid=%d&t=%s", ti.cfg.Host, gid, token)

	body, err := ti.client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("fetch torrent page: %w", err)
	}

	// Check for special cases
	bodyStr := string(body)

	// Check if gallery is removed
	if strings.Contains(bodyStr, "This gallery is currently unavailable") {
		ti.logger.Debug("gallery unavailable, marking as removed", zap.Int("gid", gid))
		return 0, ti.markGalleryRemoved(ctx, gid)
	}

	// Check if gallery not found (might be pending if posted within a week)
	if strings.Contains(bodyStr, "Gallery not found") {
		oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)
		if posted.Before(oneWeekAgo) {
			ti.logger.Debug("gallery not found and old, marking as removed", zap.Int("gid", gid))
			return 0, ti.markGalleryRemoved(ctx, gid)
		} else {
			ti.logger.Debug("gallery not found but recent (pending refresh)", zap.Int("gid", gid))
			return 0, fmt.Errorf("gallery pending for cache refresh")
		}
	}

	// Parse root gid from announce URL
	announcePattern := regexp.MustCompile(`/(\d+)/announce`)
	announceMatches := announcePattern.FindStringSubmatch(bodyStr)
	if len(announceMatches) < 2 {
		// No torrents found, but set root_gid to itself
		ti.logger.Debug("no torrents found", zap.Int("gid", gid))
		return 0, ti.updateRootGid(ctx, gid, gid)
	}

	rootGid, _ := strconv.Atoi(announceMatches[1])

	// Parse torrent information
	torrents := ti.parseTorrents(body, rootGid)

	newCount := 0
	if len(torrents) > 0 {
		// Get existing torrents
		existingHashes, err := ti.getExistingTorrentHashes(ctx, rootGid)
		if err != nil {
			return 0, fmt.Errorf("get existing torrents: %w", err)
		}

		// Filter new torrents
		var newTorrents []database.Torrent
		for _, t := range torrents {
			if t.Hash != nil && !contains(existingHashes, *t.Hash) {
				newTorrents = append(newTorrents, t)
			}
		}

		// Save new torrents
		if len(newTorrents) > 0 {
			if err := ti.saveTorrents(ctx, newTorrents); err != nil {
				return 0, fmt.Errorf("save torrents: %w", err)
			}
			newCount = len(newTorrents)
			ti.logger.Debug("saved new torrents", zap.Int("gid", gid), zap.Int("root_gid", rootGid), zap.Int("count", newCount))
		}
	}

	// Mark other galleries with same root_gid as replaced
	if rootGid != 0 {
		if err := ti.markReplacedGalleries(ctx, rootGid); err != nil {
			ti.logger.Warn("failed to mark replaced galleries", zap.Int("root_gid", rootGid), zap.Error(err))
		}
	}

	// Update root_gid
	if err := ti.updateRootGid(ctx, gid, rootGid); err != nil {
		return newCount, fmt.Errorf("update root_gid: %w", err)
	}

	if rootGid != gid {
		ti.logger.Debug("gallery replaced", zap.Int("gid", gid), zap.Int("root_gid", rootGid))
	}

	return newCount, nil
}

// parseTorrents parses torrent information from HTML
func (ti *TorrentImporter) parseTorrents(html []byte, gid int) []database.Torrent {
	var torrents []database.Torrent

	// Pattern matches both normal and expunged torrents
	pattern := regexp.MustCompile(`name="gtid"\svalue="(\d+?)"[\s\S]*?Posted:<.*?(\d{4}-\d{2}-\d{2} \d{2}:\d{2})<\/[\s\S]*?Size:.*>\s?([\d.KMGTiB ]+)<\/[\s\S]*?Uploader:.*?([\S]+)<\/[\s\S]*?(?:([0-9a-f]{40})\.torrent|(value="Expunged"))[\s\S]*?>(?:\s*?&nbsp;\s*?)?(.*?)(?:<\/a>)?<\/td>\s*?<\/tr>\s*?<\/table>`)

	matches := pattern.FindAllSubmatch(html, -1)

	for _, match := range matches {
		if len(match) < 8 {
			continue
		}

		gtidStr := string(match[1])
		posted := string(match[2])
		size := string(match[3])
		uploader := string(match[4])
		hashStr := string(match[5])
		expungedStr := string(match[6])
		name := string(match[7])

		gtid, _ := strconv.Atoi(gtidStr)
		expunged := expungedStr != ""

		var hash *string
		if hashStr != "" {
			hash = &hashStr
		}

		torrents = append(torrents, database.Torrent{
			ID:       gtid,
			Gid:      gid,
			Name:     strings.TrimSpace(name),
			Hash:     hash,
			Addedstr: &posted,
			Fsizestr: &size,
			Uploader: uploader,
			Expunged: expunged,
		})
	}

	return torrents
}

// getExistingTorrentHashes gets existing torrent hashes for a gallery
func (ti *TorrentImporter) getExistingTorrentHashes(ctx context.Context, gid int) ([]string, error) {
	pool := database.GetPool()
	query := `SELECT hash FROM torrent WHERE gid = $1 AND hash IS NOT NULL`

	ti.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query, gid)),
	)

	rows, err := pool.Query(ctx, query, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
		}
		hashes = append(hashes, hash)
	}

	return hashes, nil
}

// saveTorrents saves torrents to database
func (ti *TorrentImporter) saveTorrents(ctx context.Context, torrents []database.Torrent) error {
	pool := database.GetPool()

	query := `
		INSERT INTO torrent (id, gid, name, hash, addedstr, fsizestr, uploader, expunged)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id, gid) DO UPDATE SET
			name = EXCLUDED.name,
			hash = EXCLUDED.hash,
			addedstr = EXCLUDED.addedstr,
			fsizestr = EXCLUDED.fsizestr,
			uploader = EXCLUDED.uploader,
			expunged = EXCLUDED.expunged
	`

	for _, t := range torrents {
		ti.logger.Debug("executing upsert query",
			zap.String("sql", utils.FormatSQL(query,
				t.ID, t.Gid, t.Name, t.Hash, t.Addedstr, t.Fsizestr, t.Uploader, t.Expunged,
			)),
		)

		_, err := pool.Exec(ctx, query,
			t.ID, t.Gid, t.Name, t.Hash, t.Addedstr, t.Fsizestr, t.Uploader, t.Expunged,
		)
		if err != nil {
			return fmt.Errorf("insert torrent %d: %w", t.ID, err)
		}
	}

	return nil
}

// markReplacedGalleries marks galleries with the same root_gid as replaced
func (ti *TorrentImporter) markReplacedGalleries(ctx context.Context, rootGid int) error {
	pool := database.GetPool()
	query := `UPDATE gallery SET replaced = true WHERE root_gid = $1`

	ti.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query, rootGid)),
	)

	_, err := pool.Exec(ctx, query, rootGid)
	if err != nil {
		return fmt.Errorf("mark replaced galleries: %w", err)
	}

	return nil
}

// updateRootGid updates the root_gid for a gallery
func (ti *TorrentImporter) updateRootGid(ctx context.Context, gid int, rootGid int) error {
	pool := database.GetPool()
	query := `UPDATE gallery SET root_gid = $1 WHERE gid = $2`

	ti.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query, rootGid, gid)),
	)

	_, err := pool.Exec(ctx, query, rootGid, gid)
	if err != nil {
		return fmt.Errorf("update root_gid: %w", err)
	}

	return nil
}

// markGalleryRemoved marks a gallery as removed
func (ti *TorrentImporter) markGalleryRemoved(ctx context.Context, gid int) error {
	pool := database.GetPool()
	query := `UPDATE gallery SET removed = true WHERE gid = $1`

	ti.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query, gid)),
	)

	_, err := pool.Exec(ctx, query, gid)
	if err != nil {
		return fmt.Errorf("mark gallery removed: %w", err)
	}

	return nil
}

// contains checks if a string slice contains a value
func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
