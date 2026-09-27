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

// withDeviceTenant runs fn in a transaction RLS-bound to a business resolved from
// a DEVICE. The anonymous portal path has no auth context, so it cannot use the
// pinned connection (which only exists on protected/flagged requests); it opens
// its own tx and establishes the tenant from the device-derived business.
func withDeviceTenant(owner *database.DB, businessID string, fn func(tx *sql.Tx) error) error {
	tx, err := owner.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := database.WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}

// runGlobalTx executes fn in a transaction with NO tenant binding, for the
// platform-global tables (no business_id column, RLS disabled). Binding a tenant
// here would be a lie: with RLS off it changes nothing, and it would imply the
// rows are isolated per business when every business shares them. Callers must
// therefore gate writes on the DEVELOPER platform role instead.
func runGlobalTx(owner *database.DB, fn func(tx *sql.Tx) error) error {
	tx, err := owner.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
