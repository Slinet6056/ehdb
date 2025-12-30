package crawler

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries int
	Logger     *zap.Logger
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
