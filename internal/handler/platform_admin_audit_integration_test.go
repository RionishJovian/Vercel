package handler

import (
	"testing"

	"mega-magic/internal/database"
)

// Suspending and unsuspending a business must not change which of its users are switched on,
// and every platform action must leave an audit row.
func TestPlatformSuspendKeepsUserFlagsAndAudits(t *testing.T) {
	db := harness_testdb(t)
	repo := database.NewPlatformBusinessRepository(db)
	actor := database.PlatformActor{IP: "203.0.113.9", UserAgent: "test"}
	email := "plat-" + randHex(6) + "@example.test"
	biz, owner, err := repo.CreateTenant("Audit Cafe", email, "$2a$10$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "SMALL_WISP", 1, actor)
	if err != nil {
		t.Fatal(err)
	}
	// One extra user the owner switched off.
	var off string
	if err := db.QueryRow(`INSERT INTO users(business_id,full_name,email,password_hash,user_role,is_active)
		VALUES($1,'Off User',$2,'x','ADMIN',false) RETURNING user_id`, biz, "off-"+email).Scan(&off); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBusinessActive(biz, false, actor); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBusinessActive(biz, true, actor); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResetOwnerPassword(biz, "$2a$10$bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", actor); err != nil {
		t.Fatal(err)
	}
	var ownerOn, offOn bool
	if err := db.QueryRow(`SELECT is_active FROM users WHERE user_id=$1`, owner).Scan(&ownerOn); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT is_active FROM users WHERE user_id=$1`, off).Scan(&offOn); err != nil {
		t.Fatal(err)
	}
	if !ownerOn || offOn {
		t.Fatalf("user flags changed by suspend/unsuspend: owner=%v disabled-user=%v (want true,false)", ownerOn, offOn)
	}
	rows, err := db.Query(`SELECT action FROM audit_events WHERE business_id=$1 AND user_role='DEVELOPER' ORDER BY "timestamp", action`, biz)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		got[a] = true
	}
	for _, want := range []string{"platform.business.create", "platform.business.suspend", "platform.business.unsuspend", "platform.owner.password_reset"} {
		if !got[want] {
			t.Errorf("missing audit row %s (have %v)", want, got)
		}
	}
}
