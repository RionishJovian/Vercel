package database

import (
	"database/sql"
	"testing"
)

// TestDB_Team_isolation proves C3 tenant isolation for the team API: a business
// sees only its own users, and cross-tenant reads/updates are blocked.
func TestDB_Team_isolation(t *testing.T) {
	db := requireHarness(t)
	urepo := NewUserRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	userA := insertUser(t, db, bizA)

	// Own tenant: the user is listed and fetchable.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		us, e := urepo.ListTeamByBusinessTXN(tx, bizA)
		if e != nil {
			return e
		}
		found := false
		for _, u := range us {
			if u.UserID == userA {
				found = true
			}
		}
		if !found {
			t.Fatal("own team member not listed")
		}
		got, e := urepo.GetTeamUserTXN(tx, bizA, userA)
		if e != nil {
			return e
		}
		if got == nil {
			t.Fatal("own team member not fetchable")
		}
		n, e := urepo.CountActiveOwnersTXN(tx, bizA)
		if e != nil {
			return e
		}
		if n < 1 {
			t.Fatalf("active owners = %d, want >=1", n)
		}
		return nil
	})

	// Cross-tenant: invisible and immutable.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		us, e := urepo.ListTeamByBusinessTXN(tx, bizB)
		if e != nil {
			return e
		}
		if len(us) != 0 {
			t.Fatalf("tenant B sees %d users, want 0", len(us))
		}
		got, e := urepo.GetTeamUserTXN(tx, bizB, userA)
		if e != nil {
			return e
		}
		if got != nil {
			t.Fatal("cross-business team member visible to tenant B")
		}
		upd, e := urepo.UpdateTeamUserTXN(tx, bizB, userA, "Hacked", "OWNER", true)
		if e != nil {
			return e
		}
		if upd != nil {
			t.Fatal("cross-business team update must affect 0 rows")
		}
		return nil
	})

	// Cross-tenant disable must fail (tenant-guarded).
	var disErr error
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		disErr = urepo.SetTeamActiveTXN(tx, bizB, userA, false)
		return nil
	})
	if disErr == nil {
		t.Fatal("cross-business team disable must not succeed")
	}
}
