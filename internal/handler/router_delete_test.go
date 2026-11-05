package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

func TestDBRouterDeletionPreservesLinkedHistory(t *testing.T) {
	db := harness_testdb(t)
	business, foreign := insertBusiness(db), insertBusiness(db)
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	gin.SetMode(gin.TestMode)
	remove := func(id, tenant, role string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, tenant)
			c.Set(auth.ContextKeyUserRole, role)
		})
		r.DELETE("/routers/:id", auth.RequireMinRole(model.RoleAdmin), h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+id, nil))
		return w
	}
	for _, history := range []string{"none", "grant", "batch", "recovery"} {
		t.Run(history, func(t *testing.T) {
			device := insertGatewayNoSite(db, business)
			var voucher string
			if history == "grant" || history == "recovery" {
				if err := db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk)
VALUES($1,$2,'guest-internet','Daily',$3,500) RETURNING id`, business, device, "DELETE-"+randHex(6)).Scan(&voucher); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb) VALUES($1,$2,$3,60,1024)`, voucher, business, device); err != nil {
					t.Fatal(err)
				}
			}
			if history == "recovery" {
				if _, err := db.Exec(`INSERT INTO guest_access_recovery(voucher_id,business_id,device_id,secret_hash,current_mac) VALUES($1,$2,$3,decode(repeat('ab',32),'hex'),'02:00:00:00:00:01')`, voucher, business, device); err != nil {
					t.Fatal(err)
				}
			}
			if history == "batch" {
				if _, err := db.Exec(`INSERT INTO guest_voucher_issue_batches(business_id,id,device_id,request_hash,plan_label,price_mmk,minutes,data_mb,total_count) VALUES($1,uuid_generate_v4(),$2,repeat('a',64),'Daily',500,60,1024,1)`, business, device); err != nil {
					t.Fatal(err)
				}
			}
			for _, denied := range []struct {
				tenant, role string
				status       int
			}{{foreign, "OWNER", 404}, {business, "VIEWER", 403}} {
				w := remove(device, denied.tenant, denied.role)
				if w.Code != denied.status || strings.Contains(w.Body.String(), "history") {
					t.Fatalf("unauthorized deletion disclosed history or escaped boundary: %d", w.Code)
				}
			}
			w := remove(device, business, "OWNER")
			want, count := 409, 1
			if history == "none" {
				want, count = 200, 0
			}
			if w.Code != want {
				t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
			}
			if history != "none" && (!strings.Contains(w.Body.String(), "No records were deleted") || strings.Contains(w.Body.String(), "_fkey")) {
				t.Fatal("missing actionable conflict or leaked database constraint")
			}
			var actual int
			if err := db.QueryRow(`SELECT count(*) FROM devices WHERE device_id=$1 AND business_id=$2`, device, business).Scan(&actual); err != nil || actual != count {
				t.Fatal("device preservation/deletion mismatch", err)
			}
			if voucher != "" {
				var unchanged bool
				if err := db.QueryRow(`SELECT v.device_id=$2 AND g.device_id=$2 AND g.minutes=60 AND g.data_mb=1024 FROM vouchers v JOIN guest_voucher_grants g ON g.voucher_id=v.id WHERE v.id=$1`, voucher, device).Scan(&unchanged); err != nil || !unchanged {
					t.Fatal("failed deletion changed voucher ownership or allowance", err)
				}
			}
			if history == "batch" || history == "recovery" {
				table := "guest_voucher_issue_batches"
				if history == "recovery" {
					table = "guest_access_recovery"
				}
				if err := db.QueryRow(`SELECT count(*) FROM `+table+` WHERE device_id=$1 AND business_id=$2`, device, business).Scan(&actual); err != nil || actual != 1 {
					t.Fatal("failed deletion removed linked history", err)
				}
			}
		})
	}
}

