package server

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
)

// Never touches the configured production DB. Every schema/write below is in
// a cryptographically named disposable database, dropped even on test failure.
func subscriptionHTTPDatabase(t *testing.T) *database.DB {
	t.Helper()
	base := os.Getenv("MM_TEST_DB_URL")
	if base == "" {
		t.Skip("disposable DB URL required")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("invalid test DB URL")
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal("test admin connection unavailable")
	}
	b := make([]byte, 12)
	if _, err = rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "mega_magic_subhttp_" + hex.EncodeToString(b)
	if _, err = admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		admin.Close()
		t.Fatal("throwaway database creation failed")
	}
	var pool *sql.DB
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, e := admin.Exec(`DROP DATABASE "` + name + `"`); e != nil {
			t.Error("throwaway database cleanup failed")
		}
		admin.Close()
	})
	u.Path = "/" + name
	pool, err = sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal("throwaway connection failed")
	}
	pool.SetMaxOpenConns(4)
	db := &database.DB{DB: pool}
	for _, path := range []string{"../../database/init.sql", "../../migrations/20260930220000_auth_sessions.sql", "../../migrations/20260930070000_store_transaction_core.sql", "../../migrations/20261001160000_subscription_trials.sql"} {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(data)); e != nil {
			t.Fatalf("test schema %s failed: %v", path, e)
		}
	}
	return db
}

func TestDB_SubscriptionAuthenticatedHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := subscriptionHTTPDatabase(t)
	service, err := auth.NewService("disposable-subscription-http-test-secret", time.Hour, 24*time.Hour, 4)
	if err != nil {
		t.Fatal(err)
	}
	service.UseSessions(db)
	a := handler.NewAuthHandler(database.NewUserRepository(db), service, db)
	a.SubscriptionOnly = true
	a.SubscriptionExistingPolicy = "require_payment"
	store := handler.NewStoreHandler(db, database.NewStorePurchaseRepository(db), database.NewPricingRepository(db), nil, "")
	store.SubscriptionOnly = true
	routers := handler.NewRouterHandler(db, database.NewDeviceRepository(db), database.NewDeviceAllowanceRepository(db), nil)
	routers.SubscriptionOnly = true
	r := gin.New()
	// Actual production route declarations and middleware ordering; unused
	// feature handlers are inert placeholders, not a second test-only router.
	setupRoutes(r, service, a, routers, &handler.TelemetryHandler{}, &handler.IncidentHandler{}, &handler.WebhookHandler{}, &handler.VoucherHandler{}, &handler.PortalHandler{}, &handler.BillingHandler{}, store, &handler.GuestWifiHandler{}, &handler.MonitorHandler{}, &handler.FleetHandler{}, &handler.TopologyHandler{}, &handler.VPSHandler{}, &handler.PlatformHandler{}, &handler.SecurityHandler{}, &handler.BackupHandler{}, &handler.TeamHandler{}, &handler.PulseHandler{}, &handler.IncidentActionHandler{}, &handler.MagicCoinHandler{}, &handler.SiteHandler{}, &handler.RouterConnectionHandler{}, &handler.DeviceAllowanceHandler{}, &handler.SyslogHandler{}, &handler.TerminalTemplateHandler{}, &handler.VoucherResellerHandler{}, &handler.FleetScanRunHandler{}, &handler.AIUsageHandler{}, &handler.MagicDudeHandler{}, &handler.TerminalHistoryHandler{}, &handler.UnlockKeyHandler{}, &handler.I18nHandler{}, nil, nil, db, nil)
	var cookies []*http.Cookie
	request := func(method, path string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "https://subscription.example.test"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://subscription.example.test")
		req.Header.Set("X-MM-CSRF", "1")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: status %d want %d", method, path, w.Code, want)
		}
		return w
	}
	email := "subscription-" + time.Now().UTC().Format("150405.000000000") + "@example.test"
	w := request("POST", "/api/v1/auth/register", gin.H{"company_name": "Disposable subscription", "full_name": "Test owner", "email": email, "password": "disposable-test-password"}, 201)
	cookies = w.Result().Cookies()
	var registered model.AuthResponse
	if err = json.Unmarshal(w.Body.Bytes(), &registered); err != nil {
		t.Fatal("invalid registration response")
	}
	if registered.Token != "" || len(cookies) < 2 {
		t.Fatal("browser authentication did not use private cookies")
	}
	biz := registered.Business.BusinessID
	var start, expiry time.Time
	if err = db.QueryRow(`SELECT trial_started_at,trial_expires_at FROM business_subscription_trials WHERE business_id=$1`, biz).Scan(&start, &expiry); err != nil || expiry.Sub(start) != 336*time.Hour {
		t.Fatal("registration trial invalid", err)
	}
	request("GET", "/api/v1/subscription", nil, 200)
	request("GET", "/api/v1/routers", nil, 200)
	request("GET", "/api/v1/store/checkout-options", nil, 200)
	// Login/refresh must not restart the one-time trial.
	w = request("POST", "/api/v1/auth/login", gin.H{"email": email, "password": "disposable-test-password"}, 200)
	cookies = w.Result().Cookies()
	var repeated time.Time
	if err = db.QueryRow(`SELECT trial_expires_at FROM business_subscription_trials WHERE business_id=$1`, biz).Scan(&repeated); err != nil || !repeated.Equal(expiry) {
		t.Fatal("login extended trial")
	}
	if _, err = db.Exec(`UPDATE business_subscription_trials SET trial_started_at=now()-interval '336 hours',trial_expires_at=now() WHERE business_id=$1`, biz); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/v1/routers", nil, 402)
	request("GET", "/api/v1/me", nil, 200)
	request("GET", "/api/v1/store/purchases", nil, 200)
	request("GET", "/api/v1/store/checkout-options", nil, 200)
	var bank string
	if err = db.QueryRow(`INSERT INTO platform_service_bank_accounts(slot,holder_name,bank_name,account_number) VALUES(1,'Test','Test','TEST') ON CONFLICT(slot) DO UPDATE SET enabled=true RETURNING id`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO pricing_promo(id,active,monthly_standard_mmk,annual_standard_mmk) VALUES(true,false,1000,10000) ON CONFLICT(id) DO UPDATE SET active=false,monthly_standard_mmk=1000,annual_standard_mmk=10000`); err != nil {
		t.Fatal(err)
	}
	request("POST", "/api/v1/store/purchases", gin.H{"product_key": "magic_coins", "bank_account_id": bank, "bank_reference": "disabled", "idempotency_key": "disabled"}, 400)
	w = request("POST", "/api/v1/store/purchases", gin.H{"product_key": "monthly", "bank_account_id": bank, "bank_reference": "renew-test", "idempotency_key": "renew-test"}, 201)
	var created struct {
		Purchase model.StorePurchase `json:"purchase"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.Purchase.ID == "" {
		t.Fatal("invalid purchase response")
	}
	request("POST", "/api/v1/store/purchases/"+created.Purchase.ID+"/approve", nil, 200)
	request("POST", "/api/v1/store/purchases/"+created.Purchase.ID+"/approve", nil, 409)
	w = request("GET", "/api/v1/subscription", nil, 200)
	var status struct {
		State  string
		Access bool
	}
	if err = json.Unmarshal(w.Body.Bytes(), &status); err != nil || status.State != "paid" || !status.Access {
		t.Fatal("approved subscription did not restore access")
	}
	request("GET", "/api/v1/routers", nil, 200)
	// Browser write guards still apply with a valid, paid session.
	req := httptest.NewRequest("POST", "https://subscription.example.test/api/v1/store/purchases/"+created.Purchase.ID+"/approve", nil)
	req.Header.Set("Origin", "https://subscription.example.test")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	csrf := httptest.NewRecorder()
	r.ServeHTTP(csrf, req)
	if csrf.Code != 403 {
		t.Fatal("cookie write without CSRF header accepted")
	}
	// The recovery-route exception is not a role exception.
	if _, err = db.Exec(`UPDATE users SET user_role='VIEWER' WHERE user_id=$1`, registered.User.UserID); err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/api/v1/auth/login", gin.H{"email": email, "password": "disposable-test-password"}, 200)
	cookies = w.Result().Cookies()
	request("POST", "/api/v1/store/purchases/"+created.Purchase.ID+"/approve", nil, 403)
	if _, err = db.Exec(`UPDATE users SET user_role='OWNER' WHERE user_id=$1`, registered.User.UserID); err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/api/v1/auth/login", gin.H{"email": email, "password": "disposable-test-password"}, 200)
	cookies = w.Result().Cookies()
	ownerCookies := cookies
	w = request("POST", "/api/v1/auth/register", gin.H{"company_name": "Other disposable tenant", "full_name": "Other owner", "email": "other-" + email, "password": "disposable-test-password"}, 201)
	cookies = w.Result().Cookies()
	request("POST", "/api/v1/store/purchases/"+created.Purchase.ID+"/approve", nil, 404)
	cookies = ownerCookies
	// Suspension still overrides paid status and old valid session cookies.
	if _, err = db.Exec(`UPDATE businesses SET is_active=false WHERE business_id=$1`, biz); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/v1/subscription", nil, 401)
}
