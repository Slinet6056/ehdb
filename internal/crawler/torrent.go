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

// TorrentCrawler crawls torrents from E-Hentai torrent list page
type TorrentCrawler struct {
	client     *Client
	cfg        *config.CrawlerConfig
	logger     *zap.Logger
	maxPages   int
	statusCode string
	search     string
	retryTimes int
}

// TorrentCrawlerOptions contains optional parameters for torrent crawler
type TorrentCrawlerOptions struct {
	MaxPages   int
	StatusCode string
	Search     string
}

// NewTorrentCrawler creates a new torrent crawler
func NewTorrentCrawler(cfg *config.CrawlerConfig, logger *zap.Logger) (*TorrentCrawler, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &TorrentCrawler{
		client:     client,
		cfg:        cfg,
		logger:     logger,
		maxPages:   0, // 0 means no limit
		statusCode: "",
		search:     "",
		retryTimes: cfg.RetryTimes,
	}, nil
}

// SetOptions sets optional parameters for the crawler
func (c *TorrentCrawler) SetOptions(opts TorrentCrawlerOptions) {
	c.maxPages = opts.MaxPages
	c.statusCode = opts.StatusCode
	c.search = opts.Search
}

// TorrentListItem represents a torrent item from the list page
type TorrentListItem struct {
	Gid   int
	Token string
	Gtid  int
}

// Sync synchronizes torrents from E-Hentai torrent list page
func (c *TorrentCrawler) Sync(ctx context.Context) error {
	c.logger.Info("starting torrent sync")

	// Get last torrent ID
	lastTorrentID, err := c.getLastTorrentID(ctx)
	if err != nil {
		return fmt.Errorf("get last torrent id: %w", err)
	}

	if lastTorrentID > 0 {
		c.logger.Info("got last torrent id", zap.Int("id", lastTorrentID))
	} else {
		c.logger.Info("no existing torrents, will fetch all")
	}

	// Get existing torrent IDs for deduplication
	c.logger.Debug("loading existing torrent ids")
	existingIDs, err := c.getExistingTorrentIDs(ctx)
	if err != nil {
		return fmt.Errorf("get existing torrent ids: %w", err)
	}
	existingIDMap := make(map[int]bool)
	for _, id := range existingIDs {
		existingIDMap[id] = true
	}

	// Fetch torrent list pages
	var items []TorrentListItem
	page := 0
	finished := false

	for !finished {
		c.logger.Debug("fetching torrent list page", zap.Int("page", page))

		pageItems, err := Retry(RetryConfig{
			MaxRetries: c.retryTimes,
			Logger:     c.logger,
		}, func() ([]TorrentListItem, error) {
			return c.fetchTorrentListPage(page)
		})
		if err != nil {
			return fmt.Errorf("fetch page %d: %w", page, err)
		}

		if len(pageItems) == 0 {
			break
		}

		c.logger.Debug("got torrents from page",
			zap.Int("page", page),
			zap.Int("from_id", pageItems[0].Gtid),
			zap.Int("to_id", pageItems[len(pageItems)-1].Gtid),
			zap.Int("count", len(pageItems)),
		)

		// Filter items
		for _, item := range pageItems {
			// Check if we should stop
			if c.maxPages == 0 && item.Gtid <= lastTorrentID {
				finished = true
				break
			}

			// Skip if already exists
			if !existingIDMap[item.Gtid] {
				items = append(items, item)
			}
		}

		page++
		if c.maxPages > 0 && page >= c.maxPages {
			break
		}

		// Rate limiting
		time.Sleep(1 * time.Second)
	}

	if len(items) == 0 {
		c.logger.Info("no new torrents available")
		return nil
	}

	c.logger.Info("found new torrents", zap.Int("count", len(items)))

	// Group by gallery
	gidMap := make(map[int][]TorrentListItem)
	for _, item := range items {
		gidMap[item.Gid] = append(gidMap[item.Gid], item)
	}

	// Check which galleries exist
	gids := make([]int, 0, len(gidMap))
	for gid := range gidMap {
		gids = append(gids, gid)
	}

	c.logger.Debug("checking existing galleries", zap.Int("count", len(gids)))
	existingGids, err := c.getExistingGalleryIDs(ctx, gids)
	if err != nil {
		return fmt.Errorf("get existing galleries: %w", err)
	}

	existingGidMap := make(map[int]bool)
	for _, gid := range existingGids {
		existingGidMap[gid] = true
	}

	// Find galleries that don't exist
	var missingGids []int
	for _, gid := range gids {
		if !existingGidMap[gid] {
			missingGids = append(missingGids, gid)
		}
	}

	// Import missing galleries
	if len(missingGids) > 0 {
		c.logger.Info("importing missing galleries", zap.Int("count", len(missingGids)))

		if err := c.importMissingGalleries(ctx, items, missingGids); err != nil {
			return fmt.Errorf("import missing galleries: %w", err)
		}

		// Mark as bytorrent
		if err := c.markGalleriesByTorrent(ctx, gids); err != nil {
			c.logger.Warn("failed to mark galleries as bytorrent", zap.Error(err))
		}
	}

	// Process all torrents
	c.logger.Info("processing torrents", zap.Int("galleries", len(gidMap)))
	processed := 0
	newTorrents := 0

	for gid := range gidMap {
		token := gidMap[gid][0].Token

		count, err := Retry(RetryConfig{
			MaxRetries: c.retryTimes,
			Logger:     c.logger,
		}, func() (int, error) {
			return c.processTorrentsForGallery(ctx, gid, token)
		})
		if err != nil {
			c.logger.Error("failed to process gallery torrents", zap.Int("gid", gid), zap.Error(err))
			continue
		}

		processed++
		newTorrents += count

		// Rate limiting
		time.Sleep(1 * time.Second)
	}

	c.logger.Info("torrent sync completed",
		zap.Int("processed", processed),
		zap.Int("new_torrents", newTorrents),
	)
	return nil
}

