package server

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/hub"
	"mega-magic/internal/secretbox"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type restartHub struct {
	peer  hub.Peer
	calls int
}

func (s *restartHub) InspectRouter(context.Context, hub.Identity) (hub.Peer, error) {
	if s.peer.PeerID == "" {
		return hub.Peer{}, hub.ErrPeerNotFound
	}
	return s.peer, nil
}
func (s *restartHub) EnrollPeer(_ context.Context, id hub.Identity, key string, _ int) (hub.Peer, error) {
	s.calls++
	s.peer = hub.Peer{PeerID: "fixture-peer", TenantID: id.TenantID, RouterID: id.RouterID, RouterPublicKey: key, HubPublicKey: key, Endpoint: "hub.example.test:51820", AllowedIPs: []string{"10.77.0.1/32"}, TunnelAddress: "10.77.0.20/32", EnrollmentVersion: 1, State: "active"}
	return s.peer, nil
}
func TestAutomaticOnePasteSurvivesCoordinatorRestart(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	b, e := os.ReadFile("../../migrations/20261024160000_hub_pending_enrollments.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(b)); e != nil {
		t.Fatal(e)
	}
	var biz, id string
	if e = db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier)VALUES('Restart fixture','restart@fixture.test','SMALL_WISP')RETURNING business_id`).Scan(&biz); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(`INSERT INTO devices(business_id,hardware_name,deployment_layer)VALUES($1,'Restart fixture','MAIN_GATEWAY')RETURNING device_id`, biz).Scan(&id); e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{}
	cfg.Provisioner.Applier.CredentialKey = strings.Repeat("ab", 32)
	cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
	cfg.WireGuard.Interface = "missing-container-wg0"
	cfg.WireGuard.ManagementSourceCIDR = "10.77.0.1/32"
	stub := &restartHub{}
	makeHandler := func() *handler.VPSHandler {
		h := handler.NewVPSHandler(db, database.NewDeviceRepository(db), nil, nil, cfg.WireGuard, "")
		h.SubscriptionOnly = true
		h.ConfigureHubLookup(stub)
		h.ConfigureHubEnrollment(stub)
		if e := configureHubManagement(h, db, cfg); e != nil {
			t.Fatal(e)
		}
		return h
	}
	run := func(h *handler.VPSHandler, recovery ...bool) (int, string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true}`))
		if len(recovery) > 0 && recovery[0] {
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true,"resume_existing":true}`))
		}
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "deviceId", Value: id}}
		c.Set(auth.ContextKeyBusinessID, biz)
		c.Set(auth.ContextKeyUserID, "fixture-owner")
		c.Set(auth.ContextKeyUserRole, "OWNER")
		h.QuickEnrollGateway(c)
		var out struct {
			Script string `json:"router_script"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Script
	}
	first := makeHandler()
	code, script := run(first)
	if code != 200 || !strings.Contains(script, `src-address=10.77.0.1/32`) || strings.Contains(script, "src-address=10.77.0.9/32") {
		t.Fatalf("initial one-paste status %d or wrong source", code)
	}
	second := makeHandler()
	code, retry := run(second)
	if code != 200 || script != retry || stub.calls != 1 {
		t.Fatalf("restart changed installation or allocated peer: status=%d calls=%d", code, stub.calls)
	}
	if _, e = db.Exec(`UPDATE hub_pending_enrollments SET expires_at=now()-interval '1 second' WHERE device_id=$1 AND purpose='management'`, id); e != nil {
		t.Fatal(e)
	}
	third := makeHandler()
	code, renewed := run(third)
	if code != 200 || renewed == script || stub.calls != 1 {
		t.Fatal("management proof expiry changed existing router key or failed recovery", code)
	}
	// Expired router-key material cannot authorize silent peer replacement.
	if _, e = db.Exec(`UPDATE hub_pending_enrollments SET expires_at=now()-interval '1 second' WHERE device_id=$1`, id); e != nil {
		t.Fatal(e)
	}
	fourth := makeHandler()
	code, _ = run(fourth)
	if code != 409 || stub.calls != 1 {
		t.Fatal("expired key silently replaced peer", code)
	}
	fifth := makeHandler()
	code, repair := run(fifth, true)
	if code != 200 || stub.calls != 1 || strings.Contains(repair, "private-key=") || !strings.Contains(repair, "router key differs") || !strings.Contains(repair, "/user/ssh-keys import") {
		t.Fatal("explicit installed-tunnel repair failed or replaced the identity", code)
	}
	box, _ := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if e = (database.PendingEnrollmentStore{DB: db, Box: box}).Delete(context.Background(), biz, id); e != nil {
		t.Fatal(e)
	}
}
