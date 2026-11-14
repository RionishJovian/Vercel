package handler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
		if _, err := db.Exec(`UPDATE devices SET transport='tunnel', management_ip='10.77.0.50', ssh_port=2222, ssh_username='mm-test', ssh_secret_ciphertext='sealed', ssh_secret_kind='key', ssh_host_key_fingerprint='SHA256:'||repeat('A',43), is_online=true WHERE device_id=$1`, device); err != nil {
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
	// A connected router (verified tunnel address and saved login) that has no history
	// is still deleted outright; the disconnect exception is not a way to keep records.
	connected := func(t *testing.T) string {
		t.Helper()
		device := insertGatewayNoSite(db, business)
		if _, err := db.Exec(`UPDATE devices SET transport='tunnel', management_ip='10.77.0.60', ssh_port=2222, ssh_username='mm-test', ssh_secret_ciphertext='sealed', ssh_secret_kind='key', ssh_host_key_fingerprint='SHA256:'||repeat('A',43) WHERE device_id=$1`, device); err != nil {
			t.Fatal(err)
		}
		return device
	}
	t.Run("a connected router without history is deleted", func(t *testing.T) {
		device := connected(t)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "cleanup-script", nil })
		w := remove(h, device)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "record_kept") || exists(device) {
			t.Fatalf("status %d body %s, record exists %v", w.Code, w.Body.String(), exists(device))
		}
	})
	t.Run("a pending management migration still blocks the delete", func(t *testing.T) {
		device := withHistory(t)
		var user string
		if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture','OWNER')RETURNING user_id`, business, randHex(6)+"@example.test").Scan(&user); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO management_address_migrations(id,business_id,device_id,requested_by,old_address,new_address,interface_name,old_record,fingerprint,router_key,phase,expires_at)
VALUES(uuid_generate_v4(),$1,$2,$3,'10.77.0.50','10.77.0.51','mm-hub','old','SHA256:'||repeat('A',43),'key','planned',now()+interval '1 hour')`, business, device, user); err != nil {
			t.Fatal(err)
		}
		hubCalled := false
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { hubCalled = true; return "", nil })
		w := remove(h, device)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Recover or cancel this router's management migration") {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if hubCalled || !exists(device) || history(t, device) != 1 {
			t.Fatal("a blocked delete must not touch the Hub, the record or its history")
		}
		if saved, port, online := login(t, device); !saved || port != 2222 || !online {
			t.Fatalf("a blocked delete changed the login: %v, port %d, online %v", saved, port, online)
		}
	})
	t.Run("an unexpected failure is logged and does not leak", func(t *testing.T) {
		device := withHistory(t)
		for _, statement := range []string{
			`CREATE OR REPLACE FUNCTION test_fail_router_clear() RETURNS trigger LANGUAGE plpgsql AS $f$ BEGIN RAISE EXCEPTION 'clear refused for test'; END $f$`,
			`DROP TRIGGER IF EXISTS test_fail_router_clear ON devices`,
			`CREATE TRIGGER test_fail_router_clear AFTER UPDATE ON devices FOR EACH ROW WHEN (NEW.device_id = '` + device + `' AND NEW.management_ip IS NULL) EXECUTE FUNCTION test_fail_router_clear()`,
		} {
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
		defer db.Exec(`DROP TRIGGER IF EXISTS test_fail_router_clear ON devices`)
		var logged bytes.Buffer
		log.SetOutput(&logged)
		defer log.SetOutput(os.Stderr)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", nil })
		w := remove(h, device)
		if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "clear refused") {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if !strings.Contains(logged.String(), device) || !strings.Contains(logged.String(), "clear refused for test") {
			t.Fatalf("the underlying error was not logged: %q", logged.String())
		}
		if saved, _, _ := login(t, device); !saved {
			t.Fatal("a failed delete changed the login")
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

// ensureBroadbandSchema loads the broadband ledger once per shared test database, whichever
// test needs it first.
func ensureBroadbandSchema(t *testing.T, db *database.DB) {
	t.Helper()
	var loaded bool
	if err := db.QueryRow(`SELECT to_regclass('public.broadband_accounts') IS NOT NULL`).Scan(&loaded); err != nil {
		t.Fatal(err)
	}
	if loaded {
		return
	}
	source, err := os.ReadFile("../../db/migrations/20261104000000_broadband.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(source)); err != nil {
		t.Fatal(err)
	}
}

// Paid PPPoE accounts and their payment history keep the record, with the PPPoE message.
func TestDBRouterDeletionKeepsPaidPPPoEHistory(t *testing.T) {
	db := harness_testdb(t)
	ensureBroadbandSchema(t, db)
	business := insertBusiness(db)
	device := insertGatewayNoSite(db, business)
	if _, err := db.Exec(`UPDATE devices SET transport='tunnel', management_ip='10.77.0.70', ssh_port=2222, ssh_username='mm-test', ssh_secret_ciphertext='sealed', ssh_secret_kind='key', ssh_host_key_fingerprint='SHA256:'||repeat('A',43) WHERE device_id=$1`, device); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO broadband_accounts(id,business_id,device_id,username,profile) VALUES(uuid_generate_v4(),$1,$2,'paid-customer','10M')`, business, device); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyBusinessID, business)
		c.Set(auth.ContextKeyUserRole, "OWNER")
	})
	// No Hub removal is configured (as in the other history test), so the paid-account
	// rule decides, not the disconnect-and-keep path.
	r.DELETE("/routers/:id", NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil).DeleteRouter)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+device, nil))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "paid PPPoE accounts and payment history") {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var saved bool
	if err := db.QueryRow(`SELECT management_ip IS NOT NULL AND ssh_secret_ciphertext IS NOT NULL FROM devices WHERE device_id=$1`, device).Scan(&saved); err != nil || !saved {
		t.Fatal("a blocked delete changed the device", err)
	}
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

