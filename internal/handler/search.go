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

type SearchHandler struct {
	logger   *zap.Logger
	maxLimit int
}

func NewSearchHandler(logger *zap.Logger) *SearchHandler {
	cfg := config.Get()
	maxLimit := 25 // fallback default
	if cfg != nil && cfg.API.Limits.SearchMaxLimit > 0 {
		maxLimit = cfg.API.Limits.SearchMaxLimit
	}
	return &SearchHandler{
		logger:   logger,
		maxLimit: maxLimit,
	}
}

// Search handles GET /api/search
func (h *SearchHandler) Search(c *gin.Context) {
	// Parse query parameters
	keyword := c.Query("keyword")
	categoryParam := c.Query("category")
	expungedParam := c.DefaultQuery("expunged", "0")
	removedParam := c.DefaultQuery("removed", "0")
	replacedParam := c.DefaultQuery("replaced", "0")
	minPageParam := c.DefaultQuery("minpage", "0")
	maxPageParam := c.DefaultQuery("maxpage", "0")
	minRatingParam := c.DefaultQuery("minrating", "0")
	maxDateParam := c.Query("maxdate")
	minDateParam := c.Query("mindate")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	cursor := c.Query("cursor")

	// Validate and normalize parameters
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

	expunged, _ := strconv.Atoi(expungedParam)
	removed, _ := strconv.Atoi(removedParam)
	replaced, _ := strconv.Atoi(replacedParam)
	minPage, _ := strconv.Atoi(minPageParam)
	maxPage, _ := strconv.Atoi(maxPageParam)
	minRating, _ := strconv.ParseFloat(minRatingParam, 64)

	if minRating < 0 {
		minRating = 0
	}
	if minRating > 5 {
		minRating = 5
	}

	// Parse date range parameters (Unix timestamps)
	var maxDate, minDate int64
	if maxDateParam != "" {
		maxDate, _ = strconv.ParseInt(maxDateParam, 10, 64)
	}
	if minDateParam != "" {
		minDate, _ = strconv.ParseInt(minDateParam, 10, 64)
	}

	// Parse cursor for cursor-based pagination
	useCursor := cursor != ""
	var cursorTime int64
	var cursorGid int
	if useCursor {
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

	// Parse categories
	var categories []string
	if categoryParam != "" {
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
	}

	ctx := context.Background()
	pool := database.GetPool()

	// Parse search keyword
	searchQuery := utils.ParseSearchKeyword(keyword)

	h.logger.Debug("parsed search query",
		zap.String("keyword", keyword),
		zap.Int("phrases", len(searchQuery.Phrases)),
		zap.Int("tags", len(searchQuery.Tags)),
		zap.Int("tag_prefixes", len(searchQuery.TagPrefixes)),
		zap.Int("wildcards", len(searchQuery.Wildcards)),
		zap.Int("excludes", len(searchQuery.Excludes)),
		zap.Int("or_groups", len(searchQuery.OrGroups)),
		zap.Int("keywords", len(searchQuery.Keywords)),
	)

	// Expand tag prefixes by querying tag table
	// Returns map: prefix -> list of expanded tags
	expandedTagGroups, hasUnmatchedPrefixes := h.expandTagPrefixesGrouped(ctx, searchQuery.TagPrefixes)

	totalExpandedTags := 0
	for _, tags := range expandedTagGroups {
		totalExpandedTags += len(tags)
	}

	h.logger.Debug("expanded tags",
		zap.Int("prefix_count", len(searchQuery.TagPrefixes)),
		zap.Int("total_expanded_tags", totalExpandedTags),
		zap.Bool("has_unmatched_prefixes", hasUnmatchedPrefixes),
	)

	// Build WHERE conditions
	var conditions []string
	var args []interface{}
	argIndex := 1

	// If we have prefix tags that didn't match anything, return 0 results
	if hasUnmatchedPrefixes {
		conditions = append(conditions, "FALSE")
	}

	// Base condition: expunged
	if expunged == 0 {
		conditions = append(conditions, "expunged = false")
	}

	// Removed condition
	if removed == 0 {
		conditions = append(conditions, "removed = false")
	}

	// Replaced condition
	if replaced == 0 {
		conditions = append(conditions, "replaced = false")
	}

	// Category condition
	if len(categories) > 0 {
		categoryPlaceholders := make([]string, len(categories))
		for i, cat := range categories {
			categoryPlaceholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, cat)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("category IN (%s)", strings.Join(categoryPlaceholders, ", ")))
	}

	// Page count conditions
	if minPage > 0 {
		conditions = append(conditions, fmt.Sprintf("filecount >= $%d", argIndex))
		args = append(args, minPage)
		argIndex++
	}
	if maxPage > 0 {
		conditions = append(conditions, fmt.Sprintf("filecount <= $%d", argIndex))
		args = append(args, maxPage)
		argIndex++
	}

	// Rating condition
	if minRating > 0 {
		conditions = append(conditions, fmt.Sprintf("rating >= $%d", argIndex))
		args = append(args, minRating)
		argIndex++
	}

	// Date range conditions
	if maxDate > 0 {
		conditions = append(conditions, fmt.Sprintf("posted <= to_timestamp($%d)", argIndex))
		args = append(args, maxDate)
		argIndex++
	}
	if minDate > 0 {
		conditions = append(conditions, fmt.Sprintf("posted >= to_timestamp($%d)", argIndex))
		args = append(args, minDate)
		argIndex++
	}

	// Tags condition
	// Exact tags: all must be present (AND relationship)
	// Combine into single JSONB containment check for better performance
	if len(searchQuery.Tags) > 0 {
		tagArray := make([]string, len(searchQuery.Tags))
		for i, tag := range searchQuery.Tags {
			tagArray[i] = `"` + tag + `"`
		}
		mergedTags := "[" + strings.Join(tagArray, ", ") + "]"
		conditions = append(conditions, fmt.Sprintf("tags @> $%d::jsonb", argIndex))
		args = append(args, mergedTags)
		argIndex++
	}

	// Prefix tags: each prefix's expanded tags are OR (using ?| operator for better performance)
	// Different prefixes are AND
	for prefix, expandedTags := range expandedTagGroups {
		if len(expandedTags) == 0 {
			continue // Skip empty groups (already handled by hasUnmatchedPrefixes)
		}

		// Use ?| operator: tags ?| array['tag1', 'tag2', ...]
		// This checks if tags contains any of the values in the array
		conditions = append(conditions, fmt.Sprintf("tags ?| $%d", argIndex))
		args = append(args, expandedTags)
		argIndex++

		h.logger.Debug("added prefix tag group",
			zap.String("prefix", prefix),
			zap.Int("expanded_count", len(expandedTags)),
		)
	}

	// Build title search conditions
	var titleConditions []string

	// Exact phrases (must all match)
	for _, phrase := range searchQuery.Phrases {
		titleConditions = append(titleConditions, fmt.Sprintf(
			"(title ILIKE $%d OR title_jpn ILIKE $%d)",
			argIndex, argIndex+1,
		))
		phrasePattern := "%" + phrase + "%"
		args = append(args, phrasePattern, phrasePattern)
		argIndex += 2
	}

	// Regular keywords (must all match)
	for _, kw := range searchQuery.Keywords {
		titleConditions = append(titleConditions, fmt.Sprintf(
			"(title ILIKE $%d OR title_jpn ILIKE $%d)",
			argIndex, argIndex+1,
		))
		kwPattern := "%" + kw + "%"
		args = append(args, kwPattern, kwPattern)
		argIndex += 2
	}

	// Wildcard terms (must all match)
	for _, wildcard := range searchQuery.Wildcards {
		titleConditions = append(titleConditions, fmt.Sprintf(
			"(title ILIKE $%d OR title_jpn ILIKE $%d)",
			argIndex, argIndex+1,
		))
		args = append(args, wildcard, wildcard)
		argIndex += 2
	}

	// Exclude terms (must not match any)
	for _, exclude := range searchQuery.Excludes {
		// Check if this is a tag exclusion
		if strings.HasPrefix(exclude, "TAG_EXACT:") {
			// Exact tag exclusion: NOT (tags ? 'tag')
			tagValue := strings.TrimPrefix(exclude, "TAG_EXACT:")
			conditions = append(conditions, fmt.Sprintf("NOT (tags ? $%d)", argIndex))
			args = append(args, tagValue)
			argIndex++
		} else if strings.HasPrefix(exclude, "TAG_PREFIX:") {
			// Tag prefix exclusion: expand and use NOT (tags ?| array[...])
			tagPrefix := strings.TrimPrefix(exclude, "TAG_PREFIX:")
			expandedTags := h.expandSingleTagPrefix(ctx, tagPrefix)
			if len(expandedTags) > 0 {
				conditions = append(conditions, fmt.Sprintf("NOT (tags ?| $%d)", argIndex))
				args = append(args, expandedTags)
				argIndex++
			}
			// If no tags matched, don't add any condition (nothing to exclude)
		} else {
			// Regular title exclusion
			titleConditions = append(titleConditions, fmt.Sprintf(
				"(title NOT ILIKE $%d AND title_jpn NOT ILIKE $%d)",
				argIndex, argIndex+1,
			))
			excludePattern := "%" + exclude + "%"
			args = append(args, excludePattern, excludePattern)
			argIndex += 2
		}
	}

	// OR groups (at least one in each group must match)
	for _, orGroup := range searchQuery.OrGroups {
		var orConditions []string
		var tagOrConditions []string

		for _, orTerm := range orGroup {
			// Check if this is a tag OR
			if strings.HasPrefix(orTerm, "TAG_EXACT:") {
				// Exact tag OR: tags ? 'tag'
				tagValue := strings.TrimPrefix(orTerm, "TAG_EXACT:")
				tagOrConditions = append(tagOrConditions, fmt.Sprintf("(tags ? $%d)", argIndex))
				args = append(args, tagValue)
				argIndex++
			} else if strings.HasPrefix(orTerm, "TAG_PREFIX:") {
				// Tag prefix OR: expand and use tags ?| array[...]
				tagPrefix := strings.TrimPrefix(orTerm, "TAG_PREFIX:")
				expandedTags := h.expandSingleTagPrefix(ctx, tagPrefix)
				if len(expandedTags) > 0 {
					tagOrConditions = append(tagOrConditions, fmt.Sprintf("(tags ?| $%d)", argIndex))
					args = append(args, expandedTags)
					argIndex++
				}
				// If no tags matched, this OR branch will never match
			} else {
				// Regular title OR
				orConditions = append(orConditions, fmt.Sprintf(
					"(title ILIKE $%d OR title_jpn ILIKE $%d)",
					argIndex, argIndex+1,
				))
				orPattern := "%" + orTerm + "%"
				args = append(args, orPattern, orPattern)
				argIndex += 2
			}
		}

		// Combine tag OR conditions with title OR conditions
		allOrConditions := append(tagOrConditions, orConditions...)
		if len(allOrConditions) > 0 {
			if len(tagOrConditions) > 0 && len(orConditions) > 0 {
				// Mixed: add to main conditions (tags) and title conditions
				conditions = append(conditions, "("+strings.Join(allOrConditions, " OR ")+")")
			} else if len(tagOrConditions) > 0 {
				// Only tag conditions: add to main conditions
				conditions = append(conditions, "("+strings.Join(tagOrConditions, " OR ")+")")
			} else {
				// Only title conditions: add to title conditions
				titleConditions = append(titleConditions, "("+strings.Join(orConditions, " OR ")+")")
			}
		}
	}

	// Combine title conditions
	if len(titleConditions) > 0 {
		conditions = append(conditions, "("+strings.Join(titleConditions, " AND ")+")")
	}

	// Cursor or offset conditions
	if useCursor {
		conditions = append(conditions, fmt.Sprintf(
			"(posted < to_timestamp($%d) OR (posted = to_timestamp($%d) AND gid < $%d))",
			argIndex, argIndex, argIndex+1,
		))
		args = append(args, cursorTime, cursorGid)
		argIndex += 2
	}

	// Build the main query
	whereClause := "WHERE " + strings.Join(conditions, " AND ")
	if len(conditions) == 0 {
		whereClause = ""
	}

	var query string
	if useCursor {
		query = fmt.Sprintf(`
			SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			       posted, filecount, filesize, expunged, removed, replaced, rating,
			       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
			FROM gallery
			%s
			ORDER BY posted DESC, gid DESC
			LIMIT $%d
		`, whereClause, argIndex)
		args = append(args, limit)
	} else {
		offset := (page - 1) * limit
		query = fmt.Sprintf(`
			SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
			       posted, filecount, filesize, expunged, removed, replaced, rating,
			       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
			FROM gallery
			%s
			ORDER BY posted DESC, gid DESC
			LIMIT $%d OFFSET $%d
		`, whereClause, argIndex, argIndex+1)
		args = append(args, limit, offset)
	}

	h.logger.Debug("executing search query",
		zap.String("sql", utils.FormatSQL(query, args...)),
	)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		h.logger.Error("failed to execute search query", zap.Error(err))
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

	h.logger.Debug("search results",
		zap.Int("galleries_found", len(galleries)),
		zap.Int("root_gids", len(rootGids)),
	)

	// Count total (this might be slow for complex queries, consider caching or approximation)
	var total int64
	countWhereClause := whereClause
	countArgs := args[:len(args)-2] // Remove LIMIT and OFFSET args
	if useCursor {
		// For cursor mode, we need to remove cursor conditions for accurate count
		// Rebuild conditions without cursor
		var countConditions []string
		countArgIndex := 1
		var countArgsTemp []interface{}

		// If we have prefix tags that didn't match anything, return 0 count
		if hasUnmatchedPrefixes {
			countConditions = append(countConditions, "FALSE")
		}

		if expunged == 0 {
			countConditions = append(countConditions, "expunged = false")
		}
		if removed == 0 {
			countConditions = append(countConditions, "removed = false")
		}
		if replaced == 0 {
			countConditions = append(countConditions, "replaced = false")
		}
		if len(categories) > 0 {
			categoryPlaceholders := make([]string, len(categories))
			for i, cat := range categories {
				categoryPlaceholders[i] = fmt.Sprintf("$%d", countArgIndex)
				countArgsTemp = append(countArgsTemp, cat)
				countArgIndex++
			}
			countConditions = append(countConditions, fmt.Sprintf("category IN (%s)", strings.Join(categoryPlaceholders, ", ")))
		}
		if minPage > 0 {
			countConditions = append(countConditions, fmt.Sprintf("filecount >= $%d", countArgIndex))
			countArgsTemp = append(countArgsTemp, minPage)
			countArgIndex++
		}
		if maxPage > 0 {
			countConditions = append(countConditions, fmt.Sprintf("filecount <= $%d", countArgIndex))
			countArgsTemp = append(countArgsTemp, maxPage)
			countArgIndex++
		}
		if minRating > 0 {
			countConditions = append(countConditions, fmt.Sprintf("rating >= $%d", countArgIndex))
			countArgsTemp = append(countArgsTemp, minRating)
			countArgIndex++
		}
		// Date range conditions for count
		if maxDate > 0 {
			countConditions = append(countConditions, fmt.Sprintf("posted <= to_timestamp($%d)", countArgIndex))
			countArgsTemp = append(countArgsTemp, maxDate)
			countArgIndex++
		}
		if minDate > 0 {
			countConditions = append(countConditions, fmt.Sprintf("posted >= to_timestamp($%d)", countArgIndex))
			countArgsTemp = append(countArgsTemp, minDate)
			countArgIndex++
		}
		// Tags condition for count (same logic as main query)
		// Exact tags: all must be present (AND relationship)
		// Combine into single JSONB containment check for better performance
		if len(searchQuery.Tags) > 0 {
			tagArray := make([]string, len(searchQuery.Tags))
			for i, tag := range searchQuery.Tags {
				tagArray[i] = `"` + tag + `"`
			}
			mergedTags := "[" + strings.Join(tagArray, ", ") + "]"
			countConditions = append(countConditions, fmt.Sprintf("tags @> $%d::jsonb", countArgIndex))
			countArgsTemp = append(countArgsTemp, mergedTags)
			countArgIndex++
		}

		// Prefix tags: each prefix's expanded tags are OR (using ?| operator)
		for _, expandedTags := range expandedTagGroups {
			if len(expandedTags) == 0 {
				continue
			}

			countConditions = append(countConditions, fmt.Sprintf("tags ?| $%d", countArgIndex))
			countArgsTemp = append(countArgsTemp, expandedTags)
			countArgIndex++
		}

		// Rebuild title conditions for count
		var titleConditionsCount []string
		for _, phrase := range searchQuery.Phrases {
			titleConditionsCount = append(titleConditionsCount, fmt.Sprintf(
				"(title ILIKE $%d OR title_jpn ILIKE $%d)",
				countArgIndex, countArgIndex+1,
			))
			phrasePattern := "%" + phrase + "%"
			countArgsTemp = append(countArgsTemp, phrasePattern, phrasePattern)
			countArgIndex += 2
		}
		for _, kw := range searchQuery.Keywords {
			titleConditionsCount = append(titleConditionsCount, fmt.Sprintf(
				"(title ILIKE $%d OR title_jpn ILIKE $%d)",
				countArgIndex, countArgIndex+1,
			))
			kwPattern := "%" + kw + "%"
			countArgsTemp = append(countArgsTemp, kwPattern, kwPattern)
			countArgIndex += 2
		}
		for _, wildcard := range searchQuery.Wildcards {
			titleConditionsCount = append(titleConditionsCount, fmt.Sprintf(
				"(title ILIKE $%d OR title_jpn ILIKE $%d)",
				countArgIndex, countArgIndex+1,
			))
			countArgsTemp = append(countArgsTemp, wildcard, wildcard)
			countArgIndex += 2
		}
		for _, exclude := range searchQuery.Excludes {
			// Check if this is a tag exclusion
			if strings.HasPrefix(exclude, "TAG_EXACT:") {
				// Exact tag exclusion: NOT (tags ? 'tag')
				tagValue := strings.TrimPrefix(exclude, "TAG_EXACT:")
				countConditions = append(countConditions, fmt.Sprintf("NOT (tags ? $%d)", countArgIndex))
				countArgsTemp = append(countArgsTemp, tagValue)
				countArgIndex++
			} else if strings.HasPrefix(exclude, "TAG_PREFIX:") {
				// Tag prefix exclusion: expand and use NOT (tags ?| array[...])
				tagPrefix := strings.TrimPrefix(exclude, "TAG_PREFIX:")
				expandedTags := h.expandSingleTagPrefix(ctx, tagPrefix)
				if len(expandedTags) > 0 {
					countConditions = append(countConditions, fmt.Sprintf("NOT (tags ?| $%d)", countArgIndex))
					countArgsTemp = append(countArgsTemp, expandedTags)
					countArgIndex++
				}
			} else {
				// Regular title exclusion
				titleConditionsCount = append(titleConditionsCount, fmt.Sprintf(
					"(title NOT ILIKE $%d AND title_jpn NOT ILIKE $%d)",
					countArgIndex, countArgIndex+1,
				))
				excludePattern := "%" + exclude + "%"
				countArgsTemp = append(countArgsTemp, excludePattern, excludePattern)
				countArgIndex += 2
			}
		}
		for _, orGroup := range searchQuery.OrGroups {
			var orConditions []string
			var tagOrConditions []string

			for _, orTerm := range orGroup {
				// Check if this is a tag OR
				if strings.HasPrefix(orTerm, "TAG_EXACT:") {
					// Exact tag OR: tags ? 'tag'
					tagValue := strings.TrimPrefix(orTerm, "TAG_EXACT:")
					tagOrConditions = append(tagOrConditions, fmt.Sprintf("(tags ? $%d)", countArgIndex))
					countArgsTemp = append(countArgsTemp, tagValue)
					countArgIndex++
				} else if strings.HasPrefix(orTerm, "TAG_PREFIX:") {
					// Tag prefix OR: expand and use tags ?| array[...]
					tagPrefix := strings.TrimPrefix(orTerm, "TAG_PREFIX:")
					expandedTags := h.expandSingleTagPrefix(ctx, tagPrefix)
					if len(expandedTags) > 0 {
						tagOrConditions = append(tagOrConditions, fmt.Sprintf("(tags ?| $%d)", countArgIndex))
						countArgsTemp = append(countArgsTemp, expandedTags)
						countArgIndex++
					}
				} else {
					// Regular title OR
					orConditions = append(orConditions, fmt.Sprintf(
						"(title ILIKE $%d OR title_jpn ILIKE $%d)",
						countArgIndex, countArgIndex+1,
					))
					orPattern := "%" + orTerm + "%"
					countArgsTemp = append(countArgsTemp, orPattern, orPattern)
					countArgIndex += 2
				}
			}

			// Combine tag OR conditions with title OR conditions
			allOrConditions := append(tagOrConditions, orConditions...)
			if len(allOrConditions) > 0 {
				if len(tagOrConditions) > 0 && len(orConditions) > 0 {
					// Mixed: add to main conditions
					countConditions = append(countConditions, "("+strings.Join(allOrConditions, " OR ")+")")
				} else if len(tagOrConditions) > 0 {
					// Only tag conditions: add to main conditions
					countConditions = append(countConditions, "("+strings.Join(tagOrConditions, " OR ")+")")
				} else {
					// Only title conditions: add to title conditions
					titleConditionsCount = append(titleConditionsCount, "("+strings.Join(orConditions, " OR ")+")")
				}
			}
		}

		if len(titleConditionsCount) > 0 {
			countConditions = append(countConditions, "("+strings.Join(titleConditionsCount, " AND ")+")")
		}

		countWhereClause = "WHERE " + strings.Join(countConditions, " AND ")
		if len(countConditions) == 0 {
			countWhereClause = ""
		}
		countArgs = countArgsTemp
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM gallery %s", countWhereClause)
	h.logger.Debug("executing count query",
		zap.String("sql", utils.FormatSQL(countQuery, countArgs...)),
	)

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

	// Include next_cursor in response
	lastGallery := galleries[len(galleries)-1]
	lastPosted := lastGallery.Posted.Unix()
	lastGid := lastGallery.Gid
	nextCursor := fmt.Sprintf("%d,%d", lastPosted, lastGid)
	c.JSON(200, utils.GetResponseWithCursor(galleries, 200, "success", &total, &nextCursor))
}

// expandTagPrefixesGrouped queries tag table and returns grouped results
// Returns: map[prefix][]tags and hasUnmatchedPrefixes flag
func (h *SearchHandler) expandTagPrefixesGrouped(ctx context.Context, prefixes []string) (map[string][]string, bool) {
	if len(prefixes) == 0 {
		return make(map[string][]string), false
	}

	pool := database.GetPool()
	result := make(map[string][]string)
	hasUnmatched := false

	for _, prefix := range prefixes {
		// Query tag table for tags starting with the prefix
		query := `
			SELECT name
			FROM tag
			WHERE name LIKE $1
		`
		pattern := prefix + "%"

		h.logger.Debug("expanding tag prefix",
			zap.String("prefix", prefix),
			zap.String("pattern", pattern),
		)

		rows, err := pool.Query(ctx, query, pattern)
		if err != nil {
			h.logger.Error("failed to query tags", zap.Error(err))
			hasUnmatched = true
			continue
		}

		var tags []string
		for rows.Next() {
			var tagName string
			if err := rows.Scan(&tagName); err != nil {
				h.logger.Error("failed to scan tag", zap.Error(err))
				continue
			}
			tags = append(tags, tagName)
		}
		rows.Close()

		if len(tags) == 0 {
			h.logger.Debug("no tags matched prefix", zap.String("prefix", prefix))
			hasUnmatched = true
		} else {
			result[prefix] = tags
			h.logger.Debug("expanded tag prefix",
				zap.String("prefix", prefix),
				zap.Int("matches", len(tags)),
			)
		}
	}

	return result, hasUnmatched
}

// expandSingleTagPrefix expands a single tag prefix and returns matching tags
func (h *SearchHandler) expandSingleTagPrefix(ctx context.Context, prefix string) []string {
	pool := database.GetPool()

	// Query tag table for tags starting with the prefix
	query := `
		SELECT name
		FROM tag
		WHERE name LIKE $1
	`
	pattern := prefix + "%"

	h.logger.Debug("expanding single tag prefix",
		zap.String("prefix", prefix),
		zap.String("pattern", pattern),
	)

	rows, err := pool.Query(ctx, query, pattern)
	if err != nil {
		h.logger.Error("failed to query tags", zap.Error(err))
		return []string{}
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tagName string
		if err := rows.Scan(&tagName); err != nil {
			h.logger.Error("failed to scan tag", zap.Error(err))
			continue
		}
		tags = append(tags, tagName)
	}

	h.logger.Debug("expanded single tag prefix",
		zap.String("prefix", prefix),
		zap.Int("matches", len(tags)),
	)

	return tags
}
