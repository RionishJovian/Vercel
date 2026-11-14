package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// A Platform Admin can permanently delete a router record together with the history that
// normally keeps it. Nobody else can, a wrong name changes nothing, and any failure leaves
// the record and every ledger exactly as they were.
func TestDBRouterForceDeleteByPlatformAdmin(t *testing.T) {
	db := harness_testdb(t)
	ensureBroadbandSchema(t, db)
	business, foreign := insertBusiness(db), insertBusiness(db)
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	gin.SetMode(gin.TestMode)

	var user string
	if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Platform','fixture','OWNER')RETURNING user_id`, business, randHex(6)+"@example.test").Scan(&user); err != nil {
		t.Fatal(err)
	}
	remove := func(device, tenant, role, query string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, tenant)
			c.Set(auth.ContextKeyUserID, user)
			c.Set(auth.ContextKeyUserRole, role)
		})
		r.DELETE("/routers/:id", auth.RequireMinRole(model.RoleAdmin), h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+device+query, nil))
		return w
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// Every kind of history that blocks a normal delete, on one router that already ended its
	// Hub connection (the Nish's ax3 case).
	withEveryHistory := func(t *testing.T) (device string, voucher string) {
		t.Helper()
		device = insertGatewayNoSite(db, business)
		if err := db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk) VALUES($1,$2,'guest-internet','Daily',$3,500) RETURNING id`, business, device, "FORCE-"+randHex(6)).Scan(&voucher); err != nil {
			t.Fatal(err)
		}
		// In this order: recovery points at the voucher's grant.
		for _, seed := range []struct{ name, q string }{
			{"grant", `INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb) VALUES('` + voucher + `',$1,$2,60,1024)`},
			{"recovery", `INSERT INTO guest_access_recovery(voucher_id,business_id,device_id,secret_hash,current_mac) VALUES('` + voucher + `',$1,$2,decode(repeat('ab',32),'hex'),'02:00:00:00:00:01')`},
			{"batch", `INSERT INTO guest_voucher_issue_batches(business_id,id,device_id,request_hash,plan_label,price_mmk,minutes,data_mb,total_count) VALUES($1,uuid_generate_v4(),$2,repeat('a',64),'Daily',500,60,1024,1)`},
			{"vip", `INSERT INTO vip_wifi_codes(business_id,request_id,device_id,code,max_devices) VALUES($1,uuid_generate_v4(),$2,'VIP-'||translate(upper(substr(md5(random()::text),1,8)),'0123456789ILOU','ABCDEFGHJKMN'),2)`},
			{"ap", `INSERT INTO ap_radio_proposals(business_id,device_id,proposal,proposal_hash,fingerprint,expires_at) VALUES($1,$2,'{}',repeat('a',64),repeat('b',64),now()+interval '4 minutes')`},
		} {
			if _, err := db.Exec(seed.q, business, device); err != nil {
				t.Fatalf("seed %s: %v", seed.name, err)
			}
		}
		var account string
		if err := db.QueryRow(`INSERT INTO broadband_accounts(id,business_id,device_id,username,profile) VALUES(uuid_generate_v4(),$1,$2,'paid-customer','10M') RETURNING id`, business, device).Scan(&account); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO broadband_operations(id,business_id,device_id,account_id,kind,amount_mmk,period_days,request_hash,payload_ciphertext,state,applied_at)
VALUES(uuid_generate_v4(),$1,$2,$3,'create',15000,30,repeat('c',64),'sealed','applied',now())`, business, device, account); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO broadband_session_usage(business_id,device_id,account_id,session_id,download_bytes,upload_bytes,uptime_seconds,observed_at) VALUES($1,$2,$3,'s1',1,1,1,now())`, business, device, account); err != nil {
			t.Fatal(err)
		}
		return device, voucher
	}
	ledgers := func(device string) int {
		t.Helper()
		total := 0
		for _, table := range forceDeleteHistory {
			total += count(`SELECT count(*) FROM `+table+` WHERE device_id=$1`, device)
		}
		return total
	}
	untouched := func(t *testing.T, device string) {
		t.Helper()
		if count(`SELECT count(*) FROM devices WHERE device_id=$1`, device) != 1 || ledgers(device) != 8 {
			t.Fatalf("a refused or failed force delete changed data: %d ledger rows left", ledgers(device))
		}
	}

	t.Run("a normal delete still keeps the record", func(t *testing.T) {
		device, _ := withEveryHistory(t)
		if w := remove(device, business, "DEVELOPER", ""); w.Code != http.StatusConflict {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		untouched(t, device)
	})
	t.Run("only the Platform Admin may force", func(t *testing.T) {
		device, _ := withEveryHistory(t)
		q := "?force=true&confirm=" + url.QueryEscape("gateway-no-site")
		for _, role := range []string{"OWNER", "ADMIN", "VIEWER"} {
			if w := remove(device, business, role, q); w.Code != http.StatusForbidden {
				t.Fatalf("%s got %d, want 403", role, w.Code)
			}
		}
		untouched(t, device)
	})
	t.Run("a missing or wrong name changes nothing", func(t *testing.T) {
		device, _ := withEveryHistory(t)
		for _, q := range []string{"?force=true", "?force=true&confirm=wrong", "?force=true&confirm=GATEWAY-NO-SITE"} {
			if w := remove(device, business, "DEVELOPER", q); w.Code != http.StatusBadRequest {
				t.Fatalf("%q got %d body %s", q, w.Code, w.Body.String())
			}
		}
		untouched(t, device)
	})
	t.Run("another business's router cannot be force deleted", func(t *testing.T) {
		device, _ := withEveryHistory(t)
		if w := remove(device, foreign, "DEVELOPER", "?force=true&confirm=gateway-no-site"); w.Code != http.StatusForbidden {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		untouched(t, device)
	})
	t.Run("Hub unavailable changes nothing", func(t *testing.T) {
		device, _ := withEveryHistory(t)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", errors.New("hub down") })
		defer h.ConfigureHubRemoval(nil)
		if w := remove(device, business, "DEVELOPER", "?force=true&confirm=gateway-no-site"); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		untouched(t, device)
	})
	t.Run("a pending management migration still blocks and rolls the history back", func(t *testing.T) {
		device, _ := withEveryHistory(t)
		if _, err := db.Exec(`INSERT INTO management_address_migrations(id,business_id,device_id,requested_by,old_address,new_address,interface_name,old_record,fingerprint,router_key,phase,expires_at)
