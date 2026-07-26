package crawler

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// defaultTransientRetries is the fallback retry budget dedicated to transient
// upstream errors (HTTP 5xx / 429) when none is configured.
const defaultTransientRetries = 6

// transientBaseBackoff and transientMaxBackoff control the exponential backoff
// applied to transient errors. They are vars (not consts) so tests can shrink
// them to keep retry loops fast.
var (
	// transientBaseBackoff is the first backoff step for transient errors.
	transientBaseBackoff = 10 * time.Second
	// transientMaxBackoff caps a single transient backoff wait.
	transientMaxBackoff = 2 * time.Minute
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries int
	// TransientRetryTimes is a separate retry budget for transient upstream
	// errors (HTTP 5xx / 429). These errors are usually short-lived overload or
	// rate limiting, so they get more attempts with exponential backoff rather
	// than consuming the general MaxRetries budget. Falls back to
	// defaultTransientRetries (and never drops below MaxRetries) when unset.
	TransientRetryTimes int
	Logger              *zap.Logger
	WaitForIPUnban      bool // Whether to wait when IP is temporarily banned
}

// transientBackoff computes how long to wait before the next attempt for a
// transient error on the given zero-based transient attempt index. It uses
// exponential backoff with jitter, and honors the server's Retry-After hint
// when it asks for a longer wait. The result is capped at transientMaxBackoff.
func transientBackoff(attempt int, retryAfter time.Duration) time.Duration {
	backoff := transientBaseBackoff << attempt
	if backoff <= 0 || backoff > transientMaxBackoff {
		backoff = transientMaxBackoff
	}

	// Full jitter within +/-20% to avoid lockstep retries against the server.
	jitterSpan := int64(backoff) / 5
	if jitterSpan > 0 {
		backoff += time.Duration(rand.Int63n(2*jitterSpan+1) - jitterSpan)
	}

	if retryAfter > backoff {
		backoff = retryAfter
	}

	if backoff > transientMaxBackoff {
		backoff = transientMaxBackoff
	}

	return backoff
}

// parseIPBanDuration parses the remaining time of an IP ban
// Supports formats like: "59 minutes and 43 seconds", "1 hour and 30 minutes", "45 seconds"
func parseIPBanDuration(errMsg string) (time.Duration, bool) {
	// Check if the message contains ban information
	if !strings.Contains(errMsg, "temporarily banned") {
		return 0, false
	}

	// Extract "The ban expires in X" part
	banPattern := regexp.MustCompile(`ban expires in (.+?)\)`)
	matches := banPattern.FindStringSubmatch(errMsg)
	if len(matches) < 2 {
		return 0, false
	}

	durationStr := matches[1]
	var totalDuration time.Duration

	// Parse hours
	hourPattern := regexp.MustCompile(`(\d+)\s+hour`)
	if hourMatch := hourPattern.FindStringSubmatch(durationStr); len(hourMatch) >= 2 {
		if hours, err := strconv.Atoi(hourMatch[1]); err == nil {
			totalDuration += time.Duration(hours) * time.Hour
		}
	}

	// Parse minutes
	minutePattern := regexp.MustCompile(`(\d+)\s+minute`)
	if minuteMatch := minutePattern.FindStringSubmatch(durationStr); len(minuteMatch) >= 2 {
		if minutes, err := strconv.Atoi(minuteMatch[1]); err == nil {
			totalDuration += time.Duration(minutes) * time.Minute
		}
	}

	// Parse seconds
	secondPattern := regexp.MustCompile(`(\d+)\s+second`)
	if secondMatch := secondPattern.FindStringSubmatch(durationStr); len(secondMatch) >= 2 {
		if seconds, err := strconv.Atoi(secondMatch[1]); err == nil {
			totalDuration += time.Duration(seconds) * time.Second
		}
	}

	if totalDuration > 0 {
		return totalDuration, true
	}

	return 0, false
}

