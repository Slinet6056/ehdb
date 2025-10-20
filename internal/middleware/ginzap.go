package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinZap returns a gin.HandlerFunc middleware that logs requests using zap
func GinZap(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Skip logging for health check endpoint
		if path == "/health" {
			c.Next()
			return
		}

		// Process request
		c.Next()

		// Calculate request duration
		latency := time.Since(start)

		// Get status code
		status := c.Writer.Status()

		// Build log message similar to Gin's default format
		msg := fmt.Sprintf("[GIN] %3d | %13v | %15s | %-7s %s",
			status,
			latency,
			c.ClientIP(),
			c.Request.Method,
			path,
		)

		// Add query string if present
		if query != "" {
			msg += "?" + query
		}

		// Log with appropriate level based on status code
		// Only add essential fields for error debugging
		if len(c.Errors) > 0 {
			// Log errors with more details
			for _, e := range c.Errors {
				logger.Error(e.Error(),
					zap.Int("status", status),
					zap.Duration("latency", latency),
					zap.String("path", path),
					zap.String("method", c.Request.Method),
					zap.String("ip", c.ClientIP()),
				)
			}
		} else {
			switch {
			case status >= 500:
				logger.Error(msg,
					zap.String("path", path),
					zap.String("ip", c.ClientIP()),
					zap.String("user_agent", c.Request.UserAgent()),
				)
			case status >= 400:
				logger.Warn(msg)
			default:
				logger.Info(msg)
			}
		}
	}
}
