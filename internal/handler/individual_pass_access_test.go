package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestIndividualClientPassWorkflow(t *testing.T) {
	db := harness_testdb(t)
	service, _ := auth.NewService("fixture-client-model-test-secret", time.Hour, 24*time.Hour, 4)
	service.UseSessions(db)
	biz, foreign := insertBusiness(db), insertBusiness(db)
	makeUser := func(b string, role model.UserRole) *model.User {
		u := &model.User{BusinessID: b, UserRole: role, PasswordHash: "fixture", Email: randHex(8) + "@example.test"}
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture client','fixture',$3)RETURNING user_id`, b, u.Email, role).Scan(&u.UserID); e != nil {
			t.Fatal(e)
		}
		return u
	}
	primary, secondary, other, dev := makeUser(biz, model.RoleOwner), makeUser(biz, model.RoleAdmin), makeUser(foreign, model.RoleOwner), makeUser(biz, model.RoleDeveloper)
	if _, e := db.Exec(`INSERT INTO business_app_primary_users(business_id,user_id)VALUES($1,$2)`, foreign, other.UserID); e != nil {
		t.Fatal(e)
	}
	token := func(u *model.User) string {
		a, _, e := service.StartSession(u)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	primaryToken, secondaryToken, otherToken, devToken := token(primary), token(secondary), token(other), token(dev)
	a := NewAuthHandler(database.NewUserRepository(db), service, db)
	a.IndividualPassesEnabled = true
	p := NewPlatformHandler(database.NewPlatformBusinessRepository(db), nil, nil, db, service)
	p.IndividualPassesEnabled = true
	r := gin.New()
	managementCalls := 0
	g := r.Group("/api/v1", auth.Middleware(service), IndividualPassAccess(db, true))
	g.GET("/me/app-pass", a.AppPass)
	g.GET("/me/app-pass/renewals", a.AccountRenewals)
	g.POST("/me/app-pass/renewals", a.RequestAccountRenewal)
	g.POST("/routers", func(c *gin.Context) { managementCalls++; c.Status(204) })
	g.GET("/platform/accounts", p.Accounts)
	g.PUT("/platform/accounts/:userId/authorized-login", p.AuthorizeClientLogin)
	g.POST("/platform/account-renewals/:id/decision", p.DecideAccountRenewal)
	g.POST("/team", auth.RequireRole(string(model.RoleDeveloper)), func(c *gin.Context) { c.Status(204) })
	r.POST("/api/v1/auth/invitations/accept", ClientStaffInvitationsUnavailable)
	// This synthetic guest callback proves only that the app-pass middleware does
	// not gate this public route. Hardware internet persistence is a separate test.
	r.POST("/api/v1/guest/redeem", func(c *gin.Context) { c.Status(204) })
	req := func(method, path, bearer, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		q := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		q.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			q.Header.Set("Authorization", "Bearer "+bearer)
		}
		r.ServeHTTP(w, q)
		return w
	}
	check := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("want %d got %d %s", want, w.Code, w.Body)
		}
	}
	endpoint := "/api/v1/platform/accounts/" + primary.UserID + "/authorized-login"
	check(req("PUT", endpoint, devToken, `{"expected_primary_user_id":"","reason":"Reviewed fixture client"}`), 200)
	check(req("PUT", endpoint, devToken, `{"expected_primary_user_id":"","reason":"Stale fixture selection"}`), 409)
	check(req("GET", "/api/v1/me/app-pass", primaryToken, ""), 200)
	check(req("POST", "/api/v1/routers", primaryToken, "{}"), 402)
	check(req("POST", "/api/v1/guest/redeem", "", "{}"), 204)
	check(req("POST", "/api/v1/auth/invitations/accept", "", "{}"), 410)
	check(req("POST", "/api/v1/me/app-pass/renewals", secondaryToken, `{"plan":"monthly"}`), 403)
	w := req("POST", "/api/v1/me/app-pass/renewals", primaryToken, `{"plan":"monthly"}`)
	check(w, 200)
	var pending map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &pending); e != nil {
		t.Fatal(e)
	}
	id := pending["id"].(string)
	w = req("POST", "/api/v1/me/app-pass/renewals", primaryToken, `{"plan":"monthly"}`)
	check(w, 200)
	var retry map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &retry)
	if retry["id"] != id {
		t.Fatal("retry duplicated request")
	}
	check(req("POST", "/api/v1/me/app-pass/renewals", primaryToken, `{"plan":"annual"}`), 409)
	w = req("GET", "/api/v1/me/app-pass/renewals", otherToken, "")
	check(w, 200)
	if bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatal("renewal leaked to foreign account")
	}
	check(req("POST", "/api/v1/platform/account-renewals/"+id+"/decision", devToken, `{"approve":true}`), 200)
	check(req("POST", "/api/v1/platform/account-renewals/"+id+"/decision", devToken, `{"approve":true}`), 409)
	check(req("POST", "/api/v1/platform/account-renewals/"+id+"/decision", primaryToken, `{"approve":false}`), 403)
	check(req("POST", "/api/v1/team", primaryToken, "{}"), 403)
	check(req("POST", "/api/v1/routers", primaryToken, "{}"), 204)
	check(req("PUT", endpoint, primaryToken, `{"expected_primary_user_id":"","reason":"Client self-selection"}`), 403)
	if allowed, e := database.AccountPassBusinessAccess(context.Background(), db, biz, time.Now()); e != nil || !allowed {
		t.Fatal("paid background access denied", allowed, e)
	}
	check(req("POST", "/api/v1/routers", secondaryToken, "{}"), 402)
	check(req("GET", "/api/v1/platform/accounts", devToken, ""), 200)
	var events int
	if e := db.QueryRow(`SELECT count(*)FROM app_account_pass_events WHERE user_id=$1`, primary.UserID).Scan(&events); e != nil || events != 1 {
		t.Fatalf("replayed activation %d %v", events, e)
	}
	if _, e := db.Exec(`UPDATE app_account_passes SET expires_at=now()-interval '1 second',activated_at=now()-interval '1 month' WHERE user_id=$1`, primary.UserID); e != nil {
		t.Fatal(e)
	}
	check(req("POST", "/api/v1/routers", primaryToken, "{}"), 402)
	check(req("GET", "/api/v1/me/app-pass", primaryToken, ""), 200)
	check(req("POST", "/api/v1/guest/redeem", "", "{}"), 204)
	check(req("POST", "/api/v1/routers", devToken, "{}"), 204)
	if managementCalls != 2 {
		t.Fatalf("denied management handler executed: %d", managementCalls)
	}
}

