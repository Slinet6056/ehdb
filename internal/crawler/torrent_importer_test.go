package crawler

import (
	"strings"
	"testing"
	"time"

	"github.com/slinet/ehdb/internal/config"
)

func TestBuildTorrentImportGalleryQuery(t *testing.T) {
	t.Run("without window uses unbounded query", func(t *testing.T) {
		query, args := buildTorrentImportGalleryQuery(&config.CrawlerConfig{})

		if len(args) != 0 {
			t.Fatalf("expected no query args, got %d", len(args))
		}

		if strings.Contains(query, "posted >= $1") || strings.Contains(query, "posted <= $2") {
			t.Fatalf("expected query without posted window filters, got %q", query)
		}
	})

	t.Run("with window adds posted bounds", func(t *testing.T) {
		cfg := &config.CrawlerConfig{
			BackfillStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
			BackfillEnd:   time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC).Unix(),
		}

		query, args := buildTorrentImportGalleryQuery(cfg)

		if !strings.Contains(query, "posted >= $1") || !strings.Contains(query, "posted <= $2") {
			t.Fatalf("expected query with posted window filters, got %q", query)
		}

		if len(args) != 2 {
			t.Fatalf("expected 2 query args, got %d", len(args))
		}

		startTime, ok := args[0].(time.Time)
		if !ok {
			t.Fatalf("expected first arg to be time.Time, got %T", args[0])
		}
		if !startTime.Equal(time.Unix(cfg.BackfillStart, 0).UTC()) {
			t.Fatalf("expected start arg %s, got %s", time.Unix(cfg.BackfillStart, 0).UTC(), startTime)
		}

		endTime, ok := args[1].(time.Time)
		if !ok {
			t.Fatalf("expected second arg to be time.Time, got %T", args[1])
		}
		if !endTime.Equal(time.Unix(cfg.BackfillEnd, 0).UTC()) {
			t.Fatalf("expected end arg %s, got %s", time.Unix(cfg.BackfillEnd, 0).UTC(), endTime)
		}
	})
}
