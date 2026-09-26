package database

import (
	"database/sql"
	"testing"
)

// TestDB_UserMe_tenantScoped proves the converted Me read: a user is readable
// under its OWN business, and a cross-business read of the SAME user id is
// NOT-FOUND (no existence leak) — the RLS-scoped 404 the handler returns.
func TestDB_UserMe_tenantScoped(t *testing.T) {
	db := requireHarness(t)
	urepo := NewUserRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	userA := insertUser(t, db, bizA)

	// Own business: the session user reads their own profile.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		u, err := urepo.FindByIDTXN(tx, userA)
		if err != nil {
			return err
		}
		if u == nil {
			t.Fatal("own-business Me read must succeed")
		}
		if u.BusinessID != bizA {
			t.Fatalf("Me read business = %s, want %s", u.BusinessID, bizA)
		}
		return nil
	})

	// Cross-business: same user id under a different tenant is invisible -> nil
	// (the handler maps this to 404 "user not found").
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		u, err := urepo.FindByIDTXN(tx, userA)
		if err != nil {
			return err
		}
		if u != nil {
			t.Fatal("cross-business Me read must be NOT-FOUND (nil), no existence leak")
		}
		return nil
	})
}
