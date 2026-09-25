package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"mega-magic/internal/config"
)

// uuidRe matches a canonical 8-4-4-4-12 UUID (the type of business_id).
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// WithTenant runs fn inside a transaction with the tenant context established so
// the RLS policies actually apply.
//
// The connection is pinned for the whole transaction (sql.DB.Begin binds the Tx
// to one connection), so issuing these here reliably scopes every query fn runs:
//
//	SET LOCAL ROLE authenticated
//	SELECT set_config('app.current_business_id', $1, true)
//
// Both are transaction-local and revert when the transaction ends, so they never
// leak onto a pooled connection.
//
// Fail-closed: if the tx is nil, the business id is missing/malformed, or the
// role/context cannot be set, fn is NOT run and the error is returned. Errors are
// never swallowed.
//
// NOTE (boundary): this helper only protects queries issued on THIS tx. The
// existing bare pool helpers (db.Exec/db.QueryRow) draw arbitrary pooled
// connections and are NOT covered — that is the remaining enforcement work.
func WithTenant(tx *sql.Tx, businessID string, fn func() error) error {
	if tx == nil {
		return errors.New("database: WithTenant requires a transaction")
	}
	if businessID == "" {
		return errors.New("database: WithTenant requires a business id")
	}
	if !uuidRe.MatchString(businessID) {
		return fmt.Errorf("database: WithTenant got a malformed business id %q", businessID)
	}
	if fn == nil {
		return errors.New("database: WithTenant requires a function")
	}

	if _, err := tx.Exec(`SET LOCAL ROLE authenticated`); err != nil {
		return fmt.Errorf("database: set tenant role: %w", err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.current_business_id', $1, true)`, businessID); err != nil {
		return fmt.Errorf("database: set tenant context: %w", err)
	}

	runErr := fn()

	// Restore eagerly (SET LOCAL already reverts at tx end) so a caller that keeps
	// using the tx is not left scoped as `authenticated`. A restore failure only
	// surfaces when fn itself succeeded.
	if _, err := tx.Exec(`RESET ROLE`); err != nil && runErr == nil {
		return fmt.Errorf("database: reset tenant role: %w", err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.current_business_id', '', true)`); err != nil && runErr == nil {
		return fmt.Errorf("database: reset tenant context: %w", err)
	}
	return runErr
}

// NewRestricted opens a SECOND pool used for tenant-pinned connections.
//
// Credential choice (Stage 0, option b — security call, documented): the pool
// authenticates with the SAME configured credentials as the owner pool, and the
// tenant boundary is established per leased connection via `SET ROLE
// authenticated` (see TenantConn). This needs no new DB role or secret and is
// fully reversible behind database.rls_enforce.
//
// LIMITATION: the connection itself holds owner credentials, so the guarantee
// rests on every pinned connection being downgraded before use (the tenant
// middleware always does, and releases reset it). The stronger option (a) — a
// dedicated login role (e.g. `mega_app`) GRANTED membership in `authenticated`,
// so the pool is non-owner at LOGIN — is the recommended production hardening
// before flipping the flag.
func NewRestricted(cfg config.DatabaseConfig) (*DB, error) {
	db, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// TenantConn leases ONE connection from the pool and binds it to the tenant:
//
//	SET ROLE authenticated
//	SELECT set_config('app.current_business_id', $1, false)
//
// The caller MUST return it with ReleaseTenantConn. Fail-closed: an
// empty/malformed business id is refused before any role change.
func (db *DB) TenantConn(ctx context.Context, businessID string) (*sql.Conn, error) {
	if businessID == "" {
		return nil, errors.New("database: TenantConn requires a business id")
	}
	if !uuidRe.MatchString(businessID) {
		return nil, fmt.Errorf("database: TenantConn got a malformed business id %q", businessID)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("database: acquire conn: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `SET ROLE authenticated`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("database: set tenant role: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT set_config('app.current_business_id', $1, false)`, businessID); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("database: set tenant context: %w", err)
	}
	return conn, nil
}

// ReleaseTenantConn resets a pinned connection's role + tenant context and
// returns it to the pool. Best-effort: it always closes the conn.
func ReleaseTenantConn(ctx context.Context, conn *sql.Conn) {
	if conn == nil {
		return
	}
	_, _ = conn.ExecContext(ctx, `RESET ROLE`)
	_, _ = conn.ExecContext(ctx, `SELECT set_config('app.current_business_id', '', false)`)
	_ = conn.Close()
}
