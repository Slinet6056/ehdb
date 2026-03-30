package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

// GalleryCrawler crawls galleries from E-Hentai
type GalleryCrawler struct {
	client     *Client
	cfg        *config.CrawlerConfig
	logger     *zap.Logger
	retryTimes int
}

// NewGalleryCrawler creates a new gallery crawler
func NewGalleryCrawler(cfg *config.CrawlerConfig, logger *zap.Logger) (*GalleryCrawler, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &GalleryCrawler{
		client:     client,
		cfg:        cfg,
		logger:     logger,
		retryTimes: cfg.RetryTimes,
	}, nil
}

// GalleryListItem represents a gallery item from the list page
type GalleryListItem struct {
	Gid    string
	Token  string
	Posted string
}

const consecutiveOldPagesLimit = 3

// GetPages fetches a page of galleries
func (c *GalleryCrawler) GetPages(next string, expunged bool) ([]GalleryListItem, error) {
	url := fmt.Sprintf("https://%s/?next=%s&f_cats=0&advsearch=1&f_sname=on&f_stags=on", c.cfg.Host, next)

	if expunged {
		url += "&f_sh=on"
	}
	url += "&f_spf=&f_spt=&f_sfl=on&f_sfu=on&f_sft=on"

	body, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}

	// Parse gallery list from HTML
	firstPattern := regexp.MustCompile(`gid=\d+&amp;t=[0-9a-f]{10}&.*?posted_.*?>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}<`)
	firstMatches := firstPattern.FindAll(body, -1)
	secondPattern := regexp.MustCompile(`gid=(\d+).*?t=([0-9a-f]{10}).*?>(\d{4}-\d{2}-\d{2}\s\d{2}:\d{2})<`)

	var items []GalleryListItem
	for _, entry := range firstMatches {
		match := secondPattern.FindSubmatch(entry)
		if len(match) >= 4 {
			items = append(items, GalleryListItem{
				Gid:    string(match[1]),
				Token:  string(match[2]),
				Posted: string(match[3]),
			})
		}
	}

	return items, nil
}