// fetchTorrentListPage fetches a single page from torrents.php
func (c *TorrentCrawler) fetchTorrentListPage(page int) ([]TorrentListItem, error) {
	params := []string{}
	if c.search != "" {
		params = append(params, fmt.Sprintf("search=%s", c.search))
	}
	if c.statusCode != "" {
		params = append(params, fmt.Sprintf("s=%s", c.statusCode))
	}
	if page > 0 {
		params = append(params, fmt.Sprintf("page=%d", page))
	}

	path := "/torrents.php"
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	url := fmt.Sprintf("https://%s%s", c.cfg.Host, path)
	body, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}

	// Parse torrent items
	firstPattern := regexp.MustCompile(`gallerytorrents\.php\?gid=\d+&(?:amp;)?t=[0-9a-f]{10}&(?:amp;)?gtid=\d+"`)
	firstMatches := firstPattern.FindAll(body, -1)
	secondPattern := regexp.MustCompile(`gallerytorrents\.php\?gid=(\d+)&(?:amp;)?t=([0-9a-f]{10})&(?:amp;)?gtid=(\d+)"`)

	var items []TorrentListItem
	for _, entry := range firstMatches {
		match := secondPattern.FindSubmatch(entry)
		if len(match) >= 4 {
			gid, _ := strconv.Atoi(string(match[1]))
			token := string(match[2])
			gtid, _ := strconv.Atoi(string(match[3]))

			items = append(items, TorrentListItem{
				Gid:   gid,
				Token: token,
				Gtid:  gtid,
			})
		}
	}

	return items, nil
}

