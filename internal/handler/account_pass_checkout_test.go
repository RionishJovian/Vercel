package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/accountpass"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

func TestPaidAccountPassCheckout(t *testing.T) {
	db := harness_testdb(t)
	service, _ := auth.NewService("disposable-pass-checkout-fixture", time.Hour, 24*time.Hour, 4)
	service.UseSessions(db)
	var bank string
	if err := db.QueryRow(`INSERT INTO platform_service_bank_accounts(slot,holder_name,bank_name,account_number,enabled)VALUES(1,'Disposable payee','Fixture bank','TEST-ONLY-ACCOUNT',true) ON CONFLICT(slot)DO UPDATE SET holder_name=excluded.holder_name,bank_name=excluded.bank_name,account_number=excluded.account_number,enabled=true RETURNING id`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO pricing_promo(id,active,monthly_standard_mmk,annual_standard_mmk)VALUES(true,false,29000,285000) ON CONFLICT(id)DO UPDATE SET active=false,monthly_standard_mmk=29000,annual_standard_mmk=285000`); err != nil {
		t.Fatal(err)
	}
	makeUser := func(biz string, role model.UserRole) *model.User {
		u := &model.User{BusinessID: biz, Email: randHex(8) + "@checkout.fixture", UserRole: role, PasswordHash: "not-a-real-password"}
		if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','not-a-real-password',$3)RETURNING user_id`, biz, u.Email, role).Scan(&u.UserID); err != nil {
			t.Fatal(err)
		}
		if role == model.RoleOwner {
			if _, err := db.Exec(`INSERT INTO business_app_primary_users(business_id,user_id)VALUES($1,$2)`, biz, u.UserID); err != nil {
				t.Fatal(err)
			}
		}
		return u
	}
	dev := makeUser(insertBusiness(db), model.RoleDeveloper)
	token := func(u *model.User) string {
		a, _, e := service.StartSession(u)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	devToken := token(dev)
	a := NewAuthHandler(database.NewUserRepository(db), service, db)
	a.IndividualPassesEnabled = true
	p := NewPlatformHandler(database.NewPlatformBusinessRepository(db), nil, nil, db, service)
	p.IndividualPassesEnabled = true
	r := gin.New()
	g := r.Group("/api/v1", auth.Middleware(service), auth.RequireActiveTenant(db), IndividualPassAccess(db, true))
	g.GET("/me/app-pass", a.AppPass)
	g.GET("/me/app-pass/checkout-options", a.AccountPassCheckoutOptions)
	g.GET("/me/app-pass/renewals", a.AccountRenewals)
	g.POST("/me/app-pass/renewals", a.RequestAccountRenewal)
	g.POST("/routers", func(c *gin.Context) { c.Status(204) })
	platform := r.Group("/api/v1/platform", auth.Middleware(service), auth.RequireRole("DEVELOPER"))
	platform.POST("/account-renewals/:id/decision", p.DecideAccountRenewal)
	platform.PUT("/accounts/:userId/pass", p.RecordAccountPass)
	req := func(method, path, bearer string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		q := httptest.NewRequest(method, path, bytes.NewReader(raw))
		q.Header.Set("Content-Type", "application/json")
		q.Header.Set("Authorization", "Bearer "+bearer)
		r.ServeHTTP(w, q)
		return w
	}
	check := func(t *testing.T, w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("expected %d got %d: %s", status, w.Code, w.Body)
		}
	}
	for _, plan := range []string{"monthly", "annual"} {
		for _, state := range []string{"new", "active", "expired"} {
			t.Run(plan+"/"+state, func(t *testing.T) {
				u := makeUser(insertBusiness(db), model.RoleOwner)
				bearer := token(u)
				var previousStart, previousEnd time.Time
				if state != "new" {
					start := time.Now().UTC().AddDate(0, 0, -10)
					if state == "expired" {
						start = time.Now().UTC().AddDate(0, -2, 0)
					}
					tx, _ := db.Begin()
					_, err := database.ActivateAccountPassTXN(tx, u.UserID, u.BusinessID, dev.UserID, "monthly", 0, start)
					if err != nil {
						tx.Rollback()
						t.Fatal(err)
					}
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
					if err = db.QueryRow(`SELECT activated_at,expires_at FROM app_account_passes WHERE user_id=$1`, u.UserID).Scan(&previousStart, &previousEnd); err != nil {
						t.Fatal(err)
					}
				}
				check(t, req("GET", "/api/v1/me/app-pass/checkout-options", bearer, nil), 200)
				if state != "active" {
					check(t, req("POST", "/api/v1/routers", bearer, nil), 402)
				}
				amount := 29000
				if plan == "annual" {
					amount = 285000
				}
				key := fmt.Sprintf("%s-%s-4%s-8%s-%s", randHex(4), randHex(2), randHex(2)[:3], randHex(2)[:3], randHex(6))
				payload := map[string]any{"plan": plan, "bank_account_id": bank, "bank_reference": "FIXTURE-TRANSFER-" + randHex(4), "expected_amount_mmk": amount, "idempotency_key": key}
				// Same-key concurrent submissions must converge before any approval.
				responses := make([]*httptest.ResponseRecorder, 2)
				var wg sync.WaitGroup
				for i := range responses {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						responses[i] = req("POST", "/api/v1/me/app-pass/renewals", bearer, payload)
					}(i)
				}
				wg.Wait()
				var first, second struct {
					ID string `json:"id"`
				}
				check(t, responses[0], 200)
				check(t, responses[1], 200)
				json.Unmarshal(responses[0].Body.Bytes(), &first)
				json.Unmarshal(responses[1].Body.Bytes(), &second)
				if first.ID == "" || first.ID != second.ID {
					t.Fatal("duplicate purchase identity")
				}
				var count int
				if e := db.QueryRow(`SELECT count(*) FROM app_account_pass_renewals WHERE user_id=$1`, u.UserID).Scan(&count); e != nil || count != 1 {
					t.Fatal("purchase duplicated", count, e)
				}
				other := makeUser(insertBusiness(db), model.RoleOwner)
				w := req("GET", "/api/v1/me/app-pass/renewals", token(other), nil)
				check(t, w, 200)
				if bytes.Contains(w.Body.Bytes(), []byte(first.ID)) {
					t.Fatal("foreign checkout leaked")
				}
				payload["expected_amount_mmk"] = amount + 1
				check(t, req("POST", "/api/v1/me/app-pass/renewals", bearer, payload), 409)
				payload["expected_amount_mmk"] = amount
				path := "/api/v1/platform/account-renewals/" + first.ID + "/decision"
				check(t, req("POST", path, bearer, map[string]any{"approve": true, "payment_verified": true}), 403)
				check(t, req("POST", path, devToken, map[string]any{"approve": true}), 400)
				check(t, req("PUT", "/api/v1/platform/accounts/"+u.UserID+"/pass", devToken, map[string]any{"action": "activate", "plan": plan, "revision": 0}), 409)
				var priorRevision int64
				_ = db.QueryRow(`SELECT revision FROM app_account_passes WHERE user_id=$1`, u.UserID).Scan(&priorRevision)
				check(t, req("POST", path, devToken, map[string]any{"approve": true, "payment_verified": true}), 200)
				check(t, req("POST", path, devToken, map[string]any{"approve": true, "payment_verified": true}), 409)
				check(t, req("POST", "/api/v1/me/app-pass/renewals", bearer, payload), 200)
				var start, end time.Time
				var revision int64
				if e := db.QueryRow(`SELECT activated_at,expires_at,revision FROM app_account_passes WHERE user_id=$1`, u.UserID).Scan(&start, &end, &revision); e != nil {
					t.Fatal(e)
				}
				base := start
				if state == "active" {
					base = previousEnd
					if !start.Equal(previousStart) {
						t.Fatal("original activation reset")
					}
				}
				want, e := accountpass.CalendarExpiry(base, plan)
				if e != nil || !end.Equal(want) || revision != priorRevision+1 {
					t.Fatal("calendar entitlement incorrect", start, end, want, revision, e)
				}
				check(t, req("POST", "/api/v1/routers", bearer, nil), 204)
				var events int
				db.QueryRow(`SELECT count(*) FROM app_account_pass_events WHERE user_id=$1`, u.UserID).Scan(&events)
				if int64(events) != revision {
					t.Fatal("activation replayed", events, revision)
				}
			})
		}
	}
	// Forged prices and exempt/secondary users cannot create paid requests.
	u := makeUser(insertBusiness(db), model.RoleOwner)
	bearer := token(u)
	secondary := makeUser(u.BusinessID, model.RoleAdmin)
	key := "11111111-1111-4111-8111-111111111111"
	payload := map[string]any{"plan": "monthly", "bank_account_id": bank, "bank_reference": "DISPOSABLE", "expected_amount_mmk": 1, "idempotency_key": key}
	check(t, req("POST", "/api/v1/me/app-pass/renewals", bearer, payload), 409)
	payload["expected_amount_mmk"] = 29000
	check(t, req("POST", "/api/v1/me/app-pass/renewals", devToken, payload), 403)
	check(t, req("POST", "/api/v1/me/app-pass/renewals", token(secondary), payload), 403)
	if _, e := db.Exec(`UPDATE platform_service_bank_accounts SET enabled=false WHERE id=$1`, bank); e != nil {
		t.Fatal(e)
	}
	check(t, req("POST", "/api/v1/me/app-pass/renewals", bearer, payload), 409)
	db.Exec(`UPDATE platform_service_bank_accounts SET enabled=true WHERE id=$1`, bank)
	if _, e := db.Exec(`INSERT INTO app_account_pass_renewals(user_id,business_id,plan,bank_account_id,bank_reference,bank_snapshot,idempotency_key)VALUES($1,$2,'monthly',$3,'INVALID-PARTIAL','{}',$4)`, u.UserID, u.BusinessID, bank, key); e == nil {
		t.Fatal("partial nullable payment metadata bypassed database constraint")
	}
}
