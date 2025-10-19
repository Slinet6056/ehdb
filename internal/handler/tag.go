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

type TagHandler struct {
	logger   *zap.Logger
	maxLimit int
}

func NewTagHandler(logger *zap.Logger) *TagHandler {
	cfg := config.Get()
	maxLimit := 25 // fallback default
	if cfg != nil && cfg.API.Limits.TagMaxLimit > 0 {
		maxLimit = cfg.API.Limits.TagMaxLimit
	}
	return &TagHandler{
		logger:   logger,
		maxLimit: maxLimit,
	}
}

// GetByTag handles GET /api/tag/:tag
// Supports both traditional pagination (page/limit) and cursor-based pagination (cursor/limit)
// - Use page/limit for shallow pagination (first few pages)
// - Use cursor/limit for deep pagination (performance is constant regardless of offset)
func (h *TagHandler) GetByTag(c *gin.Context) {
	tag := c.Param("tag")
	if tag == "" {
		tag = c.Query("tag")
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

	// Parse multiple tags (comma-separated)
	tagList := strings.Split(tag, ",")
	var normalizedTags []string
	for _, t := range tagList {
		t = strings.TrimSpace(t)
		if t != "" {
			normalizedTags = append(normalizedTags, utils.NormalizeTag(t))
		}
	}

	if len(normalizedTags) == 0 {
		c.JSON(400, utils.GetResponse(nil, 400, "tag is not defined", nil))
		return
	}

	ctx := context.Background()
	pool := database.GetPool()

	// Build query for multiple tags (all tags must be present)
	// Use JSONB containment operator (@>) which can utilize GIN index (idx_gallery_tags)
	// Merge all tags into a single JSONB array for better performance (one index lookup instead of multiple)
	var query string
	var args []interface{}

	// Build merged tags JSONB array: ["tag1", "tag2", "tag3"]
	var tagArray []string
	for _, t := range normalizedTags {
		tagArray = append(tagArray, `"`+t+`"`)
	}
	mergedTags := "[" + strings.Join(tagArray, ", ") + "]"
	args = append(args, mergedTags)

	if useCursor {
		// Cursor-based pagination: composite condition to handle duplicate timestamps
		// WHERE tags @> $1::jsonb AND expunged = false
		//   AND (posted < cursor_posted OR (posted = cursor_posted AND gid < cursor_gid))
		query = `
			SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			       posted, filecount, filesize, expunged, removed, replaced, rating,
			       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
			FROM gallery
			WHERE tags @> $1::jsonb AND expunged = false
			  AND (posted < to_timestamp($2) OR (posted = to_timestamp($2) AND gid < $3))
			ORDER BY posted DESC, gid DESC
			LIMIT $4
		`
		args = append(args, cursorTime, cursorGid, limit)

		h.logger.Debug("executing tag query (cursor mode)",
			zap.String("sql", utils.FormatSQL(query, args...)),
			zap.Strings("tags", normalizedTags),
		)
	} else {
		// Traditional pagination: OFFSET/LIMIT
		offset := (page - 1) * limit
		query = `
			SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			       posted, filecount, filesize, expunged, removed, replaced, rating,
			       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
			FROM gallery
			WHERE tags @> $1::jsonb AND expunged = false
			ORDER BY posted DESC, gid DESC
			LIMIT $2 OFFSET $3
		`
		args = append(args, limit, offset)

		h.logger.Debug("executing tag query (page mode)",
			zap.String("sql", utils.FormatSQL(query, args...)),
			zap.Strings("tags", normalizedTags),
		)
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		h.logger.Error("failed to query galleries by tag", zap.Error(err))
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

	// Count total - use merged tags for single index lookup
	countQuery := "SELECT COUNT(*) FROM gallery WHERE tags @> $1::jsonb AND expunged = false"
	countArgs := []interface{}{mergedTags}

	h.logger.Debug("executing count query",
		zap.String("sql", utils.FormatSQL(countQuery, countArgs...)),
	)

	var total int64
	err = pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		h.logger.Error("failed to count galleries", zap.Error(err))
		c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
		return
	}

	h.logger.Debug("count result", zap.Int64("total", total))

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