// GetMetadatas fetches metadata for a list of galleries from E-Hentai API
func (c *GalleryCrawler) GetMetadatas(gidlist [][2]interface{}) ([]database.GalleryMetadata, error) {
	requestData := map[string]interface{}{
		"method":    "gdata",
		"gidlist":   gidlist,
		"namespace": 1,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	body, err := c.client.Post("https://api.e-hentai.org/api.php", jsonData)
	if err != nil {
		return nil, err
	}

	var response struct {
		Gmetadata []database.GalleryMetadata `json:"gmetadata"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		// Log response body for debugging
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return nil, fmt.Errorf("unmarshal response: %w (response body: %s)", err, preview)
	}

	return response.Gmetadata, nil
}

// GetLastPosted retrieves the last posted timestamp from database
func (c *GalleryCrawler) GetLastPosted(ctx context.Context) (int64, error) {
	pool := database.GetPool()

	query := `
		SELECT EXTRACT(EPOCH FROM posted)::bigint
		FROM gallery
		WHERE bytorrent = false
		ORDER BY posted DESC
		LIMIT 1
	`

	c.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query)),
	)

	var posted int64
	err := pool.QueryRow(ctx, query).Scan(&posted)
	if err != nil {
		return 0, err
	}

	return posted, nil
}

// Sync synchronizes galleries from E-Hentai
func (c *GalleryCrawler) Sync(ctx context.Context) error {
	c.logger.Info("starting gallery sync")

	thresholdPosted, err := c.getSyncThreshold(ctx)
	if err != nil {
		return err
	}

	allItems, err := c.collectGalleryItems(thresholdPosted)
	if err != nil {
		return err
	}

	if len(allItems) == 0 {
		c.logger.Info("no new galleries available")
		return nil
	}

	c.logger.Info("found new galleries", zap.Int("count", len(allItems)))
	c.logGidRange("total", allItems)

	allMetadata, err := c.fetchMetadataForItems(allItems)
	if err != nil {
		return err
	}

	// Import data
	importer := NewImporter(c.logger)
	if err := importer.Import(ctx, allMetadata, c.cfg.Offset != 0); err != nil {
		return fmt.Errorf("import data: %w", err)
	}

	return nil
}

func (c *GalleryCrawler) Backfill(ctx context.Context) error {
	c.logger.Info("starting gallery backfill")

	thresholdPosted, err := c.getSyncThreshold(ctx)
	if err != nil {
		return err
	}

	allItems, err := c.collectGalleryItems(thresholdPosted)
	if err != nil {
		return err
	}

	if len(allItems) == 0 {
		c.logger.Info("no galleries discovered for backfill")
		return nil
	}

	c.logger.Info("discovered galleries for backfill", zap.Int("count", len(allItems)))
	c.logGidRange("backfill_discovered", allItems)

	missingItems, err := c.filterMissingItems(ctx, allItems)
	if err != nil {
		return fmt.Errorf("filter missing galleries: %w", err)
	}

	c.logger.Info("identified missing galleries for backfill",
		zap.Int("missing", len(missingItems)),
		zap.Int("existing", len(allItems)-len(missingItems)),
	)

	if len(missingItems) == 0 {
		c.logger.Info("no missing galleries found during backfill")
		return nil
	}

	c.logGidRange("backfill_missing", missingItems)

	allMetadata, err := c.fetchMetadataForItems(missingItems)
	if err != nil {
		return err
	}

	importer := NewImporter(c.logger)
	if err := importer.Import(ctx, allMetadata, false); err != nil {
		return fmt.Errorf("import backfill data: %w", err)
	}

	return nil
}

func (c *GalleryCrawler) getSyncThreshold(ctx context.Context) (int64, error) {
	lastPosted, err := c.GetLastPosted(ctx)
	if err != nil {
		c.logger.Warn("failed to get last posted, starting from 0", zap.Error(err))
		lastPosted = 0
	}

	c.logger.Info("got last posted time", zap.Int64("posted", lastPosted))

	if c.cfg.Offset != 0 {
		lastPosted -= int64(c.cfg.Offset * 3600)
		c.logger.Info("applied offset", zap.Int64("new_posted", lastPosted))
	}

	return normalizePostedThreshold(lastPosted), nil
}

func normalizePostedThreshold(posted int64) int64 {
	if posted <= 0 {
		return posted
	}

	return posted - (posted % 60)
}

func (c *GalleryCrawler) collectGalleryItems(thresholdPosted int64) ([]GalleryListItem, error) {
	var allItems []GalleryListItem

	c.logger.Debug("fetching normal pages")
	items, err := c.fetchPages(false, thresholdPosted)
	if err != nil {
		return nil, fmt.Errorf("fetch normal pages: %w", err)
	}
	allItems = append(allItems, items...)

	c.logger.Debug("fetching expunged pages")
	items, err = c.fetchPages(true, thresholdPosted)
	if err != nil {
		return nil, fmt.Errorf("fetch expunged pages: %w", err)
	}
	allItems = append(allItems, items...)

	return dedupeGalleryItems(allItems), nil
}

func dedupeGalleryItems(items []GalleryListItem) []GalleryListItem {
	seen := make(map[string]struct{}, len(items))
	result := make([]GalleryListItem, 0, len(items))

	for _, item := range items {
		if _, ok := seen[item.Gid]; ok {
			continue
		}
		seen[item.Gid] = struct{}{}
		result = append(result, item)
	}

	return result
}

func (c *GalleryCrawler) fetchMetadataForItems(items []GalleryListItem) ([]database.GalleryMetadata, error) {
	var allMetadata []database.GalleryMetadata
	for i := 0; i < len(items); i += 25 {
		end := i + 25
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		var gidlist [][2]interface{}
		for _, item := range batch {
			gid, _ := strconv.Atoi(item.Gid)
			gidlist = append(gidlist, [2]interface{}{gid, item.Token})
		}

		c.logger.Debug("fetching metadata batch", zap.Int("from", i), zap.Int("to", end))

		metadata, err := Retry(RetryConfig{
			MaxRetries:     c.retryTimes,
			Logger:         c.logger,
			WaitForIPUnban: c.cfg.WaitForIPUnban,
		}, func() ([]database.GalleryMetadata, error) {
			return c.GetMetadatas(gidlist)
		})

		if err != nil {
			if errors.Is(err, ErrAuthRequired) {
				return nil, fmt.Errorf("auth failed while fetching metadata batch %d-%d: %w", i, end, err)
			}
			c.logger.Error("failed to fetch metadata batch", zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata...)
		time.Sleep(time.Duration(c.cfg.APIDelaySeconds) * time.Second)
	}

	c.logger.Debug("fetched all metadata", zap.Int("count", len(allMetadata)))
	return allMetadata, nil
}

func (c *GalleryCrawler) filterMissingItems(ctx context.Context, items []GalleryListItem) ([]GalleryListItem, error) {
	existingGIDs, err := c.getExistingGalleryIDs(ctx, items)
	if err != nil {
		return nil, err
	}

	missing := make([]GalleryListItem, 0, len(items))
	for _, item := range items {
		gid, err := strconv.Atoi(item.Gid)
		if err != nil {
			c.logger.Warn("failed to parse gid while filtering missing galleries", zap.String("gid", item.Gid), zap.Error(err))
			continue
		}
		if _, ok := existingGIDs[gid]; ok {
			continue
		}
		missing = append(missing, item)
	}

	return missing, nil
}

func (c *GalleryCrawler) getExistingGalleryIDs(ctx context.Context, items []GalleryListItem) (map[int]struct{}, error) {
	pool := database.GetPool()
	existing := make(map[int]struct{})

	const batchSize = 1000
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}

		gids := make([]int, 0, end-start)
		for _, item := range items[start:end] {
			gid, err := strconv.Atoi(item.Gid)
			if err != nil {
				c.logger.Warn("failed to parse gid while building lookup batch", zap.String("gid", item.Gid), zap.Error(err))
				continue
			}
			gids = append(gids, gid)
		}

		if len(gids) == 0 {
			continue
		}

		rows, err := pool.Query(ctx, `SELECT gid FROM gallery WHERE gid = ANY($1)`, gids)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var gid int
			if err := rows.Scan(&gid); err != nil {
				rows.Close()
				return nil, err
			}
			existing[gid] = struct{}{}
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}

		rows.Close()
	}

	return existing, nil
}

// fetchPages fetches all pages until reaching lastPosted
func (c *GalleryCrawler) fetchPages(expunged bool, lastPosted int64) ([]GalleryListItem, error) {
	var allItems []GalleryListItem
	next := ""
	page := 0
	consecutiveOldPages := 0

	for {
		c.logger.Debug("fetching page",
			zap.Bool("expunged", expunged),
			zap.Int("page", page),
		)

		items, err := Retry(RetryConfig{
			MaxRetries:     c.retryTimes,
			Logger:         c.logger,
			WaitForIPUnban: c.cfg.WaitForIPUnban,
		}, func() ([]GalleryListItem, error) {
			return c.GetPages(next, expunged)
		})

		if err != nil {
			return nil, fmt.Errorf("fetch page %d: %w", page, err)
		}

		if len(items) == 0 {
			break
		}

		pageHasRecentItems := false
		for _, item := range items {
			// Parse posted time: "2024-01-01 12:00" -> timestamp
			posted, err := c.parsePostedTime(item.Posted)
			if err != nil {
				c.logger.Warn("failed to parse posted time", zap.String("posted", item.Posted))
				continue
			}

			if posted >= lastPosted {
				allItems = append(allItems, item)
				pageHasRecentItems = true
			} else {
				continue
			}
		}

		if pageHasRecentItems {
			consecutiveOldPages = 0
		} else {
			consecutiveOldPages++
		}

		if consecutiveOldPages >= consecutiveOldPagesLimit {
			break
		}

		// Prepare for next page
		next = items[len(items)-1].Gid
		page++

		// Rate limiting for page fetches
		time.Sleep(time.Duration(c.cfg.PageDelaySeconds) * time.Second)
	}

	return allItems, nil
}

// parsePostedTime parses posted time string to Unix timestamp
func (c *GalleryCrawler) parsePostedTime(posted string) (int64, error) {
	// Format: "2024-01-01 12:00" in UTC
	t, err := time.Parse("2006-01-02 15:04", posted)
	if err != nil {
		return 0, err
	}

	// Assume UTC
	return t.UTC().Unix(), nil
}

// logGidRange logs the gid range of a list of gallery items
func (c *GalleryCrawler) logGidRange(source string, items []GalleryListItem) {
	if len(items) == 0 {
		return
	}

	minGid := items[0].Gid
	maxGid := items[0].Gid

	for _, item := range items[1:] {
		if item.Gid < minGid {
			minGid = item.Gid
		}
		if item.Gid > maxGid {
			maxGid = item.Gid
		}
	}

	c.logger.Info("gid range",
		zap.String("source", source),
		zap.String("min_gid", minGid),
		zap.String("max_gid", maxGid),
		zap.Int("count", len(items)),
	)
}