// Delete ends the Hub connection and hands back the cleanup script. If the Hub cannot
// end it, the record stays, so a connection can never outlive its record again.
func TestDBRouterDeletionEndsHubConnection(t *testing.T) {
	db := harness_testdb(t)
	business := insertBusiness(db)
	gin.SetMode(gin.TestMode)
	remove := func(h *RouterHandler, id string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, business)
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, "OWNER")
		})
		r.DELETE("/routers/:id", h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+id, nil))
		return w
	}
	exists := func(id string) bool {
		var found bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE device_id=$1)`, id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)

	t.Run("Hub unavailable keeps the record", func(t *testing.T) {
		device := insertGatewayNoSite(db, business)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", errors.New("hub down") })
		if w := remove(h, device); w.Code != http.StatusServiceUnavailable || !exists(device) {
			t.Fatalf("status %d, record exists %v", w.Code, exists(device))
		}
	})
	t.Run("success returns the cleanup script", func(t *testing.T) {
		device := insertGatewayNoSite(db, business)
		var called string
		h.ConfigureHubRemoval(func(_ context.Context, biz, _, id string) (string, error) {
			called = biz + "/" + id
			return "cleanup-script", nil
		})
		w := remove(h, device)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"router_cleanup_script":"cleanup-script"`) || exists(device) {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if called != business+"/"+device {
			t.Fatalf("removed the wrong peer: %s", called)
		}
	})
	// A router with sales history: the record and history stay, the login and connection go.
	withHistory := func(t *testing.T) string {
		t.Helper()
		device := insertGatewayNoSite(db, business)
		var voucher string
		if err := db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk) VALUES($1,$2,'guest-internet','Daily',$3,500) RETURNING id`, business, device, "HUBDEL-"+randHex(6)).Scan(&voucher); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb) VALUES($1,$2,$3,60,1024)`, voucher, business, device); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE devices SET management_ip='10.77.0.50', ssh_port=2222, ssh_username='mm-test', ssh_secret_ciphertext='sealed', ssh_secret_kind='key', ssh_host_key_fingerprint='SHA256:'||repeat('A',43), is_online=true WHERE device_id=$1`, device); err != nil {
			t.Fatal(err)
		}
		return device
	}
	login := func(t *testing.T, device string) (saved bool, port int, online bool) {
		t.Helper()
		if err := db.QueryRow(`SELECT management_ip IS NOT NULL OR ssh_username IS NOT NULL OR ssh_secret_ciphertext IS NOT NULL OR ssh_secret_kind IS NOT NULL OR ssh_host_key_fingerprint IS NOT NULL, ssh_port, is_online FROM devices WHERE device_id=$1`, device).Scan(&saved, &port, &online); err != nil {
			t.Fatal(err)
		}
		return saved, port, online
	}
	history := func(t *testing.T, device string) int {
		t.Helper()
		var rows int
		if err := db.QueryRow(`SELECT count(*) FROM vouchers v JOIN guest_voucher_grants g ON g.voucher_id=v.id WHERE v.device_id=$1 AND g.device_id=$1 AND g.minutes=60`, device).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	t.Run("a record with history ends the connection and keeps the record", func(t *testing.T) {
		device := withHistory(t)
		var ended, forgotten string
		h.ConfigureHubRemoval(func(_ context.Context, _, _, id string) (string, error) { ended = id; return "cleanup-script", nil })
		h.ConfigureHubForget(func(_ context.Context, _, id string) error { forgotten = id; return nil })
		defer h.ConfigureHubForget(nil)
		w := remove(h, device)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"record_kept":true`) || !strings.Contains(w.Body.String(), `"router_cleanup_script":"cleanup-script"`) {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if ended != device || forgotten != device {
			t.Fatalf("ended %q, forgotten %q, want %q", ended, forgotten, device)
		}
		if !exists(device) || history(t, device) != 1 {
			t.Fatal("the record and its history must stay")
		}
		if saved, port, online := login(t, device); saved || port != 22 || online {
			t.Fatalf("login still saved: %v, port %d, online %v", saved, port, online)
		}
		// Deleting again finds nothing left to end and changes nothing else.
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", nil })
		if w := remove(h, device); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"record_kept":true`) || strings.Contains(w.Body.String(), "router_cleanup_script") || !exists(device) || history(t, device) != 1 {
			t.Fatalf("second delete: status %d body %s", w.Code, w.Body.String())
		}
	})
	t.Run("Hub unavailable changes nothing on a record with history", func(t *testing.T) {
		device := withHistory(t)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", errors.New("hub down") })
		if w := remove(h, device); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %d", w.Code)
		}
		if saved, port, online := login(t, device); !saved || port != 2222 || !online || history(t, device) != 1 {
			t.Fatalf("a failed delete changed the login: %v, port %d, online %v", saved, port, online)
		}
	})
	t.Run("a failed setup clear does not undo the result", func(t *testing.T) {
		device := withHistory(t)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", nil })
		h.ConfigureHubForget(func(context.Context, string, string) error { return errors.New("store down") })
		defer h.ConfigureHubForget(nil)
		if w := remove(h, device); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"record_kept":true`) {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
	})
}

