package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/slinet/ehdb/pkg/utils"
	"go.uber.org/zap"
)

// ErrorHandler returns a middleware for error handling
func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there are any errors
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			logger.Error("request error",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.Error(err),
			)

			// Return error response
			c.JSON(500, utils.GetResponse(nil, 500, "Internal server error", nil))
		}
	}
}

// Recovery returns a middleware for panic recovery
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
				)

				c.JSON(500, utils.GetResponse(nil, 500, "Internal server error", nil))
				c.Abort()
			}
		}()

		c.Next()
	}
}
