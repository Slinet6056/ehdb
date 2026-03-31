package main

import (
	"testing"
	"time"
)

func TestResolveBackfillWindow(t *testing.T) {
	fixedNow := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	originalNowFunc := nowFunc
	nowFunc = func() time.Time {
		return fixedNow
	}
	defer func() {
		nowFunc = originalNowFunc
	}()

	tests := []struct {
		name        string
		startRaw    string
		endRaw      string
		offsetHours int
		wantStart   time.Time
		wantEnd     time.Time
		wantErr     string
	}{
		{
			name:        "offset uses current time as end",
			offsetHours: 24,
			wantStart:   fixedNow.Add(-24 * time.Hour),
			wantEnd:     fixedNow,
		},
		{
			name:      "start without end defaults to current time",
			startRaw:  "2026-03-01T00:00:00Z",
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   fixedNow,
		},
		{
			name:      "explicit start and end still work",
			startRaw:  "2026-03-01",
			endRaw:    "2026-03-02",
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 3, 2, 23, 59, 59, 0, time.UTC),
		},
		{
			name:    "end without start returns error",
			endRaw:  "2026-03-02T00:00:00Z",
			wantErr: "-end cannot be used without -start",
		},
		{
			name:    "missing all window args returns error",
			wantErr: "either -offset or -start must be provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd, err := resolveBackfillWindow(tt.startRaw, tt.endRaw, tt.offsetHours)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveBackfillWindow returned error: %v", err)
			}

			if !gotStart.Equal(tt.wantStart) {
				t.Fatalf("expected start %s, got %s", tt.wantStart.Format(time.RFC3339), gotStart.Format(time.RFC3339))
			}

			if !gotEnd.Equal(tt.wantEnd) {
				t.Fatalf("expected end %s, got %s", tt.wantEnd.Format(time.RFC3339), gotEnd.Format(time.RFC3339))
			}
		})
	}
}