VALUES(uuid_generate_v4(),$1,$2,$3,'10.77.0.50','10.77.0.51','mm-hub','old','SHA256:'||repeat('A',43),'key','planned',now()+interval '1 hour')`, business, device, user); err != nil {
			t.Fatal(err)
		}
		w := remove(device, business, "DEVELOPER", "?force=true&confirm=gateway-no-site")
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "management migration") {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		untouched(t, device)
	})
	t.Run("the Platform Admin deletes the record and its history, and the sale stays", func(t *testing.T) {
		device, voucher := withEveryHistory(t)
		var ended string
		h.ConfigureHubRemoval(func(_ context.Context, _, _, id string) (string, error) { ended = id; return "cleanup-script", nil })
		defer h.ConfigureHubRemoval(nil)
		w := remove(device, business, "DEVELOPER", "?force=true&confirm=gateway-no-site")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"forced":true`) || !strings.Contains(w.Body.String(), `"router_cleanup_script":"cleanup-script"`) {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if ended != device {
			t.Fatalf("Hub connection not ended for %q", device)
		}
		if count(`SELECT count(*) FROM devices WHERE device_id=$1`, device) != 0 || ledgers(device) != 0 {
			t.Fatal("the record or its history is still there")
		}
		// The sale itself is a business record: it loses only its device link.
		if count(`SELECT count(*) FROM vouchers WHERE id=$1 AND device_id IS NULL`, voucher) != 1 {
			t.Fatal("the voucher sale must stay, unlinked from the deleted router")
		}
		if count(`SELECT count(*) FROM audit_events WHERE entity_id=$1 AND action='router.force_delete' AND user_id=$2 AND before_state->'history_removed' ? 'broadband_accounts'`, device, user) != 1 {
			t.Fatal("the force delete was not audited with what it removed")
		}
	})
}
