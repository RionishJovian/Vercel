package server

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestIndividualProductionRouteChain(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	for _, path := range []string{"../../migrations/20261024030000_app_account_passes.sql", "../../migrations/20261024040000_individual_pass_rollout.sql", "../../migrations/20261029090000_account_pass_checkout.sql", "../../migrations/20261101090000_account_trials.sql"} {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	if e := database.CheckIndividualPassSchema(db); e == nil {
		t.Fatal("unarmed enforcement accepted")
	}
	service, _ := auth.NewService("disposable-individual-route-secret", time.Hour, 24*time.Hour, 4)
	service.UseSessions(db)
	makeUser := func(role model.UserRole) *model.User {
		u := &model.User{UserRole: role, Email: string(role) + "@fixture.test", PasswordHash: "fixture"}
		if e := db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier)VALUES('Fixture',$1,'SMALL_WISP')RETURNING business_id`, u.Email).Scan(&u.BusinessID); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture',$3,$4)RETURNING user_id`, u.BusinessID, u.Email, u.PasswordHash, u.UserRole).Scan(&u.UserID); e != nil {
			t.Fatal(e)
		}
		return u
	}
	owner, dev := makeUser(model.RoleOwner), makeUser(model.RoleDeveloper)
	legacyOperator := &model.User{BusinessID: dev.BusinessID, Email: "legacy-platform@fixture.test", UserRole: model.RoleOwner, PasswordHash: "fixture"}
	if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Legacy platform','fixture','OWNER')RETURNING user_id`, dev.BusinessID, legacyOperator.Email).Scan(&legacyOperator.UserID); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO business_app_primary_users(business_id,user_id)VALUES($1,$2)`, dev.BusinessID, legacyOperator.UserID); e != nil {
		t.Fatal(e)
	}
	token := func(u *model.User) string {
		v, _, e := service.StartSession(u)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	ownerToken, devToken := token(owner), token(dev)
	legacyOperatorToken := token(legacyOperator)
	a := handler.NewAuthHandler(database.NewUserRepository(db), service, db)
	a.IndividualPassesEnabled = true
	p := handler.NewPlatformHandler(database.NewPlatformBusinessRepository(db), nil, nil, db, service)
	p.IndividualPassesEnabled = true
	routers := handler.NewRouterHandler(db, database.NewDeviceRepository(db), database.NewDeviceAllowanceRepository(db), nil)
	routers.SubscriptionOnly = true
	sites := handler.NewSiteHandler(db, database.NewSiteRepository(db), database.NewDeviceAllowanceRepository(db))
	sites.SubscriptionOnly = true
	legacyStore := handler.NewStoreHandler(db, database.NewStorePurchaseRepository(db), database.NewPricingRepository(db), nil, "")
	legacyStore.IndividualPassesEnabled = true
	r := gin.New()
	setupRoutes(
		r,           // r
		service,     // authService
		a,           // authHandler
		routers,     // routerHandler
		nil,         // telemetryHandler
		nil,         // incidentHandler
		nil,         // webhookHandler
		nil,         // voucherHandler
		nil,         // portalHandler
		nil,         // billingHandler
		legacyStore, // storeHandler
		nil,         // guestWifiHandler
		nil,         // monitorHandler
		nil,         // fleetHandler
		nil,         // topologyHandler
		nil,         // vpsHandler
		p,           // platformHandler
		nil,         // securityHandler
		nil,         // backupHandler
		nil,         // teamHandler
		nil,         // pulseHandler
		nil,         // incidentActionHandler
		nil,         // magicCoinHandler
		sites,       // siteHandler
		nil,         // connHandler
		nil,         // allowHandler
		nil,         // syslogHandler
		nil,         // templateHandler
		nil,         // resellerHandler
		nil,         // scanHandler
		nil,         // usageHandler
		nil,         // magicDudeHandler
		nil,         // historyHandler
		nil,         // unlockHandler
		nil,         // i18nH
		nil,         // msg
		nil,         // cache
		db,          // db
		nil,         // tenantMW
	)
	registerGuestVoucherEndpoints(r, service, db, &handler.GuestVoucherHandler{}, handler.IndividualPassAccess(db, true))
	req := func(method, path, bearer, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		q := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		q.Header.Set("Content-Type", "application/json")
		q.Header.Set("Origin", "https://example.com")
		q.Header.Set("X-MM-CSRF", "1")
		if bearer != "" {
			q.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body)
		}
		return w
	}
	req("GET", "/api/v1/routers", legacyOperatorToken, "", 402)
	req("PUT", "/api/v1/platform/accounts/"+legacyOperator.UserID+"/pass", devToken, `{"action":"activate","plan":"monthly","revision":0}`, 409)
	for _, path := range []string{"/api/v1/routers", "/api/v1/sites", "/api/v1/guest-vouchers/config"} {
		req("GET", path, ownerToken, "", 402)
	}
	req("POST", "/api/v1/guest-vouchers", ownerToken, "{}", 402)
	req("POST", "/api/v1/vps/peers/11111111-1111-4111-8111-111111111111/enroll", ownerToken, "{}", 402)
	for _, path := range []string{"/api/v1/me", "/api/v1/me/app-pass", "/api/v1/me/app-pass/renewals", "/api/v1/subscription"} {
		req("GET", path, ownerToken, "", 200)
	}
	req("POST", "/api/v1/auth/invitations/accept", "", "{}", 410)
	req("POST", "/api/v1/portal/11111111-1111-4111-8111-111111111111/guest-voucher", "", "{}", 400)
	req("PUT", "/api/v1/platform/accounts/"+owner.UserID+"/pass", ownerToken, `{"action":"activate","plan":"monthly","revision":0}`, 403)
	req("PUT", "/api/v1/platform/accounts/"+owner.UserID+"/pass", devToken, `{"action":"activate","plan":"monthly","revision":0}`, 200)
	req("GET", "/api/v1/routers", ownerToken, "", 200)
	req("POST", "/api/v1/team", ownerToken, "{}", 403)
	req("POST", "/api/v1/store/purchases", ownerToken, "{}", 409)
	req("POST", "/api/v1/magic-coins/adjust", ownerToken, "{}", 409)
	req("POST", "/api/v1/team/invitations", ownerToken, "{}", 403)
	if ok, e := database.AccountPassBusinessAccess(context.Background(), db, owner.BusinessID, time.Now()); e != nil || !ok {
		t.Fatal("paid worker access", ok, e)
	}
	if _, e := db.Exec(`UPDATE app_account_passes SET activated_at=now()-interval '2 months',expires_at=now()-interval '1 second' WHERE user_id=$1`, owner.UserID); e != nil {
		t.Fatal(e)
	}
	req("GET", "/api/v1/routers", ownerToken, "", 402)
	req("POST", "/api/v1/portal/11111111-1111-4111-8111-111111111111/guest-voucher", "", "{}", 400)
	if ok, e := database.AccountPassBusinessAccess(context.Background(), db, owner.BusinessID, time.Now()); e != nil || ok {
		t.Fatal("expired worker access", ok, e)
	}
	w := req("POST", "/api/v1/me/app-pass/renewals", ownerToken, `{"plan":"annual"}`, 200)
	var renewal map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &renewal); e != nil {
		t.Fatal(e)
	}
	req("GET", "/api/v1/platform/account-renewals", devToken, "", 200)
	req("POST", "/api/v1/platform/account-renewals/"+renewal["id"].(string)+"/decision", devToken, `{"approve":true}`, 200)
	req("GET", "/api/v1/routers", ownerToken, "", 200)
	// Arm the actual deployment SQL, then prove startup readiness and capacity
	// responses through the same production route chain.
	script, e := os.ReadFile("../../deploy/activate-individual-passes.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(script)); e != nil {
		t.Fatal(e)
	}
	if e := database.CheckIndividualPassSchema(db); e != nil {
		t.Fatal(e)
	}
	req("POST", "/api/v1/sites", ownerToken, `{"name":"Fixture site"}`, 201)
	req("POST", "/api/v1/sites", ownerToken, `{"name":"Second site"}`, 409)
	req("POST", "/api/v1/routers", ownerToken, `{"hardware_name":"Fixture router","deployment_layer":"MAIN_GATEWAY","vendor":"mikrotik","category":"router","transport":"tunnel"}`, 201)
	req("POST", "/api/v1/routers", ownerToken, `{"hardware_name":"Second router","deployment_layer":"MAIN_GATEWAY","vendor":"mikrotik","category":"router","transport":"tunnel"}`, 409)
	if _, e := db.Exec(`INSERT INTO pricing_promo(id,active,monthly_standard_mmk,annual_standard_mmk)VALUES(true,false,1000,10000) ON CONFLICT(id) DO UPDATE SET active=false,monthly_standard_mmk=1000,annual_standard_mmk=10000`); e != nil {
		t.Fatal(e)
	}
	price := req("GET", "/api/v1/pricing", "", "", 200)
	var publicPolicy struct {
		Policy struct {
			Mode    string `json:"mode"`
			Routers int    `json:"routers"`
			Same    bool   `json:"same_features"`
		} `json:"account_policy"`
		Tiers []any `json:"tiers"`
	}
	if e := json.Unmarshal(price.Body.Bytes(), &publicPolicy); e != nil || publicPolicy.Policy.Mode != "individual_pass" || publicPolicy.Policy.Routers != 1 || !publicPolicy.Policy.Same || len(publicPolicy.Tiers) != 0 {
		t.Fatal("public offer differs from enforced client policy", price.Body.String(), e)
	}
	// Actual password login remains available after expiry, including a newly
	// registered client that must not inherit the historical transition.
	registered := req("POST", "/api/v1/auth/register", "", `{"company_name":"New client","full_name":"Fixture","email":"new-client@fixture.test","password":"fixture-only-password"}`, 201)
	var client model.AuthResponse
	if e := json.Unmarshal(registered.Body.Bytes(), &client); e != nil || client.Business.DeviceQuotaLimit != 1 {
		t.Fatal("new client capacity", e, registered.Body.String())
	}
	var historicalGrace int
	if e := db.QueryRow(`SELECT count(*) FROM app_account_pass_transitions WHERE user_id=$1`, client.User.UserID).Scan(&historicalGrace); e != nil || historicalGrace != 0 {
		t.Fatal("new client received old transition", historicalGrace, e)
	}
	req("PUT", "/api/v1/platform/accounts/"+client.User.UserID+"/pass", devToken, `{"action":"activate","plan":"monthly","revision":0}`, 200)
	if _, e := db.Exec(`UPDATE app_account_passes SET activated_at=now()-interval '2 months',expires_at=now()-interval '1 second' WHERE user_id=$1`, client.User.UserID); e != nil {
		t.Fatal(e)
	}
	login := req("POST", "/api/v1/auth/login", "", `{"email":"new-client@fixture.test","password":"fixture-only-password"}`, 200)
	fresh := ""
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "mm_access" {
			fresh = cookie.Value
		}
	}
	if fresh == "" {
		t.Fatal("expired client cannot establish a session")
	}
	req("GET", "/api/v1/me", fresh, "", 200)
	req("GET", "/api/v1/routers", fresh, "", 402)
	req("POST", "/api/v1/me/app-pass/renewals", fresh, `{"plan":"monthly"}`, 200)
	// Single connection makes each rollback-only catalog mutation visible to the
	// readiness function. It must reject missing RLS, triggers or tenant writes.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, mutation := range []string{`ALTER TABLE app_account_pass_renewals NO FORCE ROW LEVEL SECURITY`, `ALTER TABLE sites DISABLE TRIGGER app_site_capacity`, `GRANT UPDATE ON app_account_passes TO authenticated`} {
		if _, e := db.Exec("BEGIN; " + mutation); e != nil {
			t.Fatal(e)
		}
		e := database.CheckIndividualPassSchema(db)
		if _, rollback := db.Exec("ROLLBACK"); rollback != nil {
			t.Fatal(rollback)
		}
		if e == nil {
			t.Fatalf("unsafe schema accepted: %s", mutation)
		}
	}
}
