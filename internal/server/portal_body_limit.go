package server

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func boundedPortalBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow multipart overhead around the independently capped 5 MiB photo.
		const limit = 6 << 20
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "portal request is too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
