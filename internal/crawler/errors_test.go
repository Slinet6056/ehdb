package crawler

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsAuthFailureBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantHit bool
	}{
		{
			name:    "redirect marker",
			body:    "Please stand by while we redirect you...",
			wantHit: true,
		},
		{
			name:    "privileges marker",
			body:    "You do not have sufficient privileges to access this page.",
			wantHit: true,
		},
		{
			name:    "sad panda marker",
			body:    "Sad Panda",
			wantHit: true,
		},
		{
			name:    "normal gallery page",
			body:    "<html><title>E-Hentai Galleries</title></html>",
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := isAuthFailureBody([]byte(tt.body))
			if ok != tt.wantHit {
				t.Fatalf("unexpected marker result: got %v want %v", ok, tt.wantHit)
			}
		})
	}
}

func TestRetryAbortOnAuthFailure(t *testing.T) {
	attempts := 0

	_, err := Retry(RetryConfig{MaxRetries: 3}, func() (int, error) {
		attempts++
		return 0, fmt.Errorf("request denied: %w", ErrAuthRequired)
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected auth error, got %v", err)
	}

	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryVoidAbortOnAuthFailure(t *testing.T) {
	attempts := 0

	err := RetryVoid(RetryConfig{MaxRetries: 3}, func() error {
		attempts++
		return fmt.Errorf("request denied: %w", ErrAuthRequired)
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected auth error, got %v", err)
	}

	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestAbnormalGalleryListPageReason(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantOk bool
	}{
		{
			name:   "temporary ban marker",
			body:   "Your IP address has been temporarily banned. (The ban expires in 4 minutes and 58 seconds)",
			wantOk: true,
		},
		{
			name:   "blank page",
			body:   "   \n\t  ",
			wantOk: true,
		},
		{
			name:   "challenge page",
			body:   "<html><title>Just a moment</title><body>Checking your browser before accessing</body></html>",
			wantOk: true,
		},
		{
			name:   "normal list page",
			body:   `<html><body><div class="searchnav"></div><script>var nexturl="https://e-hentai.org/?next=123";</script><span class="posted_foo">2026-03-30 12:00</span></body></html>`,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := abnormalGalleryListPageReason([]byte(tt.body))
			if ok != tt.wantOk {
				t.Fatalf("unexpected abnormal result: got %v want %v", ok, tt.wantOk)
			}
		})
	}
}
