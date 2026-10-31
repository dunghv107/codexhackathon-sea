package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS returns CORS middleware
func (m *Middleware) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if m.allowAll {
			if origin != "" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "false")
			}
		} else if origin != "" {
			for _, o := range m.allowedOrigins {
				if o != "" && o == origin {
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		}

		if c.Writer.Header().Get("Access-Control-Allow-Origin") != "*" {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		headers := c.Writer.Header()
		headers.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With, Accept, Origin")
		headers.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		headers.Set("Access-Control-Max-Age", "86400")

		// Security headers
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("X-XSS-Protection", "1; mode=block")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// Recovery returns a custom recovery middleware
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"is_success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Đã xảy ra lỗi máy chủ nội bộ",
			},
		})
	})
}