// processTorrentsForGallery processes torrents for a single gallery, returns count of new torrents
func (c *TorrentCrawler) processTorrentsForGallery(ctx context.Context, gid int, token string) (int, error) {
	c.logger.Debug("processing gallery torrents", zap.Int("gid", gid))

	url := fmt.Sprintf("https://%s/gallerytorrents.php?gid=%d&t=%s", c.cfg.Host, gid, token)

	body, err := c.client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("fetch torrent page: %w", err)
	}

	bodyStr := string(body)

	// Check for special cases
	if strings.Contains(bodyStr, "This gallery is currently unavailable") {
		c.logger.Debug("gallery unavailable", zap.Int("gid", gid))
		return 0, nil
	}

	if strings.Contains(bodyStr, "Gallery not found") {
		c.logger.Debug("gallery not found (pending refresh)", zap.Int("gid", gid))
		return 0, nil
	}

	// Parse root gid from announce URL
	announcePattern := regexp.MustCompile(`/(\d+)/announce`)
	announceMatches := announcePattern.FindStringSubmatch(bodyStr)
	if len(announceMatches) < 2 {
		c.logger.Debug("no torrents found", zap.Int("gid", gid))
		return 0, nil
	}

	rootGid, _ := strconv.Atoi(announceMatches[1])

	// Parse torrent information (only non-expunged for sync)
	torrents := c.parseTorrents(body, rootGid)

	newCount := 0
	if len(torrents) > 0 {
		// Get existing torrents
		existingHashes, err := c.getExistingTorrentHashes(ctx, rootGid)
		if err != nil {
			return 0, fmt.Errorf("get existing torrents: %w", err)
		}

		// Filter new torrents
		var newTorrents []database.Torrent
		for _, t := range torrents {
			if t.Hash != nil && !containsString(existingHashes, *t.Hash) {
				newTorrents = append(newTorrents, t)
			}
		}

		// Save new torrents
		if len(newTorrents) > 0 {
			if err := c.saveTorrents(ctx, newTorrents); err != nil {
				return 0, fmt.Errorf("save torrents: %w", err)
			}
			newCount = len(newTorrents)
			c.logger.Info("saved new torrents", zap.Int("gid", gid), zap.Int("root_gid", rootGid), zap.Int("count", newCount))
		}
	}

	// Update root_gid
	if err := c.updateRootGid(ctx, gid, rootGid); err != nil {
		c.logger.Warn("failed to update root_gid", zap.Int("gid", gid), zap.Int("root_gid", rootGid), zap.Error(err))
	}

	if rootGid != gid {
		c.logger.Debug("gallery replaced", zap.Int("gid", gid), zap.Int("root_gid", rootGid))
	}

	return newCount, nil
}

// parseTorrents parses torrent information from HTML (only non-expunged)
func (c *TorrentCrawler) parseTorrents(html []byte, gid int) []database.Torrent {
	var torrents []database.Torrent

	// Pattern matches only non-expunged torrents
	pattern := regexp.MustCompile(`name="gtid"\svalue="(\d+?)"[\s\S]*?Posted:<.*?(\d{4}-\d{2}-\d{2} \d{2}:\d{2})<\/[\s\S]*?Size:.*>\s?([\d.KMGTiB ]+)<\/[\s\S]*?Uploader:.*?([\S]+)<\/[\s\S]*?([0-9a-f]{40})\.torrent.*?>(.*?)<\/a><\/td>`)

	matches := pattern.FindAllSubmatch(html, -1)

	for _, match := range matches {
		if len(match) < 7 {
			continue
		}

		gtidStr := string(match[1])
		posted := string(match[2])
		size := string(match[3])
		uploader := string(match[4])
		hashStr := string(match[5])
		name := string(match[6])

		gtid, _ := strconv.Atoi(gtidStr)

		torrents = append(torrents, database.Torrent{
			ID:       gtid,
			Gid:      gid,
			Name:     strings.TrimSpace(name),
			Hash:     &hashStr,
			Addedstr: &posted,
			Fsizestr: &size,
			Uploader: uploader,
			Expunged: false,
		})
	}

	return torrents
}

