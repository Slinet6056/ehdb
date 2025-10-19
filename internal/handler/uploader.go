package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

type UploaderHandler struct {
	logger   *zap.Logger
	maxLimit int
}

func NewUploaderHandler(logger *zap.Logger) *UploaderHandler {
	cfg := config.Get()
	maxLimit := 25 // fallback default
	if cfg != nil && cfg.API.Limits.UploaderMaxLimit > 0 {
		maxLimit = cfg.API.Limits.UploaderMaxLimit
	}
	return &UploaderHandler{
		logger:   logger,
		maxLimit: maxLimit,
	}
}

// GetByUploader handles GET /api/uploader/:uploader
// Supports both traditional pagination (page/limit) and cursor-based pagination (cursor/limit)
// - Use page/limit for shallow pagination (first few pages)
// - Use cursor/limit for deep pagination (performance is constant regardless of offset)
func (h *UploaderHandler) GetByUploader(c *gin.Context) {
	uploader := c.Param("uploader")
	if uploader == "" {
		uploader = c.Query("uploader")
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	cursor := c.Query("cursor") // Cursor format: "timestamp,gid" (composite cursor to handle duplicate timestamps)

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 1
	}
	if limit > h.maxLimit {
		c.JSON(400, utils.GetResponse(nil, 400, "limit is too large", nil))
		return
	}

	if uploader == "" {
		c.JSON(400, utils.GetResponse(nil, 400, "uploader is required", nil))
		return
	}

	// Determine pagination mode
	useCursor := cursor != ""
	var cursorTime int64
	var cursorGid int
	if useCursor {
		// Parse composite cursor: "timestamp,gid"
		parts := strings.Split(cursor, ",")
		if len(parts) != 2 {
			c.JSON(400, utils.GetResponse(nil, 400, "invalid cursor format, expected 'timestamp,gid'", nil))
			return
		}
		var err error
		cursorTime, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			c.JSON(400, utils.GetResponse(nil, 400, "invalid cursor timestamp", nil))
			return
		}
		cursorGid, err = strconv.Atoi(parts[1])
		if err != nil {
			c.JSON(400, utils.GetResponse(nil, 400, "invalid cursor gid", nil))
			return
		}
	}

	ctx := context.Background()
	pool := database.GetPool()

	// Build optimized query
	var query string
	var args []interface{}

	if useCursor {
		// Cursor-based pagination: composite condition to handle duplicate timestamps
		// WHERE (posted < cursor_posted) OR (posted = cursor_posted AND gid < cursor_gid)
		// This query uses the idx_gallery_uploader_exp_posted index for optimal performance
		query = `
			SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			       posted, filecount, filesize, expunged, removed, replaced, rating,
			       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
			FROM gallery
			WHERE uploader = $1 AND expunged = false
			  AND (posted < to_timestamp($2) OR (posted = to_timestamp($2) AND gid < $3))
			ORDER BY posted DESC, gid DESC
			LIMIT $4
		`
		args = []interface{}{uploader, cursorTime, cursorGid, limit}
		h.logger.Debug("executing uploader query (cursor mode)",
			zap.String("sql", utils.FormatSQL(query, uploader, cursorTime, cursorGid, limit)),
		)
	} else {
		// Traditional pagination: OFFSET/LIMIT
		// Uses the same index but performance degrades with large offsets
		offset := (page - 1) * limit
		query = `
			SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			       posted, filecount, filesize, expunged, removed, replaced, rating,
			       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
			FROM gallery
			WHERE uploader = $1 AND expunged = false
			ORDER BY posted DESC, gid DESC
			LIMIT $2 OFFSET $3
		`
		args = []interface{}{uploader, limit, offset}
		h.logger.Debug("executing uploader query (page mode)",
			zap.String("sql", utils.FormatSQL(query, uploader, limit, offset)),
		)
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		h.logger.Error("failed to query galleries by uploader", zap.Error(err))
		c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
		return
	}
	defer rows.Close()

	var galleries []database.Gallery
	var rootGids []int

	for rows.Next() {
		var g database.Gallery
		err := rows.Scan(
			&g.Gid, &g.Token, &g.ArchiverKey, &g.Title, &g.TitleJpn,
			&g.Category, &g.Thumb, &g.Uploader, &g.Posted, &g.Filecount,
			&g.Filesize, &g.Expunged, &g.Removed, &g.Replaced, &g.Rating,
			&g.Torrentcount, &g.RootGid, &g.Bytorrent, &g.Tags,
		)
		if err != nil {
			h.logger.Error("failed to scan gallery", zap.Error(err))
			continue
		}
		galleries = append(galleries, g)
		if g.RootGid != nil {
			rootGids = append(rootGids, *g.RootGid)
		}
	}

	h.logger.Debug("query results",
		zap.Int("galleries_found", len(galleries)),
		zap.Int("root_gids", len(rootGids)),
	)

	// Count total - try materialized view first, fallback to COUNT
	var total int64
	statsQuery := "SELECT COALESCE(gallery_count, 0) FROM uploader_stats_mv WHERE uploader = $1"
	h.logger.Debug("executing count query (materialized view)",
		zap.String("sql", utils.FormatSQL(statsQuery, uploader)),
	)

	err = pool.QueryRow(ctx, statsQuery, uploader).Scan(&total)

	if err != nil || total == 0 {
		h.logger.Warn("failed to get count from stats view or got 0, falling back to COUNT", zap.Error(err))
		countQuery := "SELECT COUNT(*) FROM gallery WHERE uploader = $1 AND expunged = false"
		h.logger.Debug("executing count query (direct)",
			zap.String("sql", utils.FormatSQL(countQuery, uploader)),
		)
		err = pool.QueryRow(ctx, countQuery, uploader).Scan(&total)
		if err != nil {
			h.logger.Error("failed to count galleries", zap.Error(err))
			c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
			return
		}
		h.logger.Debug("count result (direct)", zap.Int64("total", total))
	} else {
		h.logger.Debug("count result (materialized view)", zap.Int64("total", total))
	}

	// Query torrents
	torrentMap := make(map[int][]database.Torrent)
	if len(rootGids) > 0 {
		listHandler := NewListHandler(h.logger)
		torrentMap, _ = listHandler.queryTorrentsForGids(ctx, rootGids)
	}

	// Attach torrents
	for i := range galleries {
		galleries[i].Torrents = []database.Torrent{}
		if galleries[i].RootGid != nil {
			if torrents, ok := torrentMap[*galleries[i].RootGid]; ok {
				galleries[i].Torrents = torrents
			}
		}
	}

	if len(galleries) == 0 {
		c.JSON(200, utils.GetResponse([]database.Gallery{}, 200, "success", &total))
		return
	}

	// Always include next_cursor in response for both pagination modes
	// This allows users to switch from page-based to cursor-based pagination anytime
	lastGallery := galleries[len(galleries)-1]
	lastPosted := lastGallery.Posted.Unix()
	lastGid := lastGallery.Gid
	nextCursor := fmt.Sprintf("%d,%d", lastPosted, lastGid)
	c.JSON(200, utils.GetResponseWithCursor(galleries, 200, "success", &total, &nextCursor))
}
