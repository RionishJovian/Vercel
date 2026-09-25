package auth

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/database"
)

// ContextKeyPinnedConn is the gin context key for the request's RLS-bound
// database connection (present only when tenant enforcement is enabled).
const ContextKeyPinnedConn = "db_pinned_conn"

// TenantConnMiddleware pins each protected request to ONE database connection
// bound to the tenant (SET ROLE authenticated + app.current_business_id), so RLS
// applies at the CONNECTION level for the whole request — not merely per
// transaction. It must run AFTER Middleware (which resolves business_id).
//
// It is a no-op pass-through when business_id is absent, and it always releases
// the connection back to the pool afterwards. Activated only when
// database.rls_enforce is on (the caller wires it conditionally).
func TenantConnMiddleware(pool *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bid, _ := c.Get(ContextKeyBusinessID)
		businessID, _ := bid.(string)
		if businessID == "" {
			c.Next()
			return
		}

		conn, err := pool.TenantConn(c.Request.Context(), businessID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		c.Set(ContextKeyPinnedConn, conn)
		defer database.ReleaseTenantConn(c.Request.Context(), conn)

		c.Next()
	}
}

// PinnedConn returns the request's pinned tenant connection, or nil when the
// request is not running on the restricted path (flag off).
func PinnedConn(c *gin.Context) *sql.Conn {
	v, ok := c.Get(ContextKeyPinnedConn)
	if !ok {
		return nil
	}
	conn, _ := v.(*sql.Conn)
	return conn
}
