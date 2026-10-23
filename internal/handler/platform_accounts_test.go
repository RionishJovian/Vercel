package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestPlatformAccountsAndSessionPresence(t *testing.T) {
	db := harness_testdb(t)
	service, _ := auth.NewService("platform-presence-test-secret-not-production", time.Hour, 24*time.Hour, 4)
	service.UseSessions(db)
	hash, _ := service.HashPassword("fixture-password")
	biz, foreign := insertBusiness(db), insertBusiness(db)
	makeUser := func(business string, role model.UserRole) *model.User {
		t.Helper()
		u := &model.User{BusinessID: business, UserRole: role, PasswordHash: hash, Email: randHex(8) + "@example.test"}
		if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role) VALUES($1,$2,'Presence fixture',$3,$4) RETURNING user_id`, business, u.Email, hash, role).Scan(&u.UserID); err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, viewer, developer, other := makeUser(biz, model.RoleOwner), makeUser(biz, model.RoleViewer), makeUser(biz, model.RoleDeveloper), makeUser(foreign, model.RoleOwner)
	token := func(u *model.User) string {
		t.Helper()
		a, _, err := service.StartSession(u)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	ownerToken, viewerToken, devToken := token(owner), token(viewer), token(developer)
	platform := NewPlatformHandler(database.NewPlatformBusinessRepository(db), nil, nil, db, service)
	account := NewAuthHandler(database.NewUserRepository(db), service, db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api/v1", auth.Middleware(service))
	protected.POST("/me/presence", account.Presence)
	protected.GET("/platform/accounts", auth.RequireRole(string(model.RoleDeveloper)), platform.Accounts)
	request := func(method, path, bearer, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	accounts := func() []map[string]any {
		t.Helper()
		w := request("GET", "/api/v1/platform/accounts?limit=100", devToken, "")
		if w.Code != 200 {
			t.Fatalf("accounts %d %s", w.Code, w.Body.String())
		}
		var body struct {
			Accounts []map[string]any `json:"accounts"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Accounts
	}
	find := func(id string) map[string]any {
		t.Helper()
		for _, a := range accounts() {
			if a["user_id"] == id {
				return a
			}
		}
		t.Fatalf("missing user %s", id)
		return nil
	}
	beat := func(bearer string, visible bool, idle int) {
		t.Helper()
		w := request("POST", "/api/v1/me/presence", bearer, fmt.Sprintf(`{"visible":%v,"idle_seconds":%d}`, visible, idle))
		if w.Code != 204 {
			t.Fatalf("presence %d %s", w.Code, w.Body.String())
		}
	}
	t.Run("developer-only bounded access", func(t *testing.T) {
		for _, bearer := range []string{ownerToken, viewerToken} {
			if w := request("GET", "/api/v1/platform/accounts", bearer, ""); w.Code != 403 {
				t.Fatal(w.Code)
			}
		}
		if w := request("GET", "/api/v1/platform/accounts", "", ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
		if w := request("GET", "/api/v1/platform/accounts?limit=101", devToken, ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
		w := request("GET", "/api/v1/platform/accounts?limit=1&offset=1", devToken, "")
		var b struct {
			Accounts []map[string]any `json:"accounts"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &b)
		if w.Code != 200 || len(b.Accounts) != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, a := range accounts() {
			for _, key := range []string{"password_hash", "session_id", "refresh_digest", "password_digest"} {
				if _, ok := a[key]; ok {
					t.Fatal("secret exposed", key)
				}
			}
		}
	})
	t.Run("real heartbeat online idle offline and no spoofed account", func(t *testing.T) {
		if find(owner.UserID)["presence"] != "unobserved" {
			t.Fatal("invented historical presence")
		}
		beat(ownerToken, true, 0)
		spoof := request("POST", "/api/v1/me/presence", ownerToken, fmt.Sprintf(`{"visible":true,"idle_seconds":0,"user_id":"%s","business_id":"%s"}`, other.UserID, foreign))
		if spoof.Code != 204 {
			t.Fatal(spoof.Code)
		}
		if find(owner.UserID)["presence"] != "online" {
			t.Fatal("online not reported")
		}
		if find(other.UserID)["last_seen_at"] != nil {
			t.Fatal("foreign heartbeat changed")
		}
		beat(ownerToken, false, 0)
		if _, err := db.Exec(`UPDATE auth_sessions SET last_visible_at=clock_timestamp()-interval '91 seconds' WHERE user_id=$1`, owner.UserID); err != nil {
			t.Fatal(err)
		}
		if find(owner.UserID)["presence"] != "idle" {
			t.Fatal("hidden tab not idle")
		}
		beat(ownerToken, true, 600)
		// Last activity is monotonic across tab activity updates. Advance the stored
		// observation to model a genuine idle period, rather than backdating it via a client.
		if _, err := db.Exec(`UPDATE auth_sessions SET last_active_at=clock_timestamp()-interval '6 minutes' WHERE user_id=$1`, owner.UserID); err != nil {
			t.Fatal(err)
		}
		if find(owner.UserID)["presence"] != "idle" {
			t.Fatal("idle not reported")
		}
		second := token(owner)
		beat(second, true, 0)
		if find(owner.UserID)["presence"] != "online" {
			t.Fatal("active second session ignored")
		}
		if err := service.RevokeSession(second); err != nil {
			t.Fatal(err)
		}
		if find(owner.UserID)["presence"] != "idle" {
			t.Fatal("revoked session counted")
		}
		if _, err := db.Exec(`UPDATE auth_sessions SET last_seen_at=clock_timestamp()-interval '91 seconds' WHERE user_id=$1`, owner.UserID); err != nil {
			t.Fatal(err)
		}
		if find(owner.UserID)["presence"] != "offline" {
			t.Fatal("stale heartbeat counted")
		}
		for _, body := range []string{`{}`, `{"visible":true,"idle_seconds":-1}`, `{"visible":true,"idle_seconds":86401}`} {
			if w := request("POST", "/api/v1/me/presence", viewerToken, body); w.Code != 400 {
				t.Fatal(w.Code)
			}
		}
		if w := request("POST", "/api/v1/me/presence", second, `{"visible":true,"idle_seconds":0}`); w.Code != 401 {
			t.Fatal("revoked session accepted", w.Code)
		}
	})
	t.Run("legacy annual expiry and unknown activation are honest", func(t *testing.T) {
		if _, err := db.Exec(`INSERT INTO account_entitlements(user_id,business_id,tier,tier_expires_at) VALUES($1,$2,'annual',clock_timestamp()+interval '365 days') ON CONFLICT(user_id) DO UPDATE SET tier='annual',tier_expires_at=excluded.tier_expires_at`, owner.UserID, biz); err != nil {
			t.Fatal(err)
		}
		a := find(owner.UserID)
		p := a["pass"].(map[string]any)
		if p["kind"] != "annual" || p["state"] != "active" || p["activated_at"] != nil {
			t.Fatal(p)
		}
		if a["permissions"].(map[string]any)["voucher_issuance"] != true || find(viewer.UserID)["permissions"].(map[string]any)["voucher_issuance"] != false {
			t.Fatal("role permissions wrong")
		}
		if _, err := db.Exec(`INSERT INTO business_metered_quotas(business_id,purchased_extra_slots,ai_token_balance) VALUES($1,2,47000) ON CONFLICT(business_id) DO UPDATE SET ai_token_balance=47000,purchased_extra_slots=2`, biz); err != nil {
			t.Fatal(err)
		}
		l := find(owner.UserID)["limits"].(map[string]any)
		if l["ai_token_balance"] != float64(47000) || l["scope"] != "business" {
			t.Fatal(l)
		}
	})
	t.Run("paid approval dates and shared subscription expiry", func(t *testing.T) {
		for _, name := range []string{"20260930070000_store_transaction_core.sql", "20261001160000_subscription_trials.sql"} {
			b, err := os.ReadFile("../../migrations/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(string(b)); err != nil {
				t.Fatal(err)
			}
		}
		approved := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
		expiry := approved.Add(365 * 24 * time.Hour)
		if _, err := db.Exec(`INSERT INTO service_purchases(business_id,user_id,service_key,service_label,price_mmk,payment_method,status,idempotency_key,decided_at,activated_expires_at) VALUES($1,$2,'annual','Annual',10000,'magic_coins','approved',$3,$4,$5)`, biz, owner.UserID, randHex(10), approved, expiry); err != nil {
			t.Fatal(err)
		}
		platform.SubscriptionOnly = true
		platform.ExistingAccountPolicy = "require_payment"
		for _, u := range []*model.User{owner, viewer} {
			a := find(u.UserID)
			p := a["pass"].(map[string]any)
			if p["scope"] != "business" || p["kind"] != "annual" || accountTime(p["activated_at"]) == nil || !accountTime(p["activated_at"]).Equal(approved) || !accountTime(p["expires_at"]).Equal(expiry) {
				t.Fatal(p)
			}
			if a["subscription"].(map[string]any)["state"] != "paid" {
				t.Fatal(a)
			}
		}
		if _, err := db.Exec(`UPDATE service_purchases SET activated_expires_at=clock_timestamp()-interval '1 second' WHERE business_id=$1`, biz); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE business_subscription_trials SET trial_started_at=now()-interval '15 days',trial_expires_at=now()-interval '1 day' WHERE business_id=$1`, biz); err != nil {
			t.Fatal(err)
		}
		if find(owner.UserID)["subscription"].(map[string]any)["access"] != false {
			t.Fatal("expired access shown as active")
		}
		if find(owner.UserID)["permissions"].(map[string]any)["router_management"] != false {
			t.Fatal("expired subscription shown as permitted")
		}
	})
}
