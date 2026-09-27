package database

// SECURITY-CONTRACT guard rails — re-encoding the archived MikroTik Magic
// security guarantees on this stack (docs/SECURITY-CONTRACT.md §1, §3, §4).
//
// These run in the SAME throwaway-DB harness as voucher_integration_test.go
// (activated by MM_TEST_DB_URL; skip when unset). They prove, against a real
// Postgres loaded from database/init.sql, that the app's serving path is
// fail-closed on inactive accounts and that active accounts are the only ones
// resolvable for credential auth.

import (
	"testing"
)

// TestUserRepo_inactiveIsUnfindable encodes §4 auth-session hygiene and §3
// fail-closed: an INACTIVE (suspended) user must never be returned by the
// credential-path lookup (FindByEmail). Login therefore cannot proceed — the
// business-suspend cascade that locked an admin out in the 2026-09-27 incident
// is the exact regression this pins.
func TestUserRepo_inactiveIsUnfindable(t *testing.T) {
	requireHarness(t)
	bizA := insertBusiness(t, harnessDB)
	userA := insertUser(t, harnessDB, bizA)

	// Make the user inactive exactly as a business-suspend cascade does.
	_, err := harnessDB.Exec(`UPDATE users SET is_active = false WHERE user_id = $1`, userA)
	if err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	repo := NewUserRepository(harnessDB)
	var email string
	_ = harnessDB.QueryRow(`SELECT email FROM users WHERE user_id = $1`, userA).Scan(&email)

	u, err := repo.FindByEmail(email)
	if err != nil {
		t.Fatalf("FindByEmail errored (should return nil,nil for inactive): %v", err)
	}
	if u != nil {
		t.Fatalf("FindByEmail returned an inactive user (%v); want nil — suspended account must be unfindable for login", u.UserID)
	}
}

// TestUserRepo_activeIsFindable is the positive control: the active user IS
// resolvable. Guards against a regression where the is_active=true filter is
// removed entirely (which would then also break legitimate logins).
func TestUserRepo_activeIsFindable(t *testing.T) {
	requireHarness(t)
	bizA := insertBusiness(t, harnessDB)
	userA := insertUser(t, harnessDB, bizA)

	var email string
	_ = harnessDB.QueryRow(`SELECT email FROM users WHERE user_id = $1`, userA).Scan(&email)

	repo := NewUserRepository(harnessDB)
	u, err := repo.FindByEmail(email)
	if err != nil {
		t.Fatalf("active user FindByEmail errored: %v", err)
	}
	if u == nil || u.UserID == "" {
		t.Fatalf("active user not resolved")
	}
	// Bogus email correctly yields a nil user (nil,nil contract — no error, no row).
	if ghost, gerr := repo.FindByEmail("ghost-" + randHex(6) + "@nobody.test"); gerr != nil || ghost != nil {
		t.Fatalf("unknown email returned non-nil (user=%v err=%v); want nil,nil", ghost, gerr)
	}
}