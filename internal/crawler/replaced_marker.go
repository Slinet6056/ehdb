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

	query := `
		WITH latest_versions AS (
			SELECT COALESCE(root_gid, gid) AS group_id, MAX(gid) AS max_gid
			FROM gallery
			GROUP BY COALESCE(root_gid, gid)
			HAVING COUNT(*) > 1
		)
		UPDATE gallery g
		SET replaced = (g.gid != lv.max_gid)
		FROM latest_versions lv
		WHERE COALESCE(g.root_gid, g.gid) = lv.group_id
		  AND g.replaced != (g.gid != lv.max_gid)
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
