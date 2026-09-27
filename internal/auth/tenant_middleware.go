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

// RequireActiveTenant rejects requests from a tenant whose business is
// suspended (businesses.is_active = false).
//
// Blocking suspension at login alone is not enough: a token issued before the
// suspension stays valid until it expires, so the tenant keeps working for the
// whole token lifetime and the suspend action looks like it did nothing. This
// closes that window.
//
// It costs one primary-key lookup per protected request. That is deliberate —
// a suspension that does not take effect immediately is not a suspension — and
// it is a single indexed read on a small table.
//
// Fails CLOSED: if the tenant state cannot be read, the request is refused.
// Failing open would let a database blip re-enable a suspended tenant.
func RequireActiveTenant(pool *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bid, _ := c.Get(ContextKeyBusinessID)
		businessID, _ := bid.(string)
		if businessID == "" {
			// No tenant context (e.g. platform routes) — nothing to check here.
			c.Next()
			return
		}

		var active bool
		err := pool.QueryRow(
			`SELECT is_active FROM businesses WHERE business_id = $1`, businessID).Scan(&active)
		if err != nil || !active {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": "this business account is suspended"})
			return
		}

		c.Next()
	}
}
