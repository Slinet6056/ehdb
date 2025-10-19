package handler

import (
	"context"
	"fmt"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

type GalleryHandler struct {
	logger *zap.Logger
}

func NewGalleryHandler(logger *zap.Logger) *GalleryHandler {
	return &GalleryHandler{logger: logger}
}

// GetGallery handles GET /api/gallery/:gid/:token and GET /api/g/:gid/:token
func (h *GalleryHandler) GetGallery(c *gin.Context) {
	gid := c.Param("gid")
	token := c.Param("token")

	// Also check query parameters
	if gid == "" {
		gid = c.Query("gid")
	}
	if token == "" {
		token = c.Query("token")
	}

	// Validate gid and token
	gidPattern := regexp.MustCompile(`^\d+$`)
	tokenPattern := regexp.MustCompile(`^[0-9a-f]{10}$`)

	if !gidPattern.MatchString(gid) || !tokenPattern.MatchString(token) {
		c.JSON(400, utils.GetResponse(nil, 400, "gid or token is invalid", nil))
		return
	}

	ctx := context.Background()
	pool := database.GetPool()

	// Query gallery
	query := `
		SELECT gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
		       posted, filecount, filesize, expunged, removed, replaced, rating,
		       torrentcount, root_gid, bytorrent, COALESCE(tags, '[]'::jsonb)
		FROM gallery
		WHERE gid = $1 AND token = $2
	`

	h.logger.Debug("executing gallery query",
		zap.String("sql", utils.FormatSQL(query, gid, token)),
	)

	var gallery database.Gallery
	err := pool.QueryRow(ctx, query, gid, token).Scan(
		&gallery.Gid, &gallery.Token, &gallery.ArchiverKey, &gallery.Title,
		&gallery.TitleJpn, &gallery.Category, &gallery.Thumb, &gallery.Uploader,
		&gallery.Posted, &gallery.Filecount, &gallery.Filesize, &gallery.Expunged,
		&gallery.Removed, &gallery.Replaced, &gallery.Rating, &gallery.Torrentcount,
		&gallery.RootGid, &gallery.Bytorrent, &gallery.Tags,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			h.logger.Debug("gallery not found", zap.String("gid", gid), zap.String("token", token))
			c.JSON(404, utils.GetResponse(nil, 404, "no gallery matches gid and token", nil))
			return
		}
		h.logger.Error("failed to query gallery", zap.Error(err), zap.String("gid", gid), zap.String("token", token))
		c.JSON(500, utils.GetResponse(nil, 500, "database error", nil))
		return
	}

	h.logger.Debug("gallery found",
		zap.String("gid", gid),
		zap.String("token", token),
		zap.String("category", gallery.Category),
	)

	// Query torrents if root_gid exists
	gallery.Torrents = []database.Torrent{}
	if gallery.RootGid != nil {
		torrents, err := h.queryTorrents(ctx, *gallery.RootGid)
		if err != nil {
			h.logger.Error("failed to query torrents", zap.Error(err))
			// Don't fail the request, just return empty torrents
		} else {
			gallery.Torrents = torrents
		}
	}

	c.JSON(200, utils.GetResponse(gallery, 200, "success", nil))
}

// queryTorrents queries torrents for a given root_gid
func (h *GalleryHandler) queryTorrents(ctx context.Context, rootGid int) ([]database.Torrent, error) {
	pool := database.GetPool()
	query := `
		SELECT id, gid, name, hash, addedstr, fsizestr, uploader, expunged
		FROM torrent
		WHERE gid = $1
		ORDER BY id
	`

	h.logger.Debug("executing torrent query",
		zap.String("sql", utils.FormatSQL(query, rootGid)),
		zap.Int("root_gid", rootGid),
	)

	rows, err := pool.Query(ctx, query, rootGid)
	if err != nil {
		h.logger.Error("failed to query torrents", zap.Error(err), zap.Int("root_gid", rootGid))
		return nil, fmt.Errorf("query torrents: %w", err)
	}
	defer rows.Close()

	var torrents []database.Torrent
	for rows.Next() {
		var t database.Torrent
		err := rows.Scan(&t.ID, &t.Gid, &t.Name, &t.Hash, &t.Addedstr, &t.Fsizestr, &t.Uploader, &t.Expunged)
		if err != nil {
			h.logger.Error("failed to scan torrent", zap.Error(err))
			return nil, fmt.Errorf("scan torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	h.logger.Debug("torrents found",
		zap.Int("root_gid", rootGid),
		zap.Int("torrent_count", len(torrents)),
	)

	return torrents, nil
}
