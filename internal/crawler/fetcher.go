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
	"go.uber.org/zap"
)

// Fetcher manually fetches specific galleries
type Fetcher struct {
	crawler *GalleryCrawler
	logger  *zap.Logger
}

// NewFetcher creates a new fetcher
func NewFetcher(cfg *config.CrawlerConfig, logger *zap.Logger) *Fetcher {
	crawler, _ := NewGalleryCrawler(cfg, logger)
	return &Fetcher{
		crawler: crawler,
		logger:  logger,
	}
}

// Fetch fetches specific galleries by gid/token pairs
func (f *Fetcher) Fetch(ctx context.Context, gidTokens []string) error {
	f.logger.Info("starting fetch", zap.Int("count", len(gidTokens)))

	// Parse gid/token pairs
	var fetchList [][2]interface{}
	pattern := regexp.MustCompile(`(\d+)[/,_\s]([0-9a-f]{10})`)

	for _, item := range gidTokens {
		// Remove /g/ prefix if present
		item = strings.TrimPrefix(item, "/g/")
		item = strings.TrimPrefix(item, "g/")

		matches := pattern.FindStringSubmatch(item)
		if len(matches) >= 3 {
			gid, _ := strconv.Atoi(matches[1])
			token := matches[2]
			fetchList = append(fetchList, [2]interface{}{gid, token})
		} else {
			f.logger.Warn("invalid gid/token format", zap.String("item", item))
		}
	}

	if len(fetchList) == 0 {
		return fmt.Errorf("no valid gid/token pairs found")
	}

	f.logger.Debug("parsed gid/token pairs", zap.Int("count", len(fetchList)))

	// Fetch metadata in batches
	var allMetadata []database.GalleryMetadata
	for i := 0; i < len(fetchList); i += 25 {
		end := i + 25
		if end > len(fetchList) {
			end = len(fetchList)
		}

		batch := fetchList[i:end]

		f.logger.Debug("fetching metadata batch", zap.Int("from", i), zap.Int("to", end))

		metadata, err := Retry(RetryConfig{
			MaxRetries: f.crawler.retryTimes,
			Logger:     f.logger,
		}, func() ([]database.GalleryMetadata, error) {
			return f.crawler.GetMetadatas(batch)
		})

		if err != nil {
			f.logger.Error("failed to fetch metadata batch", zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata...)

		// Rate limiting
		time.Sleep(2 * time.Second)
	}

	f.logger.Debug("fetched all metadata", zap.Int("count", len(allMetadata)))

	// Import data with force flag
	importer := NewImporter(f.logger)
	if err := importer.Import(ctx, allMetadata, true); err != nil {
		return fmt.Errorf("import data: %w", err)
	}

	return nil
}
