package crawler

import (
	"context"
	"fmt"

	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

// ReplacedMarker marks replaced galleries
type ReplacedMarker struct {
	logger *zap.Logger
}

// NewReplacedMarker creates a new replaced marker
func NewReplacedMarker(logger *zap.Logger) *ReplacedMarker {
	return &ReplacedMarker{logger: logger}
}

// MarkReplaced marks all replaced galleries
// A gallery is marked as replaced if it has a root_gid and it's not the latest version
func (rm *ReplacedMarker) MarkReplaced(ctx context.Context) error {
	rm.logger.Info("starting to mark replaced galleries")

	pool := database.GetPool()

	// SQL logic from markreplaced.js:
	// UPDATE gallery LEFT JOIN (SELECT root_gid, MAX(gid) AS max_gid, gid FROM gallery GROUP BY IFNULL(root_gid, gid)) AS t
	// ON gallery.gid = t.max_gid SET gallery.replaced = t.max_gid IS NULL

	// PostgreSQL equivalent:
	query := `
		UPDATE gallery
		SET replaced = (
			CASE
				WHEN gid IN (
					SELECT MAX(gid)
					FROM gallery
					GROUP BY COALESCE(root_gid, gid)
				) THEN false
				ELSE true
			END
		)
		WHERE root_gid IS NOT NULL
	`

	rm.logger.Debug("executing update query",
		zap.String("sql", utils.FormatSQL(query)),
	)

	result, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("mark replaced: %w", err)
	}

	rowsAffected := result.RowsAffected()
	rm.logger.Info("mark replaced completed", zap.Int64("rows_affected", rowsAffected))

	return nil
}
