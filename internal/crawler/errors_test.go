package crawler

import (
	"errors"
	"fmt"
	"testing"
	"time"
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

// withFastBackoff shrinks the transient backoff durations for the duration of a
// test so retry loops don't sleep for real seconds.
func withFastBackoff(t *testing.T) {
	t.Helper()
	base, max := transientBaseBackoff, transientMaxBackoff
	transientBaseBackoff = time.Millisecond
	transientMaxBackoff = 5 * time.Millisecond
	t.Cleanup(func() {
		transientBaseBackoff = base
		transientMaxBackoff = max
	})
}

func TestRetryUsesTransientBudget(t *testing.T) {
	withFastBackoff(t)
	attempts := 0

	_, err := Retry(RetryConfig{MaxRetries: 2, TransientRetryTimes: 4}, func() (int, error) {
		attempts++
		return 0, fmt.Errorf("fetch page: %w", &TransientError{StatusCode: 503})
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should attempt exactly TransientRetryTimes times, not MaxRetries.
	if attempts != 4 {
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}

	var te *TransientError
	if !errors.As(err, &te) {
		t.Fatalf("expected wrapped TransientError, got %v", err)
	}
}

func TestRetryTransientThenSuccess(t *testing.T) {
	withFastBackoff(t)
	attempts := 0

	got, err := Retry(RetryConfig{MaxRetries: 1, TransientRetryTimes: 3}, func() (int, error) {
		attempts++
		if attempts < 2 {
			return 0, fmt.Errorf("fetch page: %w", &TransientError{StatusCode: 502})
		}
		return 42, nil
	})

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryNormalErrorKeepsSmallBudget(t *testing.T) {
	attempts := 0

	err := RetryVoid(RetryConfig{MaxRetries: 1, TransientRetryTimes: 6}, func() error {
		attempts++
		return fmt.Errorf("parse failure")
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Ordinary errors must not borrow the larger transient budget: a bug that
	// routed them through it would yield 6 attempts instead of 1.
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestIsTransientStatusCode(t *testing.T) {
	transient := []int{429, 500, 502, 503, 504}
	for _, code := range transient {
		if !isTransientStatusCode(code) {
			t.Fatalf("expected %d to be transient", code)
		}
	}

	notTransient := []int{200, 301, 400, 401, 403, 404}
	for _, code := range notTransient {
		if isTransientStatusCode(code) {
			t.Fatalf("expected %d to not be transient", code)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("30"); d != 30*time.Second {
		t.Fatalf("expected 30s, got %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Fatalf("expected 0 for empty, got %v", d)
	}
	if d := parseRetryAfter("0"); d != 0 {
		t.Fatalf("expected 0 for zero seconds, got %v", d)
	}
	if d := parseRetryAfter("not-a-number"); d != 0 {
		t.Fatalf("expected 0 for garbage, got %v", d)
	}
}

func TestIsTemporaryBanError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantHit bool
	}{
		{
			name:    "temporary ban message",
			err:     errors.New("torrent page abnormal: Your IP address has been temporarily banned. (The ban expires in 4 minutes and 58 seconds): abnormal page response"),
			wantHit: true,
		},
		{
			name:    "cloudflare challenge only",
			err:     errors.New("torrent page abnormal: cloudflare: abnormal page response"),
			wantHit: false,
		},
		{
			name:    "nil error",
			err:     nil,
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTemporaryBanError(tt.err)
			if got != tt.wantHit {
				t.Fatalf("unexpected temporary ban result: got %v want %v", got, tt.wantHit)
			}
		})
	}
}
