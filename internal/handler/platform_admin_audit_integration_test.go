package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
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

func TestPlatformPricingPublishIsAudited(t *testing.T) {
	db := harness_testdb(t)
	repo := database.NewPlatformBusinessRepository(db)
	email := "price-" + randHex(6) + "@example.test"
	biz, owner, err := repo.CreateTenant("Price Cafe", email, "$2a$10$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "SMALL_WISP", 1, database.PlatformActor{})
	if err != nil {
		t.Fatal(err)
	}
	h := NewPlatformHandler(repo, database.NewPlatformIncidentRepository(db), database.NewPricingRepository(db), db, nil)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(auth.ContextKeyBusinessID, biz)
	c.Set(auth.ContextKeyUserID, owner)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/platform/pricing", strings.NewReader(`{"monthly_standard_mmk":9000,"annual_standard_mmk":90000,"active":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.PutPricing(c)
	if w.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM audit_events WHERE business_id=$1 AND action='platform.pricing.publish' AND after_state->>'monthly_standard_mmk'='9000'`, biz).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pricing audit rows = %d (err %v), want 1", n, err)
	}
}

func TestWebFigAddonActiveFollowsApproval(t *testing.T) {
	db := harness_testdb(t)
	repo := database.NewPlatformBusinessRepository(db)
	email := "wf-" + randHex(6) + "@example.test"
	biz, owner, err := repo.CreateTenant("WebFig Cafe", email, "$2a$10$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "SMALL_WISP", 1, database.PlatformActor{})
	if err != nil {
		t.Fatal(err)
	}
	var addon string
	if err := db.QueryRow(`INSERT INTO store_addons(addon_key,kind,name_en,name_my,price_mmk,period)
		VALUES('webfig_access','feature','WebFig','WebFig',10000,'monthly')
		ON CONFLICT (addon_key) DO UPDATE SET active=true RETURNING id`).Scan(&addon); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	active := func() bool {
		ok, err := database.WebFigAddonActive(context.Background(), db, biz, now)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if active() {
		t.Fatal("active with no request")
	}
	var req string
	if err := db.QueryRow(`INSERT INTO store_addon_requests(business_id,user_id,addon_id,price_mmk,period,bank_reference,idempotency_key)
		VALUES($1,$2,$3,10000,'monthly','REF-0001',$4) RETURNING id`, biz, owner, addon, "idem-"+randHex(8)).Scan(&req); err != nil {
		t.Fatal(err)
	}
	if active() {
		t.Fatal("active while the request is still pending")
	}
	if _, err := db.Exec(`UPDATE store_addon_requests SET status='approved', decided_at=$2 WHERE id=$1`, req, now.AddDate(0, 0, -10)); err != nil {
		t.Fatal(err)
	}
	if !active() {
		t.Fatal("not active 10 days after approval")
	}
	if _, err := db.Exec(`UPDATE store_addon_requests SET decided_at=$2 WHERE id=$1`, req, now.AddDate(0, -1, -2)); err != nil {
		t.Fatal(err)
	}
	if active() {
		t.Fatal("still active more than a day after the month ended")
	}
}
