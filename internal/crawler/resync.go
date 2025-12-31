package crawler

import (
	"context"
	"fmt"
	"time"

	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

// Resyncer resyncs galleries from recent hours
type Resyncer struct {
	crawler *GalleryCrawler
	logger  *zap.Logger
}

// NewResyncer creates a new resyncer
func NewResyncer(cfg *config.CrawlerConfig, logger *zap.Logger) *Resyncer {
	crawler, _ := NewGalleryCrawler(cfg, logger)
	return &Resyncer{
		crawler: crawler,
		logger:  logger,
	}
}

// Resync resyncs galleries from the last N hours
func (r *Resyncer) Resync(ctx context.Context, hours int) error {
	r.logger.Info("starting resync", zap.Int("hours", hours))

	pool := database.GetPool()

	// Get galleries from the last N hours
	sinceTimestamp := time.Now().Unix() - int64(hours*3600)
	query := `
		SELECT gid, token
		FROM gallery
		WHERE EXTRACT(EPOCH FROM posted) >= $1
		ORDER BY gid ASC
	`

	r.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query, sinceTimestamp)),
	)

	rows, err := pool.Query(ctx, query, sinceTimestamp)
	if err != nil {
		return fmt.Errorf("query galleries: %w", err)
	}
	defer rows.Close()

	var gidTokens []struct {
		Gid   int
		Token string
	}

	for rows.Next() {
		var item struct {
			Gid   int
			Token string
		}
		if err := rows.Scan(&item.Gid, &item.Token); err != nil {
			r.logger.Error("failed to scan gallery", zap.Error(err))
			continue
		}
		gidTokens = append(gidTokens, item)
	}

	r.logger.Info("found galleries to resync", zap.Int("count", len(gidTokens)))

	if len(gidTokens) == 0 {
		r.logger.Info("no galleries to resync")
		return nil
	}

	// Fetch metadata in batches
	var allMetadata []database.GalleryMetadata
	for i := 0; i < len(gidTokens); i += 25 {
		end := i + 25
		if end > len(gidTokens) {
			end = len(gidTokens)
		}

		batch := gidTokens[i:end]
		var gidlist [][2]interface{}
		for _, item := range batch {
			gidlist = append(gidlist, [2]interface{}{item.Gid, item.Token})
		}

		r.logger.Debug("fetching metadata batch", zap.Int("from", i), zap.Int("to", end))

		metadata, err := Retry(RetryConfig{
			MaxRetries:     r.crawler.retryTimes,
			Logger:         r.logger,
			WaitForIPUnban: r.crawler.cfg.WaitForIPUnban,
		}, func() ([]database.GalleryMetadata, error) {
			return r.crawler.GetMetadatas(gidlist)
		})

		if err != nil {
			r.logger.Error("failed to fetch metadata batch", zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata...)

		// Rate limiting for API calls
		time.Sleep(time.Duration(r.crawler.cfg.APIDelaySeconds) * time.Second)
	}

	r.logger.Debug("fetched all metadata", zap.Int("count", len(allMetadata)))

	// Import data with force flag
	importer := NewImporter(r.logger)
	if err := importer.Import(ctx, allMetadata, true); err != nil {
		return fmt.Errorf("import data: %w", err)
	}

	return nil
}
