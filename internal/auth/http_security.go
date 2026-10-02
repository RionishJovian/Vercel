package auth

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Cookie-authenticated writes must come from the same browser origin. The
// custom header also prevents cross-origin HTML forms from logging users in.
func CookieOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != c.Request.Host || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
				c.AbortWithStatusJSON(403, gin.H{"error": "cross-origin request refused"})
				return
			}
		}
		_, cookieErr := c.Cookie("mm_access")
		_, refreshErr := c.Cookie("mm_refresh")
		cookieRequest := cookieErr == nil || refreshErr == nil
		authRequest := strings.HasPrefix(c.Request.URL.Path, "/api/v1/auth/")
		if c.GetHeader("Authorization") == "" && ((cookieRequest && origin == "") || ((cookieRequest || authRequest) && c.GetHeader("X-MM-CSRF") != "1")) {
			c.AbortWithStatusJSON(403, gin.H{"error": "same-origin request required"})
			return
		}
		c.Next()
	}
}

// Bounded process-local rate limit. Deployments with multiple API instances
// should enforce a shared limit at their proxy as well.
func BoundedAuthRateLimit() gin.HandlerFunc {
	return boundedRateLimit("too many authentication attempts", true)
}

// Public portal writes and lookups are anonymous but still bounded. This is
// process-local protection; multiple replicas also need a shared proxy limit.
func BoundedPortalRateLimit() gin.HandlerFunc {
	return boundedRateLimit("too many portal requests", false)
}

func boundedRateLimit(message string, skipLogout bool) gin.HandlerFunc {
	type bucket struct {
		count int
		until time.Time
	}
	var mu sync.Mutex
	entries := map[string]bucket{}
	return func(c *gin.Context) {
		if skipLogout && strings.HasSuffix(c.Request.URL.Path, "/logout") {
			c.Next()
			return
		}
		key := c.ClientIP()
		now := time.Now()
		mu.Lock()
		b := entries[key]
		if !now.Before(b.until) {
			b = bucket{until: now.Add(time.Minute)}
		}
		if len(entries) >= 4096 {
			for k, v := range entries {
				if !now.Before(v.until) {
					delete(entries, k)
				}
			}
		}
		_, known := entries[key]
		blocked := b.count >= 30 || (!known && len(entries) >= 4096)
		if !blocked {
			b.count++
			entries[key] = b
		}
		mu.Unlock()
		if blocked {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": message})
			return
		}
		c.Next()
	}
}
