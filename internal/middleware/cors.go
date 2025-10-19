package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS returns a middleware for CORS support
func CORS(corsEnabled bool, corsOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if corsEnabled {
			origin := corsOrigin
			if origin == "" {
				origin = "*"
			}

			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
		}

		c.Next()
	}
}
