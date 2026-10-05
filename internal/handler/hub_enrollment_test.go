package handler

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

type enrollmentStub struct{ calls int }

func (s *enrollmentStub) EnrollPeer(_ context.Context, id hub.Identity, key string, port int) (hub.Peer, error) {
	s.calls++
	if id != (hub.Identity{TenantID: "tenant", RouterID: "gateway", RequestedBy: "owner"}) || port != 443 {
		panic("untrusted enrollment identity")
	}
	return hub.Peer{PeerID: "peer", TenantID: id.TenantID, RouterID: id.RouterID, RouterPublicKey: key, HubPublicKey: key, Endpoint: "hub.example.com:51820", AllowedIPs: []string{"10.77.0.1/32"}, EnrollmentVersion: 1, State: "active", TunnelAddress: "10.77.0.2/32"}, nil
}
func TestHubEnrollmentOwnerScope(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	for _, tc := range []struct {
		name, role, tenant, layer string
		confirm, enabled          bool
		want                      int
	}{
		{"valid", "OWNER", "tenant", "MAIN_GATEWAY", true, true, 200},
		{"viewer", "VIEWER", "tenant", "MAIN_GATEWAY", true, true, 403},
		{"foreign", "OWNER", "foreign", "MAIN_GATEWAY", true, true, 404},
		{"downstream", "OWNER", "tenant", "ACCESS_POINT", true, true, 404},
		{"unconfirmed", "OWNER", "tenant", "MAIN_GATEWAY", false, true, 400},
		{"disabled", "OWNER", "tenant", "MAIN_GATEWAY", true, false, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &enrollmentStub{}
			h := &VPSHandler{livePlan: func(string) bool { return true }, liveDevice: func(*gin.Context, string, string) (*model.Device, error) {
				return &model.Device{DeviceID: "gateway", BusinessID: tc.tenant, DeploymentLayer: tc.layer}, nil
			}}
			if tc.enabled {
				h.ConfigureHubEnrollment(stub)
			}
			confirm := "false"
			if tc.confirm {
				confirm = "true"
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"router_public_key":"`+key+`","management_rest_port":443,"confirm_enrollment":`+confirm+`,"tenantId":"attacker"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "deviceId", Value: "gateway"}}
			c.Set(auth.ContextKeyBusinessID, "tenant")
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, tc.role)
			h.EnrollGateway(c)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if (stub.calls == 1) != (tc.want == 200) {
				t.Fatal("unauthorized Hub mutation", stub.calls)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("enrollment response cacheable")
			}
			if tc.want == 200 && !strings.Contains(w.Body.String(), `"router_configuration_required":true`) {
				t.Fatal("claimed router configured")
			}
			if tc.want == 200 && (!strings.Contains(w.Body.String(), `"router_script":`) || !strings.Contains(w.Body.String(), `"disconnect_script":`)) {
				t.Fatal("missing onboarding artifacts")
			}
			// Bootstrap uses the same ownership/role/rollout boundaries, but never
			// calls the mutating Hub API and needs no enrollment consent yet.
			before := stub.calls
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{{Key: "deviceId", Value: "gateway"}}
			c.Set(auth.ContextKeyBusinessID, "tenant")
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, tc.role)
			h.GatewayBootstrap(c)
			want := tc.want
			if tc.name == "unconfirmed" {
				want = 200
			}
			if w.Code != want || stub.calls != before {
				t.Fatal("bootstrap escaped authorization or mutated Hub", w.Code, w.Body.String())
			}
			if want == 200 && (!strings.Contains(w.Body.String(), `"bootstrap_script":`) || strings.Contains(w.Body.String(), "private-key=")) {
				t.Fatal("invalid bootstrap artifact")
			}
		})
	}
}

func TestQuickEnrollmentPreservesExistingTunnel(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	stub := &enrollmentStub{}
	peer := hub.Peer{PeerID: "existing", TenantID: "tenant", RouterID: "gateway", RouterPublicKey: key, HubPublicKey: key, Endpoint: "hub.example.com:51820", AllowedIPs: []string{"10.77.0.9/32"}, EnrollmentVersion: 1, State: "active", TunnelAddress: "10.77.0.2/32"}
	h := &VPSHandler{enrollmentHub: stub, liveHub: managementInspector{peer}, livePlan: func(string) bool { return true }, liveDevice: func(*gin.Context, string, string) (*model.Device, error) {
		return &model.Device{DeviceID: "gateway", BusinessID: "tenant", DeploymentLayer: "MAIN_GATEWAY"}, nil
	}}
	calls := 0
	h.ConfigureAutomaticManagement(func(_ context.Context, biz, id, pub, script string) (string, error) {
		calls++
		if biz != "tenant" || id != "gateway" || pub != key || strings.Contains(script, "private-key=") {
			t.Fatal("replaced existing tunnel identity")
		}
		return script + "\nmanagement", nil
	}, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "deviceId", Value: "gateway"}}
	c.Set(auth.ContextKeyBusinessID, "tenant")
	c.Set(auth.ContextKeyUserID, "owner")
	c.Set(auth.ContextKeyUserRole, "OWNER")
	h.QuickEnrollGateway(c)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "hub_router_key_unavailable") || stub.calls != 0 || calls != 1 {
		t.Fatal(w.Code, w.Body.String(), stub.calls, calls)
	}
}
