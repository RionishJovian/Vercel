package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type managementInspector struct{ peer hub.Peer }

func (s managementInspector) InspectRouter(context.Context, hub.Identity) (hub.Peer, error) {
	return s.peer, nil
}
func TestManagementAuthenticationBoundaries(t *testing.T) {
	now := time.Now()
	old := now.Add(-6 * time.Minute)
	for _, tc := range []struct {
		name, role, tenant, address string
		stamp                       *time.Time
		fail                        bool
		want                        int
	}{
		{"valid", "OWNER", "tenant", "10.77.0.2/32", &now, false, 200},
		{"viewer", "VIEWER", "tenant", "10.77.0.2/32", &now, false, 403},
		{"foreign", "OWNER", "foreign", "10.77.0.2/32", &now, false, 409},
		{"stale", "OWNER", "tenant", "10.77.0.2/32", &old, false, 409},
		{"absent", "OWNER", "tenant", "10.77.0.2/32", nil, false, 409},
		{"public", "OWNER", "tenant", "8.8.8.8/32", &now, false, 502},
		{"broad", "OWNER", "tenant", "10.77.0.0/24", &now, false, 502},
		{"verification fails", "OWNER", "tenant", "10.77.0.2/32", &now, true, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := &VPSHandler{livePlan: func(string) bool { return true }, liveDevice: func(*gin.Context, string, string) (*model.Device, error) {
				return &model.Device{DeviceID: "router", BusinessID: "tenant", DeploymentLayer: "MAIN_GATEWAY"}, nil
			}, liveHub: managementInspector{hub.Peer{TenantID: tc.tenant, RouterID: "router", State: "active", TunnelAddress: tc.address, LastHandshakeAt: tc.stamp}}}
			h.ConfigureManagementAuthentication(func(_ context.Context, biz, id, address, user, password string, port int, fingerprint string) error {
				calls++
				if biz != "tenant" || id != "router" || address != "10.77.0.2" || password != "secret" {
					t.Fatal("untrusted input")
				}
				if tc.fail {
					return errors.New("secret diagnostic must not escape")
				}
				return nil
			})
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(auth.ContextKeyBusinessID, "tenant")
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, tc.role)
			c.Params = gin.Params{{Key: "deviceId", Value: "router"}}
			pin := "SHA256:" + base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"username":"operator","password":"secret","port":22,"fingerprint":"`+pin+`","confirm":true,"address":"8.8.8.8","tenantId":"attacker"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			h.AuthenticateGateway(c)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("credential leakage or caching")
			}
			if (calls == 1) != (tc.want == 200 || tc.fail) {
				t.Fatal("verification escaped gate", calls)
			}
		})
	}
}

func TestFinishConnectionBoundaries(t *testing.T) {
	now := time.Now()
	old := now.Add(-6 * time.Minute)
	for _, tc := range []struct {
		name, role, tenant, address string
		stamp                       *time.Time
		want                        int
	}{
		{"valid", "OWNER", "tenant", "10.77.0.2/32", &now, 200},
		{"viewer", "VIEWER", "tenant", "10.77.0.2/32", &now, 403},
		{"foreign", "OWNER", "foreign", "10.77.0.2/32", &now, 409},
		{"stale", "OWNER", "tenant", "10.77.0.2/32", &old, 409},
		{"missing", "OWNER", "tenant", "10.77.0.2/32", nil, 409},
		{"public", "OWNER", "tenant", "8.8.8.8/32", &now, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := &VPSHandler{livePlan: func(string) bool { return true }, liveDevice: func(*gin.Context, string, string) (*model.Device, error) {
				return &model.Device{DeviceID: "router", BusinessID: "tenant", DeploymentLayer: "MAIN_GATEWAY"}, nil
			}, liveHub: managementInspector{hub.Peer{TenantID: tc.tenant, RouterID: "router", RouterPublicKey: "trusted", State: "active", TunnelAddress: tc.address, LastHandshakeAt: tc.stamp}}}
			h.ConfigureAutomaticManagement(nil, func(_ context.Context, biz, id, address, key string) error {
				calls++
				if biz != "tenant" || id != "router" || address != "10.77.0.2" || key != "trusted" {
					t.Fatal("client-supplied identity")
				}
				return nil
			})
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"address":"8.8.8.8","router_key":"attacker"}`))
			c.Params = gin.Params{{Key: "deviceId", Value: "router"}}
			c.Set(auth.ContextKeyBusinessID, "tenant")
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, tc.role)
			h.FinishGatewayConnection(c)
			if w.Code != tc.want || (calls == 1) != (tc.want == 200) {
				t.Fatal(w.Code, w.Body.String(), calls)
			}
		})
	}
}
