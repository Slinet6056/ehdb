package crawler

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildBackfillResumeWindow(t *testing.T) {
	window := backfillWindow{
		startPosted: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		endPosted:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC).Unix(),
	}

	items := []GalleryListItem{
		{Gid: "3", Posted: "2026-03-30 12:00"},
		{Gid: "2", Posted: "2026-03-15 08:30"},
		{Gid: "1", Posted: "2026-02-01 04:15"},
	}

	resumeStart, resumeEnd, ok := buildBackfillResumeWindow(window, items, func(value string) (int64, error) {
		parsed, err := time.Parse("2006-01-02 15:04", value)
		if err != nil {
			return 0, err
		}

		return parsed.UTC().Unix(), nil
	})
	if !ok {
		t.Fatal("expected resumable window")
	}

	if !resumeStart.Equal(time.Unix(window.startPosted, 0).UTC()) {
		t.Fatalf("unexpected resume start: got %s", resumeStart)
	}

	wantEnd := time.Date(2026, 2, 1, 4, 15, 0, 0, time.UTC)
	if !resumeEnd.Equal(wantEnd) {
		t.Fatalf("unexpected resume end: got %s want %s", resumeEnd, wantEnd)
	}
}

func TestBuildBackfillResumeWindowRejectsInvalidRange(t *testing.T) {
	window := backfillWindow{
		startPosted: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).Unix(),
		endPosted:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC).Unix(),
	}

	items := []GalleryListItem{{Gid: "1", Posted: "2026-02-01 00:00"}}
	_, _, ok := buildBackfillResumeWindow(window, items, func(value string) (int64, error) {
		parsed, err := time.Parse("2006-01-02 15:04", value)
		if err != nil {
			return 0, err
		}

		return parsed.UTC().Unix(), nil
	})
	if ok {
		t.Fatal("expected non-resumable window when resume range is empty")
	}
}

func TestPartialBackfillError(t *testing.T) {
	cause := errors.New("fetch normal pages: exceeded max retries")
	err := &PartialBackfillError{
		Cause:           cause,
		ImportedCount:   12,
		DiscoveredCount: 20,
		MissingCount:    14,
		ResumeStart:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ResumeEnd:       time.Date(2026, 2, 1, 4, 15, 0, 0, time.UTC),
	}

	message := err.Error()
	checks := []string{
		"partial backfill interrupted",
		"importing 12 of 14 missing galleries",
		"2026-01-01T00:00:00Z",
		"2026-02-01T04:15:00Z",
		cause.Error(),
	}
	for _, check := range checks {
		if !strings.Contains(message, check) {
			t.Fatalf("expected error message to contain %q, got %q", check, message)
		}
	}

	if !errors.Is(err, cause) {
		t.Fatal("expected partial backfill error to unwrap cause")
	}
}

func TestPartialBackfillErrorAllowsZeroMissingCount(t *testing.T) {
	cause := errors.New("fetch expunged pages: exceeded max retries")
	err := &PartialBackfillError{
		Cause:           cause,
		ImportedCount:   0,
		DiscoveredCount: 8,
		MissingCount:    0,
		ResumeStart:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ResumeEnd:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	message := err.Error()
	checks := []string{
		"importing 0 of 0 missing galleries",
		"rerun overlapping window",
		"2026-01-15T00:00:00Z",
	}
	for _, check := range checks {
		if !strings.Contains(message, check) {
			t.Fatalf("expected error message to contain %q, got %q", check, message)
		}
	}
}