func TestClientBusinessCapacitySerializesCreation(t *testing.T) {
	db := harness_testdb(t)
	if _, e := db.Exec(`INSERT INTO app_account_pass_rollouts(rollout_key,starts_at,transition_days,enforcement_enabled) VALUES('individual-v1',now(),7,true) ON CONFLICT(rollout_key) DO UPDATE SET enforcement_enabled=true`); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE app_account_pass_rollouts SET enforcement_enabled=false WHERE rollout_key='individual-v1'`)
	})
	for _, resource := range []string{"sites", "devices"} {
		t.Run(resource, func(t *testing.T) {
			biz := insertBusiness(db)
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					var e error
					if resource == "sites" {
						_, e = db.Exec(`INSERT INTO sites(business_id,name)VALUES($1,$2)`, biz, fmt.Sprintf("site-%d", i))
					} else {
						_, e = db.Exec(`INSERT INTO devices(business_id,hardware_name)VALUES($1,$2)`, biz, fmt.Sprintf("router-%d", i))
					}
					results <- e
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			ok, denied := 0, 0
			for e := range results {
				if e == nil {
					ok++
				} else if bytes.Contains([]byte(e.Error()), []byte("One ")) {
					denied++
				} else {
					t.Fatal(e)
				}
			}
			if ok != 1 || denied != 1 {
				t.Fatalf("capacity race: %d admitted %d denied", ok, denied)
			}
		})
	}
}
