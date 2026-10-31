package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
)

func TestDBStoreAddonCatalogAndRequests(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	b, e := os.ReadFile("../../migrations/20261031100000_store_addons.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(b)); e != nil {
		t.Fatal(e)
	}
	svc, _ := auth.NewService("store-addon-test-signing-secret", time.Hour, time.Hour, 4)
	svc.UseSessions(db)
	mkBiz := func(name string) string {
		var id string
		if e := db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier)VALUES($1,$2,'SMALL_WISP')RETURNING business_id`, name, name+"@fixture.test").Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	bizA, bizB := mkBiz("Addon A"), mkBiz("Addon B")
	token := func(biz string, role model.UserRole) string {
		u := &model.User{BusinessID: biz, UserRole: role, PasswordHash: "fixture"}
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture',$3)RETURNING user_id`, biz, string(role)+biz+"@fixture.test", string(role)).Scan(&u.UserID); e != nil {
			t.Fatal(e)
		}
		tok, _, e := svc.StartSession(u)
		if e != nil {
			t.Fatal(e)
		}
		return tok
	}
	owner, viewer, ownerB, dev := token(bizA, model.RoleOwner), token(bizA, model.RoleViewer), token(bizB, model.RoleOwner), token(bizA, model.RoleDeveloper)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewStoreAddonHandler(db)
	h.RegisterTenant(r.Group("/api/v1", auth.Middleware(svc)))
	h.RegisterPlatform(r.Group("/api/v1/platform", auth.Middleware(svc), auth.RequireRole(string(model.RoleDeveloper))))

	call := func(method, path, tok, body string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, "https://app.test"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body)
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}

	item := `{"addon_key":"extra_router","kind":"router_slot","name_en":"Extra router","name_my":"Router တိုးမည်","price_mmk":5000,"period":"monthly"}`
	call("POST", "/api/v1/platform/store/addons", "", item, 401)
	call("POST", "/api/v1/platform/store/addons", owner, item, 403)
	created := call("POST", "/api/v1/platform/store/addons", dev, item, 201)["addon"].(map[string]any)
	id := created["id"].(string)
	call("POST", "/api/v1/platform/store/addons", dev, item, 409)
	call("POST", "/api/v1/platform/store/addons", dev, `{"addon_key":"BAD","kind":"x"}`, 400)
	hidden := call("POST", "/api/v1/platform/store/addons", dev, strings.Replace(strings.Replace(item, "extra_router", "hidden_item", 1), `"period":"monthly"`, `"period":"annual","active":false`, 1), 201)["addon"].(map[string]any)

	list := call("GET", "/api/v1/store/addons", viewer, "", 200)
	if got := len(list["addons"].([]any)); got != 1 {
		t.Fatalf("an owner-facing list must show only active add-ons, got %d", got)
	}
	call("GET", "/api/v1/store/addons", "", "", 401)

	base := "/api/v1/store/addons/" + id + "/requests"
	body := `{"bank_reference":"KBZ 123456","idempotency_key":"key-0001-aaaa"}`
	call("POST", base, viewer, body, 403)
	call("POST", base, owner, `{"bank_reference":"x","idempotency_key":"short"}`, 400)
	first := call("POST", base, owner, body, 201)["request"].(map[string]any)
	again := call("POST", base, owner, body, 200)["request"].(map[string]any)
	if first["id"] != again["id"] {
		t.Fatal("a replayed idempotency key must return the same request")
	}
	call("POST", base, owner, `{"bank_reference":"KBZ 999999","idempotency_key":"key-0002-bbbb"}`, 409)
	call("POST", "/api/v1/store/addons/"+hidden["id"].(string)+"/requests", owner, `{"bank_reference":"KBZ 123456","idempotency_key":"key-0003-cccc"}`, 404)

	mine := call("GET", "/api/v1/store/addons", owner, "", 200)
	if got := len(mine["requests"].([]any)); got != 1 {
		t.Fatalf("expected the business's own request, got %d", got)
	}
	other := call("GET", "/api/v1/store/addons", ownerB, "", 200)
	if got := len(other["requests"].([]any)); got != 0 {
		t.Fatalf("another business must not see this request, got %d", got)
	}
	// The same key from another business is a different request.
	call("POST", base, ownerB, body, 201)

	pending := call("GET", "/api/v1/platform/store/requests?status=pending", dev, "", 200)["requests"].([]any)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending requests, got %d", len(pending))
	}
	call("POST", "/api/v1/platform/store/requests/"+first["id"].(string)+"/decision", owner, `{"decision":"approve"}`, 403)
	call("POST", "/api/v1/platform/store/requests/"+first["id"].(string)+"/decision", dev, `{"decision":"maybe"}`, 400)
	decided := call("POST", "/api/v1/platform/store/requests/"+first["id"].(string)+"/decision", dev, `{"decision":"approve","note":"paid"}`, 200)["request"].(map[string]any)
	if decided["status"] != "approved" {
		t.Fatalf("status %v", decided["status"])
	}
	call("POST", "/api/v1/platform/store/requests/"+first["id"].(string)+"/decision", dev, `{"decision":"reject"}`, 409)
	call("POST", "/api/v1/platform/store/requests/11111111-1111-4111-8111-111111111111/decision", dev, `{"decision":"reject"}`, 404)
	// After a decision the business may raise a new request for the same add-on.
	call("POST", base, owner, `{"bank_reference":"KBZ 777777","idempotency_key":"key-0004-dddd"}`, 201)
}
