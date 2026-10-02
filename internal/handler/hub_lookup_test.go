package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

type lookupStub struct {
	calls int
	peer  hub.Peer
}

func (s *lookupStub) InspectRouter(_ context.Context, id hub.Identity) (hub.Peer, error) {
	s.calls++
	if id != (hub.Identity{TenantID: "business-a", RouterID: "gateway-a", RequestedBy: "owner-a"}) {
		panic("untrusted identity")
	}
	return s.peer, nil
}

func TestLivePeerAuthorizationAndHonestStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	stale := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	for _, tc := range []struct {
		name, role, business, layer string
		enabled, plan               bool
		handshake                   *time.Time
		want                        int
		status                      string
	}{
		{"disabled", "OWNER", "business-a", "MAIN_GATEWAY", false, true, nil, 503, ""},
		{"viewer", "VIEWER", "business-a", "MAIN_GATEWAY", true, true, nil, 403, ""},
		{"no pass", "OWNER", "business-a", "MAIN_GATEWAY", true, false, nil, 403, ""},
		{"other tenant", "OWNER", "business-b", "MAIN_GATEWAY", true, true, nil, 404, ""},
		{"downstream", "OWNER", "business-a", "ACCESS_POINT", true, true, nil, 404, ""},
		{"never", "OWNER", "business-a", "MAIN_GATEWAY", true, true, nil, 200, "no_recent_handshake"},
		{"stale", "OWNER", "business-a", "MAIN_GATEWAY", true, true, &stale, 200, "no_recent_handshake"},
		{"future", "OWNER", "business-a", "MAIN_GATEWAY", true, true, &future, 200, "no_recent_handshake"},
		{"fresh", "OWNER", "business-a", "MAIN_GATEWAY", true, true, &now, 200, "recent_handshake"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &lookupStub{peer: hub.Peer{PeerID: "peer-a", TenantID: "business-a", RouterID: "gateway-a", State: "active", LastHandshakeAt: tc.handshake}}
			h := &VPSHandler{livePlan: func(string) bool { return tc.plan }, liveDevice: func(*gin.Context, string, string) (*model.Device, error) {
				return &model.Device{DeviceID: "gateway-a", BusinessID: tc.business, DeploymentLayer: tc.layer}, nil
			}}
			if tc.enabled {
				h.liveHub = s
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{{Key: "deviceId", Value: "gateway-a"}}
			c.Set(auth.ContextKeyBusinessID, "business-a")
			c.Set(auth.ContextKeyUserID, "owner-a")
			c.Set(auth.ContextKeyUserRole, tc.role)
			h.LivePeer(c)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if tc.want != 200 && s.calls != 0 {
				t.Fatal("unauthorized Hub call")
			}
			if tc.want == 200 && (s.calls != 1 || !strings.Contains(w.Body.String(), tc.status) || !strings.Contains(w.Body.String(), `"lan_reachability_verified":false`)) {
				t.Fatal(w.Body.String())
			}
		})
	}
}
