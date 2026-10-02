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
	return hub.Peer{PeerID: "peer", TenantID: id.TenantID, RouterID: id.RouterID, RouterPublicKey: key, EnrollmentVersion: 1, State: "active", TunnelAddress: "10.77.0.2/32"}, nil
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
		})
	}
}