// A commit that fails after the Hub has ended the connection must say so, keep the only
// copy of the cleanup script, and leave a state that pressing Delete again can finish.
func TestDBRouterDeleteCommitFailsAfterHubEnded(t *testing.T) {
	db := harness_testdb(t)
	business := insertBusiness(db)
	device := insertGatewayNoSite(db, business)
	gin.SetMode(gin.TestMode)
	// A deferred trigger runs at COMMIT, so this is a genuine commit failure.
	for _, statement := range []string{
		`CREATE OR REPLACE FUNCTION test_fail_router_delete_commit() RETURNS trigger LANGUAGE plpgsql AS $f$ BEGIN RAISE EXCEPTION 'commit refused for test'; END $f$`,
		`DROP TRIGGER IF EXISTS test_fail_router_delete ON devices`,
		`CREATE CONSTRAINT TRIGGER test_fail_router_delete AFTER DELETE ON devices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (OLD.device_id = '` + device + `') EXECUTE FUNCTION test_fail_router_delete_commit()`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	dropTrigger := func() {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS test_fail_router_delete ON devices`); err != nil {
			t.Fatal(err)
		}
	}
	defer dropTrigger()

	peerExists := true
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) {
		if !peerExists {
			return "", nil
		}
		peerExists = false
		return "cleanup-script", nil
	})
	remove := func() *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, business)
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, "OWNER")
		})
		r.DELETE("/routers/:id", h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+device, nil))
		return w
	}
	exists := func() bool {
		var found bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE device_id=$1)`, device).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}

	w := remove()
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "hub_ended_record_unchanged") || !strings.Contains(w.Body.String(), `"router_cleanup_script":"cleanup-script"`) {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "commit refused") {
		t.Fatalf("leaked a database error: %s", w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("the cleanup script must not be cached")
	}
	if peerExists || !exists() {
		t.Fatalf("peer exists %v, record exists %v", peerExists, exists())
	}

	dropTrigger()
	w = remove()
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "router_cleanup_script") || exists() {
		t.Fatalf("second delete: status %d body %s, record exists %v", w.Code, w.Body, exists())
	}
}

