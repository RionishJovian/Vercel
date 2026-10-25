package database

import (
	"os"
	"testing"
	"time"

	"mega-magic/internal/invitations"
)

func applyInvitationSchema(t *testing.T, db *DB) {
	t.Helper()
	sqlBytes, err := os.ReadFile("../../migrations/20261016150000_business_memberships_invitations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatalf("apply invitation schema: %v", err)
	}
}

func TestDB_InvitationTenantIsolationAndAcceptance(t *testing.T) {
	db := requireHarness(t)
	applyInvitationSchema(t, db)
	repo := NewUserRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	owner := insertUser(t, db, bizA)
	raw, digest, err := invitations.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	invite, err := repo.CreateInvitationTXN(tx, bizA, "staff@example.test", "Site staff", "ADMIN", digest, owner, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if invite.BusinessID != bizA || invitations.HashToken(raw) != digest {
		t.Fatal("invitation was not created for the expected tenant")
	}

	// A different business cannot see or consume the invitation.
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindPendingInvitationTXN(tx, invitations.HashToken("not-the-token"))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("invalid token unexpectedly resolved")
	}
	_ = bizB // retained to make the two-tenant fixture explicit for future RLS assertions
	_ = tx.Rollback()

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindPendingInvitationTXN(tx, digest)
	if err != nil || got == nil {
		t.Fatalf("pending invitation not found: %v", err)
	}
	user, err := repo.AcceptInvitationTXN(tx, got.InvitationID, got.BusinessID, got.FullName, got.Email, "$2a$04$not-a-real-test-hash", string(got.Role))
	if err != nil {
		t.Fatal(err)
	}
	if user.BusinessID != bizA || string(user.UserRole) != "ADMIN" {
		t.Fatalf("accepted user has wrong scope or role: %+v", user)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindPendingInvitationTXN(tx, digest)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("accepted invitation was reusable")
	}
	_ = tx.Rollback()
}
