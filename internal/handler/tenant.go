package handler

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// beginTenantTx starts a transaction for a tenant request. When the request
// carries a middleware-pinned tenant connection (rls_enforce ON), the tx runs on
// THAT pinned connection so the whole request is connection-level isolated;
// otherwise it falls back to the owner pool (flag OFF / unconverted paths).
func beginTenantTx(c *gin.Context, owner *database.DB) (*sql.Tx, error) {
	if conn := auth.PinnedConn(c); conn != nil {
		return conn.BeginTx(c.Request.Context(), nil)
	}
	return owner.Begin()
}

// runInTenant executes fn in a transaction that is RLS-bound to businessID,
// committing on success and rolling back on error. WithTenant establishes the
// role + tenant context (idempotent on an already-pinned connection).
func runInTenant(c *gin.Context, owner *database.DB, businessID string, fn func(tx *sql.Tx) error) error {
	tx, err := beginTenantTx(c, owner)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := database.WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}
