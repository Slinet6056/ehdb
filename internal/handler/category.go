package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

type CategoryHandler struct {
	logger   *zap.Logger
	maxLimit int
}

func NewCategoryHandler(logger *zap.Logger) *CategoryHandler {
	cfg := config.Get()
	maxLimit := 25 // fallback default
	if cfg != nil && cfg.API.Limits.CategoryMaxLimit > 0 {
		maxLimit = cfg.API.Limits.CategoryMaxLimit
	}
	return &CategoryHandler{
		logger:   logger,
		maxLimit: maxLimit,
	}
}

// GetByCategory handles GET /api/category/:category or GET /api/cat/:category
// Supports both traditional pagination (page/limit) and cursor-based pagination (cursor/limit)
// - Use page/limit for shallow pagination (first few pages)
// - Use cursor/limit for deep pagination (performance is constant regardless of offset)
func (h *CategoryHandler) GetByCategory(c *gin.Context) {
	categoryParam := c.Param("category")
	if categoryParam == "" {
		categoryParam = c.Query("category")
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

	// Parse category (can be bit mask or category name)
	var categories []string
	if catNum, err := strconv.Atoi(categoryParam); err == nil {
		// Numeric category (bit mask)
		if catNum < 0 {
			catNum = (-catNum) ^ 2047
		}
		categories = utils.GetCategoriesFromBits(catNum)
	} else {
		// String category (support comma-separated list)
		for _, cat := range strings.Split(categoryParam, ",") {
			cat = strings.TrimSpace(cat)
			if cat != "" {
				categories = append(categories, cat)
			}
		}
	}

	if len(categories) == 0 {
		c.JSON(400, utils.GetResponse(nil, 400, "invalid category", nil))
		return
	}

	ctx := context.Background()
	pool := database.GetPool()

	// Optimize query based on number of categories and pagination mode
	// For single category: direct query (best index usage)
	// For multiple categories: UNION ALL (better than ANY for index usage)
	var query string
	var args []interface{}

	if len(categories) == 1 {
		// Single category - direct query with optimal index usage
		if useCursor {
			// Cursor-based pagination: composite condition to handle duplicate timestamps
			// WHERE (posted < cursor_posted) OR (posted = cursor_posted AND gid < cursor_gid)
			query = `
				SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
				       posted, filecount, filesize, expunged, removed, replaced, rating,
				       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
				FROM gallery
				WHERE category = $1 AND expunged = false
				  AND (posted < to_timestamp($2) OR (posted = to_timestamp($2) AND gid < $3))
				ORDER BY posted DESC, gid DESC
				LIMIT $4
			`
			args = []interface{}{categories[0], cursorTime, cursorGid, limit}
			h.logger.Debug("executing single category query (cursor mode)",
				zap.String("sql", utils.FormatSQL(query, categories[0], cursorTime, cursorGid, limit)),
			)
		} else {
			// Traditional pagination: OFFSET/LIMIT
			offset := (page - 1) * limit
			query = `
				SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
				       posted, filecount, filesize, expunged, removed, replaced, rating,
				       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
				FROM gallery
				WHERE category = $1 AND expunged = false
				ORDER BY posted DESC, gid DESC
				LIMIT $2 OFFSET $3
			`
			args = []interface{}{categories[0], limit, offset}
			h.logger.Debug("executing single category query (page mode)",
				zap.String("sql", utils.FormatSQL(query, categories[0], limit, offset)),
			)
		}
	} else {
		// Multiple categories - use UNION ALL for better index usage
		// Each UNION branch can use the index independently and push down LIMIT
		var unions []string

		if useCursor {
			// Cursor-based pagination: each branch uses composite cursor
			for i, cat := range categories {
				unions = append(unions, fmt.Sprintf(`
					(SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
					        posted, filecount, filesize, expunged, removed, replaced, rating,
					        torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
					 FROM gallery
					 WHERE category = $%d AND expunged = false
					   AND (posted < to_timestamp($%d) OR (posted = to_timestamp($%d) AND gid < $%d))
					 ORDER BY posted DESC, gid DESC
					 LIMIT $%d)
				`, i+1, len(categories)+1, len(categories)+1, len(categories)+2, len(categories)+3))
				args = append(args, cat)
			}
			args = append(args, cursorTime, cursorGid, limit)

			query = strings.Join(unions, " UNION ALL ") + `
				ORDER BY posted DESC, gid DESC
				LIMIT ` + fmt.Sprintf("$%d", len(categories)+3)

			h.logger.Debug("executing multi-category query (cursor mode)",
				zap.String("sql", utils.FormatSQL(query, args...)),
				zap.Int("category_count", len(categories)),
			)
		} else {
			// Traditional pagination: each branch needs to fetch enough rows
			offset := (page - 1) * limit
			fetchLimit := limit + offset // Each branch needs to fetch enough rows for offset

			for i, cat := range categories {
				unions = append(unions, fmt.Sprintf(`
					(SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
					        posted, filecount, filesize, expunged, removed, replaced, rating,
					        torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
					 FROM gallery
					 WHERE category = $%d AND expunged = false
					 ORDER BY posted DESC, gid DESC
					 LIMIT $%d)
				`, i+1, len(categories)+1)) // All branches use the same fetchLimit parameter
				args = append(args, cat)
			}

			// Add fetchLimit once (all branches reference the same parameter)
			args = append(args, fetchLimit)

			query = strings.Join(unions, " UNION ALL ") + fmt.Sprintf(`
				ORDER BY posted DESC, gid DESC
				LIMIT $%d OFFSET $%d
			`, len(categories)+2, len(categories)+3)
			args = append(args, limit, offset)

			h.logger.Debug("executing multi-category query (page mode)",
				zap.String("sql", utils.FormatSQL(query, args...)),
				zap.Int("category_count", len(categories)),
				zap.Int("fetch_limit_per_branch", fetchLimit),
			)
		}
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		h.logger.Error("failed to query galleries by category", zap.Error(err))
		c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
		return
	}
	defer rows.Close()

	var galleries []database.Gallery
	var rootGids []int

	for rows.Next() {
		var g database.Gallery
		var postedTime time.Time
		err := rows.Scan(
			&g.Gid, &g.Token, &g.ArchiverKey, &g.Title, &g.TitleJpn,
			&g.Category, &g.Thumb, &g.Uploader, &postedTime, &g.Filecount,
			&g.Filesize, &g.Expunged, &g.Removed, &g.Replaced, &g.Rating,
			&g.Torrentcount, &g.RootGid, &g.Bytorrent, &g.Tags,
		)
		if err != nil {
			h.logger.Error("failed to scan gallery", zap.Error(err))
			continue
		}
		g.Posted = database.UnixTime{Time: postedTime}
		galleries = append(galleries, g)
		if g.RootGid != nil {
			rootGids = append(rootGids, *g.RootGid)
		}
	}

	h.logger.Debug("query results",
		zap.Int("galleries_found", len(galleries)),
		zap.Int("root_gids", len(rootGids)),
	)

	// Count total - use materialized view for all categories
	// Since categories are mutually exclusive in E-Hentai, we can sum the counts
	var total int64

	if len(categories) == 1 {
		// Single category - direct query from materialized view
		statKey := "category_" + strings.ToLower(categories[0])
		statsQuery := "SELECT COALESCE(stat_value, 0) FROM gallery_stats_mv WHERE stat_key = $1"
		h.logger.Debug("executing count query (materialized view, single)",
			zap.String("sql", utils.FormatSQL(statsQuery, statKey)),
		)

		err = pool.QueryRow(ctx, statsQuery, statKey).Scan(&total)

		if err != nil || total == 0 {
			h.logger.Warn("failed to get count from stats view or got 0, falling back to COUNT", zap.Error(err))
			countQuery := "SELECT COUNT(*) FROM gallery WHERE category = $1 AND expunged = false"
			h.logger.Debug("executing count query (direct, single)",
				zap.String("sql", utils.FormatSQL(countQuery, categories[0])),
			)
			err = pool.QueryRow(ctx, countQuery, categories[0]).Scan(&total)
			if err != nil {
				h.logger.Error("failed to count galleries", zap.Error(err))
				c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
				return
			}
			h.logger.Debug("count result (direct)", zap.Int64("total", total))
		} else {
			h.logger.Debug("count result (materialized view)", zap.Int64("total", total))
		}
	} else {
		// Multiple categories - sum from materialized view
		statKeys := make([]string, len(categories))
		for i, cat := range categories {
			statKeys[i] = "category_" + strings.ToLower(cat)
		}

		statsQuery := "SELECT COALESCE(SUM(stat_value), 0) FROM gallery_stats_mv WHERE stat_key = ANY($1)"
		h.logger.Debug("executing count query (materialized view, multi)",
			zap.String("sql", utils.FormatSQL(statsQuery, statKeys)),
		)

		err = pool.QueryRow(ctx, statsQuery, statKeys).Scan(&total)

		if err != nil || total == 0 {
			h.logger.Warn("failed to get count from stats view or got 0, falling back to COUNT", zap.Error(err))

			// Build count query for multiple categories
			var countArgs []interface{}
			if len(categories) == 1 {
				countQuery := "SELECT COUNT(*) FROM gallery WHERE category = $1 AND expunged = false"
				countArgs = []interface{}{categories[0]}
				h.logger.Debug("executing count query (direct, multi-fallback)",
					zap.String("sql", utils.FormatSQL(countQuery, countArgs...)),
				)
				err = pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
			} else {
				countQuery := "SELECT COUNT(*) FROM gallery WHERE category = ANY($1) AND expunged = false"
				countArgs = []interface{}{categories}
				h.logger.Debug("executing count query (direct, multi-fallback)",
					zap.String("sql", utils.FormatSQL(countQuery, categories)),
				)
				err = pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
			}

			if err != nil {
				h.logger.Error("failed to count galleries", zap.Error(err))
				c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
				return
			}
			h.logger.Debug("count result (direct)", zap.Int64("total", total))
		} else {
			h.logger.Debug("count result (materialized view)", zap.Int64("total", total))
		}
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
