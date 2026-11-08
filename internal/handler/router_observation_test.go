package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type observationFixture struct {
	calls            int
	business, device string
	snapshot routeros.NetworkInventory
}

func (f *observationFixture) ReadObservations(_ context.Context, b, d string) (routeros.NetworkInventory, error) {
	f.calls++
	f.business = b
	f.device = d
	if f.snapshot.Available != nil {return f.snapshot, nil}
	return routeros.NetworkInventory{WriteEnabled: true, Available: map[string]bool{"arp": true}, Objects: map[string][]routeros.NetworkObject{"arp": {}}}, nil
}

func TestRouterObservationAuthenticatedContract(t *testing.T) {
	db := harness_testdb(t)
	for _, file := range []string{"20261023120000_device_discovery_labels.sql", "20261024010000_discovery_label_isolation.sql"} {
		raw, e := os.ReadFile("../../migrations/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	biz, foreign := insertBusiness(db), insertBusiness(db)
	var device, other string
	for b, target := range map[string]*string{biz: &device, foreign: &other} {
		if e := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,deployment_layer) VALUES($1,'fixture','MAIN_GATEWAY') RETURNING device_id`, b).Scan(target); e != nil {
			t.Fatal(e)
		}
	}
	svc, _ := auth.NewService("test-secret-key-for-testing-only-16chars", time.Hour, 24*time.Hour, 4)
	svc.UseSessions(db)
	tokens := map[string]string{}
	for _, role := range []model.UserRole{model.RoleViewer, model.RoleAdmin, model.RoleOwner, model.RoleDeveloper} {
		u := &model.User{BusinessID: biz, UserRole: role, Email: randHex(8) + "@example.test"}
		u.PasswordHash, _ = svc.HashPassword("disposable-password")
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role) VALUES($1,$2,'Fixture',$3,$4) RETURNING user_id`, biz, u.Email, u.PasswordHash, role).Scan(&u.UserID); e != nil {
			t.Fatal(e)
		}
		a, _, e := svc.StartSession(u)
		if e != nil {
			t.Fatal(e)
		}
		tokens[string(role)] = a
	}
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	reader := &observationFixture{}
	h.EnableObservations(reader)
	r := gin.New()
	g := r.Group("/api/v1/routers", auth.CookieOrigin(), auth.Middleware(svc), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleViewer))
	RegisterRouterObservationRoutes(g, h)
	request := func(method, id, action, role, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/routers/"+id+"/"+action, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if tokens[role] != "" {
			req.Header.Set("Authorization", "Bearer "+tokens[role])
		}
		r.ServeHTTP(w, req)
		return w
	}
	body := `{"mac":"02:00:00:00:00:01","name":"Reception","kind":"ap"}`
	t.Run("denied writes do not mutate", func(t *testing.T) {
		for _, role := range []string{"", "VIEWER"} {
			w := request("PUT", device, "discovery-labels", role, body)
			want := 403
			if role == "" {
				want = 401
			}
			if w.Code != want {
				t.Fatalf("%s: %d %s", role, w.Code, w.Body)
			}
		}
		var n int
		db.QueryRow(`SELECT count(*) FROM device_discovery_labels`).Scan(&n)
		if n != 0 {
			t.Fatal("denied request mutated labels")
		}
	})
	t.Run("MAC contract and tenant readback", func(t *testing.T) {
		for _, role := range []string{"ADMIN", "OWNER", "DEVELOPER"} {
			w := request("PUT", device, "discovery-labels", role, body)
			if w.Code != 200 {
				t.Fatalf("%s %d %s", role, w.Code, w.Body)
			}
			var result struct{ Labels []struct{ MAC, Name string } }
			if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || len(result.Labels) != 1 || result.Labels[0].MAC != "02:00:00:00:00:01" || result.Labels[0].Name != "Reception" {
				t.Fatalf("frontend contract: %s", w.Body)
			}
		}
		w := request("GET", device, "discovery-labels", "VIEWER", "")
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})
	t.Run("foreign targets refused before connector", func(t *testing.T) {
		before := reader.calls
		for _, action := range []string{"observations", "discovery", "discovery-labels"} {
			if w := request("GET", other, action, "ADMIN", ""); w.Code != 404 {
				t.Fatalf("foreign %s %d", action, w.Code)
			}
		}
		if w := request("PUT", other, "discovery-labels", "ADMIN", body); w.Code != 404 {
			t.Fatal(w.Code)
		}
		if reader.calls != before {
			t.Fatal("foreign connector invoked")
		}
	})
	t.Run("forced RLS and composite device ownership", func(t *testing.T) {
		e := runInTenantForTest(db, foreign, func(tx *sql.Tx) error {
			var n int
			if e := tx.QueryRow(`SELECT count(*) FROM device_discovery_labels`).Scan(&n); e != nil {
				return e
			}
			if n != 0 {
				return fmt.Errorf("foreign tenant saw labels")
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		e = runInTenantForTest(db, foreign, func(tx *sql.Tx) error {
			_, e := tx.Exec(`INSERT INTO device_discovery_labels(business_id,device_id,mac_address,name,kind) VALUES($1,$2,'02:00:00:00:00:02','foreign','other')`, foreign, device)
			return e
		})
		if e == nil {
			t.Fatal("foreign device accepted")
		}
	})
	t.Run("invalid MAC and length rejected", func(t *testing.T) {
		for _, b := range []string{`{"mac":"invalid","name":"X","kind":"ap"}`, `{"mac":"01:00:00:00:00:01","name":"X","kind":"ap"}`, `{"mac":"02:00:00:00:00:01","name":" ","kind":"ap"}`} {
			if w := request("PUT", device, "discovery-labels", "ADMIN", b); w.Code != 400 {
				t.Fatal(w.Code)
			}
		}
	})
	t.Run("read-only discovery is tenant bound and does not grant device writes", func(t *testing.T) {
		before := reader.calls
		w := request("GET", device, "discovery", "VIEWER", "")
		if w.Code != 200 || reader.calls != before+1 || reader.business != biz || reader.device != device {
			t.Fatalf("expected authorized gateway reader, got %d %s", w.Code, w.Body)
		}
		var result struct {Discovery struct {
			ReadsOnly bool `json:"reads_only"`
			PhysicalLinksVerified bool `json:"physical_links_verified"`
			Partial bool `json:"partial"`
			Devices []struct{MAC string `json:"mac"`} `json:"devices"`
		} `json:"discovery"`}
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		if !result.Discovery.ReadsOnly || result.Discovery.PhysicalLinksVerified || !result.Discovery.Partial || len(result.Discovery.Devices) != 0 {
			t.Fatalf("discovery returned guessed devices or links: %s", w.Body)
		}
	})

	t.Run("mixed-brand discovery returns hints but no device management", func(t *testing.T) {
		reader.snapshot = routeros.NetworkInventory{
			ObservedAt:"2026-11-08T08:00:00Z",
			Available:map[string]bool{"neighbors":true,"arp":true},
			Objects:map[string][]routeros.NetworkObject{
				"neighbors":{{Values:map[string]string{
					"mac-address":"A0:10:20:30:40:50","address":"192.168.88.10",
					"identity":"shop-ap","platform":"iKuai","board":"IK-X",
					"interface":"ether2",
				}}},
				"arp":{{Values:map[string]string{"mac-address":"A0:10:20:30:40:50","address":"192.168.88.10"}}},
			},
		}
		defer func(){reader.snapshot = routeros.NetworkInventory{}}()
		w:=request("GET",device,"discovery","VIEWER","")
		if w.Code!=200{t.Fatalf("discovery %d %s",w.Code,w.Body)}
		var response struct{Discovery struct{
			Devices []struct{
				MAC string `json:"mac"`
				ReportedVendor string `json:"reported_vendor"`
				ReportedModel string `json:"reported_model"`
				OnlineVerified bool `json:"online_verified"`
				Capabilities []struct{Name,Status string} `json:"capabilities"`
				Attachments []struct{DirectPhysicalLinkVerified bool `json:"direct_physical_link_verified"`} `json:"attachments"`
			} `json:"devices"`
		} `json:"discovery"`}
		if err:=json.Unmarshal(w.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
		if len(response.Discovery.Devices)!=1{t.Fatalf("expected one merged MAC: %s",w.Body)}
		found:=response.Discovery.Devices[0]
		if found.MAC!="A0:10:20:30:40:50"||found.ReportedVendor!="iKuai"||found.ReportedModel!="IK-X"||found.OnlineVerified {
			t.Fatalf("discovery incorrectly claimed capabilities: %+v",found)
		}
		for _,cap:=range found.Capabilities {
			if cap.Name!="gateway_observation"&&cap.Status!="unknown"{t.Fatalf("unexpected write capability: %+v",cap)}
		}
		for _,link:=range found.Attachments{if link.DirectPhysicalLinkVerified{t.Fatal("invented physical hop")}}
	})

	t.Run("read connector scope and no write capability", func(t *testing.T) {
		w := request("GET", device, "observations", "VIEWER", "")
		if w.Code != 200 || reader.business != biz || reader.device != device {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var result struct{ Inventory routeros.NetworkInventory }
		json.Unmarshal(w.Body.Bytes(), &result)
		if result.Inventory.WriteEnabled {
			t.Fatal("reader advertised router writes")
		}
		h.observations = nil
		if w = request("GET", device, "observations", "VIEWER", ""); w.Code != 503 {
			t.Fatal(w.Code)
		}
	})
	t.Run("disconnect explicitly unavailable", func(t *testing.T) {
		b := `{"id":"*1","mac":"02:00:00:00:00:01","address":"10.0.0.2","user":"guest","confirm":true}`
		if w := request("POST", device, "guests/disconnect", "VIEWER", b); w.Code != 403 {
			t.Fatal(w.Code)
		}
		if w := request("POST", device, "guests/disconnect", "ADMIN", b); w.Code != 503 {
			t.Fatal(w.Code)
		}
	})
}

func runInTenantForTest(db *database.DB, biz string, fn func(*sql.Tx) error) error {
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	return database.WithTenant(tx, biz, func() error { return fn(tx) })
}
