package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// A device that has Magic Hub rows (pending enrollment, callback token, peer state) and no
// sales history must delete cleanly, and a device with any history table must keep its record.
func TestDBRouterDeletionWithHubRowsAndHistory(t *testing.T) {
	db := harness_testdb(t)
	business := insertBusiness(db)
	gin.SetMode(gin.TestMode)
	user := "11111111-1111-4111-8111-111111111111"
	remove := func(h *RouterHandler, role, id string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, business)
			c.Set(auth.ContextKeyUserID, user)
			c.Set(auth.ContextKeyUserRole, role)
		})
		r.DELETE("/routers/:id", h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+id, nil))
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
	hubRows := func(t *testing.T, device string) {
		t.Helper()
		for _, q := range []string{
			`INSERT INTO hub_pending_enrollments(business_id,device_id,purpose,ciphertext,expires_at) VALUES($1,$2,'router_key','sealed',now()+interval '1 hour')`,
			`INSERT INTO hub_callback_tokens(business_id,device_id,token_hash,requested_by,expires_at) VALUES($1,$2,decode(repeat('ab',32),'hex'),'` + user + `',now()+interval '1 hour')`,
			`INSERT INTO hub_peer_state(business_id,device_id,peer_id,router_public_key,tunnel_address,endpoint) VALUES($1,$2,'peer-1',repeat('A',43)||'=','10.77.0.9/32','hub.test:51820')`,
		} {
			if _, err := db.Exec(q, business, device); err != nil {
				t.Fatalf("seed hub rows: %v", err)
			}
		}
	}
	history := map[string]string{
		"vip wifi code": `INSERT INTO vip_wifi_codes(business_id,request_id,device_id,code,max_devices) VALUES($1,uuid_generate_v4(),$2,'VIP-'||translate(upper(substr(md5(random()::text),1,8)),'0123456789ILOU','ABCDEFGHJKMN'),2)`,
		"ap radio proposal": `INSERT INTO ap_radio_proposals(business_id,device_id,proposal,proposal_hash,fingerprint,expires_at)
			VALUES($1,$2,'{}',repeat('a',64),repeat('b',64),now()+interval '4 minutes')`,
	}

	for _, role := range []string{"OWNER", "ADMIN"} {
		t.Run(role+" deletes a clean router that has Hub rows", func(t *testing.T) {
			h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
			h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", nil })
			device := insertGatewayNoSite(db, business)
			hubRows(t, device)
			w := remove(h, role, device)
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			for _, table := range []string{"devices", "hub_pending_enrollments", "hub_callback_tokens", "hub_peer_state"} {
				if n := count(`SELECT count(*) FROM `+table+` WHERE device_id=$1`, device); n != 0 {
					t.Fatalf("%s still has %d row(s) for the deleted router", table, n)
				}
			}
		})
	}
	t.Run("a router without Hub wiring and with Hub rows is deleted", func(t *testing.T) {
		h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
		device := insertGatewayNoSite(db, business)
		hubRows(t, device)
		if w := remove(h, "OWNER", device); w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if n := count(`SELECT count(*) FROM devices WHERE device_id=$1`, device); n != 0 {
			t.Fatal("device still exists")
		}
	})
	for name, seed := range history {
		t.Run(name+" history keeps the record and ends only the connection", func(t *testing.T) {
			h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
			h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "cleanup", nil })
			h.ConfigureHubForget(func(context.Context, string, string) error { return nil })
			device := insertGatewayNoSite(db, business)
			if _, err := db.Exec(`UPDATE devices SET transport='tunnel', management_ip='10.77.0.9', ssh_username='mm', ssh_secret_ciphertext='sealed', ssh_secret_kind='key', ssh_host_key_fingerprint='SHA256:'||repeat('A',43), is_online=true WHERE device_id=$1`, device); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(seed, business, device); err != nil {
				t.Fatalf("seed history: %v", err)
			}
			w := remove(h, "OWNER", device)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"record_kept":true`) {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if n := count(`SELECT count(*) FROM devices WHERE device_id=$1 AND management_ip IS NULL AND ssh_secret_ciphertext IS NULL AND NOT is_online`, device); n != 1 {
				t.Fatal("record must stay with its connection ended")
			}
		})
		t.Run(name+" history on a non-gateway device returns a clear conflict", func(t *testing.T) {
			h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
			var device string
			if err := db.QueryRow(`INSERT INTO devices (business_id, hardware_name, deployment_layer) VALUES ($1,'ap','ACCESS_POINT') RETURNING device_id`, business).Scan(&device); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(seed, business, device); err != nil {
				t.Fatalf("seed history: %v", err)
			}
			w := remove(h, "OWNER", device)
			if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "_fkey") {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// With RLS enforcement on, the delete runs as the restricted tenant role. The Hub tables are closed
// to that role on purpose; their rows must still go with the device (the cascade runs as their owner).
func TestDBRouterDeletionAsTenantRoleRemovesHubRows(t *testing.T) {
	db := harness_testdb(t)
	business := insertBusiness(db)
	device := insertGatewayNoSite(db, business)
	for _, q := range []string{
		`INSERT INTO hub_pending_enrollments(business_id,device_id,purpose,ciphertext,expires_at) VALUES($1,$2,'router_key','sealed',now()+interval '1 hour')`,
		`INSERT INTO hub_peer_state(business_id,device_id,peer_id,router_public_key,tunnel_address,endpoint) VALUES($1,$2,'peer-1',repeat('A',43)||'=','10.77.0.9/32','hub.test:51820')`,
	} {
		if _, err := db.Exec(q, business, device); err != nil {
			t.Fatal(err)
		}
	}
	// Production grants the tenant role these on devices only (db/migrations/20261106120000_tenant_role_grants.sql).
	if _, err := db.Exec(`GRANT SELECT, DELETE ON devices TO authenticated`); err != nil {
		t.Skipf("authenticated role unavailable: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL ROLE authenticated`); err != nil {
		t.Skipf("authenticated role unavailable: %v", err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.current_business_id',$1,true)`, business); err != nil {
		t.Fatal(err)
	}
	if err := database.NewDeviceRepository(db).DeleteDeviceTXN(tx, device); err != nil {
		t.Fatalf("tenant role could not delete a router with Hub rows: %v", err)
	}
}
