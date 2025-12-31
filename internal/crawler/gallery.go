package crawler

import (
	"context"
	"encoding/json"
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

	// Get last posted time
	lastPosted, err := c.GetLastPosted(ctx)
	if err != nil {
		c.logger.Warn("failed to get last posted, starting from 0", zap.Error(err))
		lastPosted = 0
	}

	c.logger.Info("got last posted time", zap.Int64("posted", lastPosted))

	// Apply offset if configured
	if c.cfg.Offset != 0 {
		lastPosted -= int64(c.cfg.Offset * 3600)
		c.logger.Info("applied offset", zap.Int64("new_posted", lastPosted))
	}

	var allItems []GalleryListItem

	// Fetch normal pages
	c.logger.Debug("fetching normal pages")
	items, err := c.fetchPages(false, lastPosted)
	if err != nil {
		return fmt.Errorf("fetch normal pages: %w", err)
	}
	allItems = append(allItems, items...)

	// Fetch expunged pages
	c.logger.Debug("fetching expunged pages")
	items, err = c.fetchPages(true, lastPosted)
	if err != nil {
		return fmt.Errorf("fetch expunged pages: %w", err)
	}
	allItems = append(allItems, items...)

	if len(allItems) == 0 {
		c.logger.Info("no new galleries available")
		return nil
	}

	c.logger.Info("found new galleries", zap.Int("count", len(allItems)))
	c.logGidRange("total", allItems)

	// Sort by gid
	// (skipping sort for simplicity, items are already chronologically ordered)

	// Fetch metadata in batches of 25
	var allMetadata []database.GalleryMetadata
	for i := 0; i < len(allItems); i += 25 {
		end := i + 25
		if end > len(allItems) {
			end = len(allItems)
		}

		batch := allItems[i:end]
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
			c.logger.Error("failed to fetch metadata batch", zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata...)

		// Rate limiting for API calls
		time.Sleep(time.Duration(c.cfg.APIDelaySeconds) * time.Second)
	}

	c.logger.Debug("fetched all metadata", zap.Int("count", len(allMetadata)))

	// Import data
	importer := NewImporter(c.logger)
	if err := importer.Import(ctx, allMetadata, c.cfg.Offset != 0); err != nil {
		return fmt.Errorf("import data: %w", err)
	}

	return nil
}

// fetchPages fetches all pages until reaching lastPosted
func (c *GalleryCrawler) fetchPages(expunged bool, lastPosted int64) ([]GalleryListItem, error) {
	var allItems []GalleryListItem
	next := ""
	page := 0

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

		// Check each item's posted time
		finish := false
		for _, item := range items {
			// Parse posted time: "2024-01-01 12:00" -> timestamp
			posted, err := c.parsePostedTime(item.Posted)
			if err != nil {
				c.logger.Warn("failed to parse posted time", zap.String("posted", item.Posted))
				continue
			}

			if posted > lastPosted {
				allItems = append(allItems, item)
			} else {
				finish = true
				break
			}
		}

		if finish {
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
