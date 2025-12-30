package crawler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries     int
	Logger         *zap.Logger
	WaitForIPUnban bool // Whether to wait when IP is temporarily banned
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
	var lastErr error

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3 // fallback default
	}

	for i := 0; i < maxRetries; i++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if this is an IP ban error
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

				// Reset retry counter since this is an IP ban, not a real failure
				i = -1
				continue
			}
		}

		if cfg.Logger != nil {
			cfg.Logger.Warn("operation failed, retrying",
				zap.Int("attempt", i+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err),
			)
		}

		// Don't sleep after the last attempt
		if i < maxRetries-1 {
			// Exponential backoff: 5s, 10s, 15s...
			sleepDuration := time.Duration((i+1)*5) * time.Second
			time.Sleep(sleepDuration)
		}
	}

	var zero T
	return zero, fmt.Errorf("exceeded max retries (%d): %w", maxRetries, lastErr)
}

// RetryVoid executes a function that returns only an error with exponential backoff retry logic
func RetryVoid(cfg RetryConfig, fn func() error) error {
	var lastErr error

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3 // fallback default
	}

	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if this is an IP ban error
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

				// Reset retry counter since this is an IP ban, not a real failure
				i = -1
				continue
			}
		}

		if cfg.Logger != nil {
			cfg.Logger.Warn("operation failed, retrying",
				zap.Int("attempt", i+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err),
			)
		}

		// Don't sleep after the last attempt
		if i < maxRetries-1 {
			// Exponential backoff: 5s, 10s, 15s...
			sleepDuration := time.Duration((i+1)*5) * time.Second
			time.Sleep(sleepDuration)
		}
	}

	return fmt.Errorf("exceeded max retries (%d): %w", maxRetries, lastErr)
}
