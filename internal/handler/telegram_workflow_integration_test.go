package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
	"net/http/httptest"
	"strings"
	"testing"
)

type runtimeStub struct{ wakes int }

func (r *runtimeStub) Status(string) telegram.RuntimeStatus {
	return telegram.RuntimeStatus{Scope: "business", Receiver: "starting", Token: "unknown"}
}
func (r *runtimeStub) Wake()                                  { r.wakes++ }
func (r *runtimeStub) Probe(context.Context, string) error    { return nil }
func (r *runtimeStub) SendTest(context.Context, string) error { return nil }
func TestTelegramSavedBindingRoleAndTenantIsolation(t *testing.T) {
	db := harness_testdb(t)
	platform, customer := insertBusiness(db), insertBusiness(db)
	var dev string
	if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Admin','fixture','DEVELOPER') RETURNING user_id`, platform, randHex(8)+"@bot.fixture").Scan(&dev); e != nil {
		t.Fatal(e)
	}
	repo := database.NewBusinessTelegramBotRepository(db)
	h := NewGuestWifiHandler(db, nil, repo, nil, nil, nil, "", nil)
	rt := &runtimeStub{}
	h.EnableTelegramRuntime(rt)
	call := func(method, bid, role string, body map[string]any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("business_id", bid)
		c.Set("user_role", role)
		c.Request = httptest.NewRequest(method, "/api/v1/guest-wifi/telegram-bot", bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")
		if method == "PUT" {
			h.PutBotBinding(c)
		} else {
			h.GetBotBinding(c)
		}
		return w
	}
	body := map[string]any{"bot_token": "fixture:secret-platform-token", "target_chat_id": int64(-100123), "approver_user_id": 42, "enabled": true}
	if w := call("PUT", platform, "DEVELOPER", body); w.Code != 200 {
		t.Fatalf("platform save: %d %s", w.Code, w.Body)
	}
	if w := call("PUT", platform, "OWNER", body); w.Code != 403 {
		t.Fatalf("legacy owner edited platform token: %d", w.Code)
	}
	body["bot_token"] = "fixture:secret-customer-token"
	body["business_id"] = platform
	if w := call("PUT", customer, "OWNER", body); w.Code != 200 {
		t.Fatalf("customer save: %d %s", w.Code, w.Body)
	}
	body["bot_token"] = ""
	if w := call("PUT", platform, "DEVELOPER", body); w.Code != 200 {
		t.Fatalf("keep existing token: %d %s", w.Code, w.Body)
	}
	saved, e := repo.GetByBusiness(platform)
	if e != nil || saved.BotToken != "fixture:secret-platform-token" {
		t.Fatal("saved platform token changed or was replaced by foreign tenant")
	}
	w := call("GET", platform, "DEVELOPER", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-platform") {
		t.Fatalf("unmasked response: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"scope":"platform"`) || !strings.Contains(w.Body.String(), `"receiver":"starting"`) {
		t.Fatalf("scope/runtime missing: %s", w.Body)
	}
	if rt.wakes != 3 {
		t.Fatalf("successful saves must wake receiver, got %d", rt.wakes)
	}
}
func TestTelegramRenewalUsesSharedExactlyOnceCore(t *testing.T) {
	db := harness_testdb(t)
	adminBiz, clientBiz := insertBusiness(db), insertBusiness(db)
	makeUser := func(bid string, role model.UserRole) string {
		var id string
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture',$3)RETURNING user_id`, bid, randHex(8)+"@bot-renewal.fixture", role).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	dev, owner := makeUser(adminBiz, model.RoleDeveloper), makeUser(clientBiz, model.RoleOwner)
	var id string
	if e := db.QueryRow(`INSERT INTO app_account_pass_renewals(user_id,business_id,plan)VALUES($1,$2,'monthly')RETURNING id`, owner, clientBiz).Scan(&id); e != nil {
		t.Fatal(e)
	}
	h := NewPlatformHandler(nil, nil, nil, db, nil)
	d := AccountRenewalDecider{Handler: h, ActorID: owner}
	if e := d.DecidePurchaseRequest(clientBiz, id, true); e == nil {
		t.Fatal("customer impersonated platform approver")
	}
	d.ActorID = dev
	if e := d.DecidePurchaseRequest(adminBiz, id, true); e == nil {
		t.Fatal("wrong resolved business accepted")
	}
	if e := d.DecidePurchaseRequest(clientBiz, id, true); e != nil {
		t.Fatal(e)
	}
	if e := d.DecidePurchaseRequest(clientBiz, id, true); e == nil {
		t.Fatal("replayed approval succeeded")
	}
	var count int
	if e := db.QueryRow(`SELECT count(*) FROM app_account_passes WHERE user_id=$1 AND business_id=$2`, owner, clientBiz).Scan(&count); e != nil || count != 1 {
		t.Fatal("pass not granted exactly once")
	}
	var actor string
	if e := db.QueryRow(`SELECT decided_by FROM app_account_pass_renewals WHERE id=$1`, id).Scan(&actor); e != nil || actor != dev {
		t.Fatal("platform decision actor not recorded")
	}
}

func TestRenewalNotificationIsAfterCommitAndNotRepeated(t *testing.T) {
	db := harness_testdb(t)
	bid := insertBusiness(db)
	var uid string
	if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture','OWNER')RETURNING user_id`, bid, randHex(8)+"@notification.fixture").Scan(&uid); e != nil {
		t.Fatal(e)
	}
	h := NewAuthHandler(nil, nil, db)
	calls := 0
	h.NotifyRenewal = func(id string) {
		calls++
		var count int
		if e := db.QueryRow(`SELECT count(*) FROM app_account_pass_renewals WHERE id=$1 AND business_id=$2`, id, bid).Scan(&count); e != nil || count != 1 {
			t.Error("notification ran before the request committed")
		}
	}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("business_id", bid)
		c.Set("user_id", uid)
		c.Set("user_role", "OWNER")
		c.Request = httptest.NewRequest("POST", "/api/v1/me/app-pass/renewals", strings.NewReader(`{"plan":"monthly"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.RequestAccountRenewal(c)
		if w.Code != 200 {
			t.Fatalf("renewal: %d %s", w.Code, w.Body)
		}
	}
	if calls != 1 {
		t.Fatalf("idempotent renewal sent %d notifications", calls)
	}
}