// importMissingGalleries imports galleries that don't exist in database
func (c *TorrentCrawler) importMissingGalleries(ctx context.Context, items []TorrentListItem, missingGids []int) error {
	// Build gidlist for missing galleries
	var gidlist [][2]interface{}
	gidTokenMap := make(map[int]string)

	for _, item := range items {
		for _, gid := range missingGids {
			if item.Gid == gid {
				if _, exists := gidTokenMap[gid]; !exists {
					gidTokenMap[gid] = item.Token
					gidlist = append(gidlist, [2]interface{}{gid, item.Token})
				}
				break
			}
		}
	}

	// Fetch metadata in batches
	var allMetadata []database.GalleryMetadata
	for i := 0; i < len(gidlist); i += 25 {
		end := i + 25
		if end > len(gidlist) {
			end = len(gidlist)
		}

		batch := gidlist[i:end]
		c.logger.Debug("fetching metadata batch", zap.Int("from", i), zap.Int("to", end))

		metadata, err := Retry(RetryConfig{
			MaxRetries: c.retryTimes,
			Logger:     c.logger,
		}, func() ([]database.GalleryMetadata, error) {
			return c.GetMetadatas(batch)
		})

		if err != nil {
			c.logger.Error("failed to fetch metadata batch", zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata...)

		// Rate limiting
		time.Sleep(1 * time.Second)
	}

	// Import galleries
	if len(allMetadata) > 0 {
		c.logger.Debug("importing metadata", zap.Int("count", len(allMetadata)))
		importer := NewImporter(c.logger)
		if err := importer.Import(ctx, allMetadata, false); err != nil {
			return fmt.Errorf("import metadata: %w", err)
		}
	}

	return nil
}

// GetMetadatas fetches metadata from E-Hentai API
func (c *TorrentCrawler) GetMetadatas(gidlist [][2]interface{}) ([]database.GalleryMetadata, error) {
	// Reuse GalleryCrawler's GetMetadatas logic
	gc := &GalleryCrawler{
		client: c.client,
		cfg:    c.cfg,
		logger: c.logger,
	}
	return gc.GetMetadatas(gidlist)
}

// Database helper functions

func (c *TorrentCrawler) getLastTorrentID(ctx context.Context) (int, error) {
	pool := database.GetPool()
	query := `SELECT id FROM torrent ORDER BY id DESC LIMIT 1`

	c.logger.Debug("executing query", zap.String("sql", utils.FormatSQL(query)))

	var id int
	err := pool.QueryRow(ctx, query).Scan(&id)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return 0, nil
		}
		return 0, err
	}

	return id, nil
}

func (c *TorrentCrawler) getExistingTorrentIDs(ctx context.Context) ([]int, error) {
	pool := database.GetPool()
	query := `SELECT id FROM torrent`

	c.logger.Debug("executing query", zap.String("sql", utils.FormatSQL(query)))

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func (c *TorrentCrawler) getExistingGalleryIDs(ctx context.Context, gids []int) ([]int, error) {
	if len(gids) == 0 {
		return []int{}, nil
	}

	pool := database.GetPool()
	query := `SELECT gid FROM gallery WHERE gid = ANY($1)`

	c.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query)),
		zap.Int("count", len(gids)),
	)

	rows, err := pool.Query(ctx, query, gids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var existingGids []int
	for rows.Next() {
		var gid int
		if err := rows.Scan(&gid); err != nil {
			continue
		}
		existingGids = append(existingGids, gid)
	}

	return existingGids, nil
}

func (c *TorrentCrawler) getExistingTorrentHashes(ctx context.Context, gid int) ([]string, error) {
	pool := database.GetPool()
	query := `SELECT hash FROM torrent WHERE gid = $1 AND hash IS NOT NULL`

	c.logger.Debug("executing query",
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

func (c *TorrentCrawler) saveTorrents(ctx context.Context, torrents []database.Torrent) error {
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
		c.logger.Debug("executing upsert query",
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

func (c *TorrentCrawler) updateRootGid(ctx context.Context, gid int, rootGid int) error {
	pool := database.GetPool()
	query := `UPDATE gallery SET root_gid = $1 WHERE gid = $2`

	c.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query, rootGid, gid)),
	)

	_, err := pool.Exec(ctx, query, rootGid, gid)
	if err != nil {
		return fmt.Errorf("update root_gid: %w", err)
	}

	return nil
}

func (c *TorrentCrawler) markGalleriesByTorrent(ctx context.Context, gids []int) error {
	if len(gids) == 0 {
		return nil
	}

	pool := database.GetPool()
	query := `UPDATE gallery SET bytorrent = true WHERE gid = ANY($1)`

	c.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query)),
		zap.Int("count", len(gids)),
	)

	_, err := pool.Exec(ctx, query, gids)
	if err != nil {
		return fmt.Errorf("mark galleries bytorrent: %w", err)
	}

	return nil
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
