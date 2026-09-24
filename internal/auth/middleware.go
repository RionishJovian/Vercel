package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// ContextKeyUserID is the gin context key for the authenticated user ID
	ContextKeyUserID = "user_id"
	// ContextKeyBusinessID is the gin context key for the tenant business ID
	ContextKeyBusinessID = "business_id"
	// ContextKeyUserEmail is the gin context key for the user email
	ContextKeyUserEmail = "user_email"
	// ContextKeyUserRole is the gin context key for the user role
	ContextKeyUserRole = "user_role"
)

// Middleware returns a Gin middleware that validates JWT tokens and extracts
// tenant context. It sets the business_id session variable in Postgres for RLS.
func Middleware(authService *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization must be Bearer <token>",
			})
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired token",
			})
			return
		}

		// Set tenant context in request context
		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyBusinessID, claims.BusinessID)
		c.Set(ContextKeyUserEmail, claims.Email)
		c.Set(ContextKeyUserRole, claims.UserRole)

		c.Next()
	}
}

// OptionalMiddleware is like Middleware but doesn't abort if no token is present.
// Claims will be empty if no valid token is provided.
func OptionalMiddleware(authService *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.Next()
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			c.Next()
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyBusinessID, claims.BusinessID)
		c.Set(ContextKeyUserEmail, claims.Email)
		c.Set(ContextKeyUserRole, claims.UserRole)

		c.Next()
	}
}

// GetUserID returns the authenticated user ID from context
func GetUserID(c *gin.Context) string {
	id, _ := c.Get(ContextKeyUserID)
	return id.(string)
}

// GetBusinessID returns the tenant business ID from context
func GetBusinessID(c *gin.Context) string {
	id, _ := c.Get(ContextKeyBusinessID)
	return id.(string)
}

// GetUserRole returns the user role from context
func GetUserRole(c *gin.Context) string {
	role, _ := c.Get(ContextKeyUserRole)
	return role.(string)
}

// RequireRole returns a middleware that checks the user has one of the specified roles
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get(ContextKeyUserRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		roleStr := userRole.(string)
		for _, allowed := range roles {
			if roleStr == allowed {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "insufficient permissions",
		})
	}
}