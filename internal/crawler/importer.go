package crawler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

// Importer imports gallery data to database
type Importer struct {
	logger *zap.Logger
}

// NewImporter creates a new importer
func NewImporter(logger *zap.Logger) *Importer {
	return &Importer{logger: logger}
}

// Import imports gallery metadata to database
func (imp *Importer) Import(ctx context.Context, metadataList []database.GalleryMetadata, force bool) error {
	imp.logger.Info("starting data import", zap.Int("count", len(metadataList)))

	imported := 0

	// Load existing galleries
	existingGalleries, err := imp.loadGalleries(ctx)
	if err != nil {
		return fmt.Errorf("load galleries: %w", err)
	}

	for idx, metadata := range metadataList {
		if metadata.Error != "" {
			imp.logger.Warn("metadata has error, skipping", zap.Int("gid", metadata.Gid), zap.String("error", metadata.Error))
			continue
		}

		// Normalize tags
		var normalizedTags []string
		for _, tag := range metadata.Tags {
			normalizedTags = append(normalizedTags, utils.NormalizeTag(tag))
		}

		// Parse posted time (format: "1609459200" Unix timestamp string)
		postedInt, err := strconv.ParseInt(metadata.Posted, 10, 64)
		if err != nil {
			imp.logger.Error("failed to parse posted time", zap.Int("gid", metadata.Gid), zap.Error(err))
			continue
		}
		posted := time.Unix(postedInt, 0).UTC()

		// Parse numeric fields
		filecount, _ := strconv.Atoi(metadata.Filecount)
		rating, _ := strconv.ParseFloat(metadata.Rating, 64)
		torrentcount, _ := strconv.Atoi(metadata.Torrentcount)

		// Check if gallery exists
		existingPosted, exists := existingGalleries[metadata.Gid]

		if !exists {
			// Insert new gallery
			imp.logger.Debug("inserting new gallery", zap.Int("gid", metadata.Gid))

			err := imp.insertGallery(ctx, metadata, posted, filecount, rating, torrentcount, normalizedTags)
			if err != nil {
				imp.logger.Error("failed to insert gallery", zap.Int("gid", metadata.Gid), zap.Error(err))
				continue
			}

			imported++
		} else if force || postedInt > existingPosted {
			// Update existing gallery
			imp.logger.Debug("updating existing gallery", zap.Int("gid", metadata.Gid))

			err := imp.updateGallery(ctx, metadata, posted, filecount, rating, torrentcount, normalizedTags)
			if err != nil {
				imp.logger.Error("failed to update gallery", zap.Int("gid", metadata.Gid), zap.Error(err))
				continue
			}

			imported++
		}

		if (idx+1)%1000 == 0 {
			imp.logger.Info("import progress", zap.Int("processed", idx+1), zap.Int("imported", imported))
		}
	}

	imp.logger.Info("import completed", zap.Int("imported", imported))

	// Refresh statistics if data was imported
	if imported > 0 {
		imp.logger.Debug("refreshing statistics views")
		if err := imp.refreshStats(ctx); err != nil {
			imp.logger.Error("failed to refresh stats", zap.Error(err))
		}
	}

	return nil
}

// loadGalleries loads existing galleries from database
func (imp *Importer) loadGalleries(ctx context.Context) (map[int]int64, error) {
	pool := database.GetPool()

	query := `SELECT gid, EXTRACT(EPOCH FROM posted)::bigint FROM gallery`
	imp.logger.Debug("executing query",
		zap.String("sql", utils.FormatSQL(query)),
	)
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	galleries := make(map[int]int64)
	for rows.Next() {
		var gid int
		var posted int64
		if err := rows.Scan(&gid, &posted); err != nil {
			return nil, err
		}
		galleries[gid] = posted
	}

	return galleries, nil
}

// insertGallery inserts a new gallery
func (imp *Importer) insertGallery(ctx context.Context, metadata database.GalleryMetadata, posted time.Time, filecount int, rating float64, torrentcount int, tags []string) error {
	pool := database.GetPool()

	// Convert tags to JSONB array
	tagsJSON, err := tagsToJSON(tags)
	if err != nil {
		return fmt.Errorf("convert tags to JSON: %w", err)
	}

	query := `
		INSERT INTO gallery (
			gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			posted, filecount, filesize, expunged, rating, torrentcount, tags
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
	`

	imp.logger.Debug("executing insert query",
		zap.String("sql", utils.FormatSQL(query,
			metadata.Gid,
			metadata.Token,
			metadata.ArchiverKey,
			metadata.Title,
			metadata.TitleJpn,
			metadata.Category,
			metadata.Thumb,
			metadata.Uploader,
			posted,
			filecount,
			metadata.Filesize,
			metadata.Expunged,
			rating,
			torrentcount,
			tagsJSON,
		)),
	)

	_, err = pool.Exec(ctx, query,
		metadata.Gid,
		metadata.Token,
		metadata.ArchiverKey,
		metadata.Title,
		metadata.TitleJpn,
		metadata.Category,
		metadata.Thumb,
		metadata.Uploader,
		posted,
		filecount,
		metadata.Filesize,
		metadata.Expunged,
		rating,
		torrentcount,
		tagsJSON,
	)

	return err
}

// updateGallery updates an existing gallery
func (imp *Importer) updateGallery(ctx context.Context, metadata database.GalleryMetadata, posted time.Time, filecount int, rating float64, torrentcount int, tags []string) error {
	pool := database.GetPool()

	// Convert tags to JSONB array
	tagsJSON, err := tagsToJSON(tags)
	if err != nil {
		return fmt.Errorf("convert tags to JSON: %w", err)
	}

	query := `
		UPDATE gallery SET
			token = $2,
			archiver_key = $3,
			title = $4,
			title_jpn = $5,
			category = $6,
			thumb = $7,
			uploader = $8,
			posted = $9,
			filecount = $10,
			filesize = $11,
			expunged = $12,
			rating = $13,
			torrentcount = $14,
			bytorrent = false,
			tags = $15
		WHERE gid = $1
	`

	imp.logger.Debug("executing update query",
		zap.String("sql", utils.FormatSQL(query,
			metadata.Gid,
			metadata.Token,
			metadata.ArchiverKey,
			metadata.Title,
			metadata.TitleJpn,
			metadata.Category,
			metadata.Thumb,
			metadata.Uploader,
			posted,
			filecount,
			metadata.Filesize,
			metadata.Expunged,
			rating,
			torrentcount,
			tagsJSON,
		)),
	)

	_, err = pool.Exec(ctx, query,
		metadata.Gid,
		metadata.Token,
		metadata.ArchiverKey,
		metadata.Title,
		metadata.TitleJpn,
		metadata.Category,
		metadata.Thumb,
		metadata.Uploader,
		posted,
		filecount,
		metadata.Filesize,
		metadata.Expunged,
		rating,
		torrentcount,
		tagsJSON,
	)

	return err
}

// refreshStats refreshes statistics materialized views
func (imp *Importer) refreshStats(ctx context.Context) error {
	pool := database.GetPool()

	query := "SELECT refresh_all_stats(false)"
	imp.logger.Debug("executing stats refresh",
		zap.String("sql", utils.FormatSQL(query)),
	)
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("refresh stats: %w", err)
	}

	return nil
}

// tagsToJSON converts tag array to JSON string
func tagsToJSON(tags []string) (string, error) {
	if len(tags) == 0 {
		return "[]", nil
	}

	result := "["
	for i, tag := range tags {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`"%s"`, tag)
	}
	result += "]"

	return result, nil
}