func TestInventoryAnswered(t *testing.T) {
	for _, tc := range []struct {
		name string
		inv  routeros.NetworkInventory
		want bool
	}{
		{"nothing read", routeros.NetworkInventory{}, false},
		{"time but no answer", routeros.NetworkInventory{ObservedAt: "now", Available: map[string]bool{"arp": false}}, false},
		{"answer without a time", routeros.NetworkInventory{Health: map[string]string{"version": "7.25"}}, false},
		{"version read", routeros.NetworkInventory{ObservedAt: "now", Health: map[string]string{"version": "7.25"}}, true},
		{"a source read", routeros.NetworkInventory{ObservedAt: "now", Available: map[string]bool{"arp": true}}, true},
	} {
		if got := inventoryAnswered(tc.inv); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestDBConnectionHistoryStore(t *testing.T) {
	db := harness_testdb(t)
	repo := database.NewDeviceRepository(db)
	business, other := insertBusiness(db), insertBusiness(db)
	device, foreign := insertGatewayNoSite(db, business), insertGatewayNoSite(db, other)

	gateways, err := repo.ListGatewaysAcrossTenants()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, g := range gateways {
		seen[g.DeviceID] = g.BusinessID
	}
	if seen[device] != business || seen[foreign] != other {
		t.Fatalf("gateways %v", seen)
	}

	for _, action := range []string{database.ActionConnectionUp, database.ActionConnectionOffline} {
		if err := repo.RecordConnectionChange(business, device, action, "test"); err != nil {
			t.Fatal(err)
		}
		// created_at is the transaction time, so space the rows apart.
		if _, err := db.Exec(`UPDATE device_change_events SET created_at = created_at - interval '1 minute' WHERE device_id=$1`, device); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.RecordConnectionChange(other, foreign, database.ActionConnectionUp, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO device_change_events(business_id,device_id,user_role,action,reason) VALUES($1,$2,'OWNER','router.reboot.requested','unrelated')`, business, device); err != nil {
		t.Fatal(err)
	}
	latest, err := repo.LatestConnectionChanges(business)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || latest[device].Action != database.ActionConnectionOffline {
		t.Fatalf("latest %+v", latest)
	}
}

func TestDBRouterAlerts(t *testing.T) {
	db := harness_testdb(t)
	repo := database.NewDeviceRepository(db)
	// The handler test database doesn't run db/migrations, so apply the real file.
	migration, err := os.ReadFile("../../db/migrations/20261106130000_platform_router_alerts.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS platform_router_alerts`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	business := insertBusiness(db)
	add := func(device, action, age string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO device_change_events(business_id,device_id,user_role,action,reason,created_at) VALUES($1,$2,'SYSTEM',$3,'Last contact earlier.',now() - $4::interval)`, business, device, action, age); err != nil {
			t.Fatal(err)
		}
	}
	due := func() map[string]*database.RouterAlert {
		t.Helper()
		alerts, err := repo.DueRouterAlerts(15 * time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]*database.RouterAlert{}
		for _, a := range alerts {
			if a.BusinessID == business {
				out[a.DeviceID+"/"+a.Kind] = a
			}
		}
		return out
	}
	up, offline := database.ActionConnectionUp, database.ActionConnectionOffline

	dropped := insertGatewayNoSite(db, business)
	add(dropped, up, "2 hours")
	add(dropped, offline, "20 minutes")
	brief := insertGatewayNoSite(db, business)
	add(brief, up, "2 hours")
	add(brief, offline, "5 minutes")
	firstSeen := insertGatewayNoSite(db, business)
	add(firstSeen, offline, "2 hours")
	blip := insertGatewayNoSite(db, business)
	add(blip, up, "3 hours")
	add(blip, offline, "2 hours")
	add(blip, up, "1 hour 55 minutes")

	got := due()
	alert := got[dropped+"/offline"]
	if len(got) != 1 || alert == nil || alert.Detail != "Last contact earlier." {
		t.Fatalf("due %v", got)
	}

	created, err := repo.CreateRouterAlert(alert)
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	if created, err := repo.CreateRouterAlert(alert); err != nil || created {
		t.Fatalf("a repeat must not be new: %v %v", created, err)
	}
	if got := due(); len(got) != 0 {
		t.Fatalf("already alerted: %v", got)
	}

	add(dropped, up, "1 minute")
	got = due()
	back := got[dropped+"/recovered"]
	if len(got) != 1 || back == nil || !back.OfflineSince.Equal(alert.OfflineSince) {
		t.Fatalf("recovery %v", got)
	}
	if created, err := repo.CreateRouterAlert(back); err != nil || !created {
		t.Fatalf("create recovery: %v %v", created, err)
	}
	if got := due(); len(got) != 0 {
		t.Fatalf("recovery sent twice: %v", got)
	}

	list, unread, err := repo.ListRouterAlerts(100)
	if err != nil || unread < 2 || len(list) < 2 || list[0].HardwareName == "" || list[0].CompanyName == "" {
		t.Fatalf("list %d, unread %d, err %v", len(list), unread, err)
	}
	if err := repo.MarkRouterAlertsRead(); err != nil {
		t.Fatal(err)
	}
	if _, unread, err := repo.ListRouterAlerts(100); err != nil || unread != 0 {
		t.Fatalf("unread %d, err %v", unread, err)
	}
}