// Retry executes a function with exponential backoff retry logic
// Returns the result and error from the function
func Retry[T any](cfg RetryConfig, fn func() (T, error)) (T, error) {
	result, err := runWithRetry(cfg, func() (any, error) {
		return fn()
	})
	if err != nil {
		var zero T
		return zero, err
	}

	// runWithRetry only returns a nil error alongside a value produced by fn on
	// success, so the assertion is always safe here.
	value, _ := result.(T)
	return value, nil
}

// RetryVoid executes a function that returns only an error with exponential backoff retry logic
func RetryVoid(cfg RetryConfig, fn func() error) error {
	_, err := runWithRetry(cfg, func() (any, error) {
		return nil, fn()
	})
	return err
}

// runWithRetry drives the retry loop shared by Retry and RetryVoid.
//
// It maintains two independent budgets:
//   - a general budget (MaxRetries) with linear backoff for ordinary failures
//     such as parse errors or abnormal pages;
//   - a transient budget (TransientRetryTimes) with exponential backoff + jitter
//     for temporary upstream errors (HTTP 5xx / 429), honoring Retry-After.
//
// Auth failures abort immediately, and IP bans wait for the reported unban
// window without consuming either budget (matching the previous behavior).
func runWithRetry(cfg RetryConfig, fn func() (any, error)) (any, error) {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3 // fallback default
	}

	transientMax := cfg.TransientRetryTimes
	if transientMax <= 0 {
		transientMax = defaultTransientRetries
	}
	// A transient error should never get fewer attempts than an ordinary one.
	if transientMax < maxRetries {
		transientMax = maxRetries
	}

	var lastErr error
	normalAttempts := 0
	transientAttempts := 0

	for {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		if errors.Is(err, ErrAuthRequired) {
			if cfg.Logger != nil {
				cfg.Logger.Error("auth failure detected, aborting retries", zap.Error(err))
			}
			return nil, fmt.Errorf("authentication failure: %w", err)
		}

		// IP bans wait for the reported window and do not consume any retry budget.
		if cfg.WaitForIPUnban {
			if duration, isIPBan := parseIPBanDuration(err.Error()); isIPBan {
				if cfg.Logger != nil {
					cfg.Logger.Warn("IP temporarily banned, waiting for unban",
						zap.Duration("wait_duration", duration),
						zap.String("unban_time", time.Now().Add(duration).Format("2006-01-02 15:04:05")),
					)
				}

				// Wait for ban to expire, plus 10 extra seconds to ensure complete unban
				time.Sleep(duration + 10*time.Second)

				if cfg.Logger != nil {
					cfg.Logger.Info("IP ban wait completed, retrying")
				}
				continue
			}
		}

		// Transient upstream errors get their own larger budget and exponential backoff.
		if te, ok := asTransientError(err); ok {
			if transientAttempts >= transientMax-1 {
				return nil, fmt.Errorf("exceeded max transient retries (%d): %w", transientMax, lastErr)
			}

			sleepDuration := transientBackoff(transientAttempts, te.RetryAfter)
			if cfg.Logger != nil {
				cfg.Logger.Warn("transient upstream error, backing off",
					zap.Int("attempt", transientAttempts+1),
					zap.Int("max_retries", transientMax),
					zap.Int("status_code", te.StatusCode),
					zap.Duration("backoff", sleepDuration),
					zap.Error(err),
				)
			}
			transientAttempts++
			time.Sleep(sleepDuration)
			continue
		}

		if normalAttempts >= maxRetries-1 {
			return nil, fmt.Errorf("exceeded max retries (%d): %w", maxRetries, lastErr)
		}

		if cfg.Logger != nil {
			cfg.Logger.Warn("operation failed, retrying",
				zap.Int("attempt", normalAttempts+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err),
			)
		}

		// Linear backoff: 5s, 10s, 15s...
		sleepDuration := time.Duration((normalAttempts+1)*5) * time.Second
		normalAttempts++
		time.Sleep(sleepDuration)
	}
}
