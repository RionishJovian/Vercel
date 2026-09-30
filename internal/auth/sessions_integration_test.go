package auth

import (
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"os"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	base := os.Getenv("MM_TEST_DB_URL")
	if base == "" {
		t.Skip("set MM_TEST_DB_URL to run disposable PostgreSQL integration tests")
	}
	admin, db, name, ok := newThrowaway(t, base)
	if !ok {
		return
	}
	defer func() {
		db.Close()
		if _, err := admin.Exec(`DROP DATABASE "` + name + `"`); err != nil {
			t.Error("throwaway database cleanup failed")
		}
		admin.Close()
	}()
	if err := database.CheckAuthSessionSchema(db); err != nil {
		t.Fatal(err)
	}
	var business string
	if err := db.QueryRow(`SELECT business_id FROM businesses LIMIT 1`).Scan(&business); err != nil {
		t.Fatal(err)
	}
	s, _ := NewService("integration-test-secret-long-enough", time.Hour, 24*time.Hour, 4)
	s.UseSessions(db)
	hash, _ := s.HashPassword("test-password")
	u := &model.User{BusinessID: business, UserRole: model.RoleOwner, PasswordHash: hash}
	if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role) VALUES($1,$2,'Session Test',$3,'OWNER') RETURNING user_id`, business, randomID()+"@example.test", hash).Scan(&u.UserID); err != nil {
		t.Fatal(err)
	}
	mustStart := func() (string, string) {
		t.Helper()
		a, r, err := s.StartSession(u)
		if err != nil {
			t.Fatal(err)
		}
		return a, r
	}
	a, r := mustStart()
	if _, err := s.ValidateToken(a); err != nil {
		t.Fatal(err)
	}
	a2, r2, err := s.RotateSession(r, u)
	if err != nil {
		t.Fatal(err)
	}
	if r2 == r {
		t.Fatal("refresh was not rotated")
	}
	if _, err := s.ValidateToken(a2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RotateSession(r, u); err == nil {
		t.Fatal("old refresh reused")
	}
	if _, err := s.ValidateToken(a2); err == nil {
		t.Fatal("reuse did not revoke access")
	}
	if _, _, err := s.RotateSession(r2, u); err == nil {
		t.Fatal("reuse did not revoke replacement refresh")
	}
	for _, change := range []struct{ name, update, restore string }{
		{"disable", `UPDATE users SET is_active=false WHERE user_id=$1`, `UPDATE users SET is_active=true WHERE user_id=$1`},
		{"role", `UPDATE users SET user_role='VIEWER' WHERE user_id=$1`, `UPDATE users SET user_role='OWNER' WHERE user_id=$1`},
		{"password", `UPDATE users SET password_hash='changed' WHERE user_id=$1`, `UPDATE users SET password_hash=$2 WHERE user_id=$1`},
	} {
		t.Run(change.name, func(t *testing.T) {
			a, r := mustStart()
			if _, err := db.Exec(change.update, u.UserID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ValidateToken(a); err == nil {
				t.Fatal("changed account retained access")
			}
			if change.name == "password" {
				_, err = db.Exec(change.restore, u.UserID, hash)
			} else {
				_, err = db.Exec(change.restore, u.UserID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ValidateToken(a); err == nil {
				t.Fatal("restoring account resurrected session")
			}
			if _, _, err := s.RotateSession(r, u); err == nil {
				t.Fatal("revoked refresh accepted")
			}
		})
	}
	a, r = mustStart()
	if err := s.RevokeSession(r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateToken(a); err == nil {
		t.Fatal("logout left access valid")
	}
	a, _ = mustStart()
	if _, err := db.Exec(`UPDATE businesses SET is_active=false WHERE business_id=$1`, business); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateToken(a); err == nil {
		t.Fatal("suspended business retained access")
	}
}
