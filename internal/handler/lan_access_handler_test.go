package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/lanaccess"
)

type lanHandlerTransport struct{}

func (lanHandlerTransport) Dial(context.Context, lanaccess.Grant) (net.Conn, error) {
	return nil, errors.New("fixture must never open a socket")
}

type lanScopeStub struct {
	calls   int
	foreign bool
}

func (s *lanScopeStub) ResolveLANScope(_ context.Context, business, gateway, user string) (lanaccess.Scope, error) {
	s.calls++
	if business != "tenant" || gateway != "gateway" || user != "owner" {
		panic("untrusted scope")
	}
	if s.foreign {
		business = "foreign"
	}
	return lanaccess.Scope{BusinessID: business, GatewayID: gateway, UserID: user, Role: "OWNER", Verified: true, LANs: []netip.Prefix{netip.MustParsePrefix("192.168.88.0/24")}, Protected: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/24")}}, nil
}
func lanHandlerContext(body, role, user string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	c.Params = gin.Params{{Key: "deviceId", Value: "gateway"}}
	c.Set(auth.ContextKeyBusinessID, "tenant")
	c.Set(auth.ContextKeyUserID, user)
	c.Set(auth.ContextKeyUserRole, role)
	return c, w
}
func TestLANAccessHTTPControlGuards(t *testing.T) {
	valid := `{"target_ip":"192.168.88.20","port":5000,"duration_minutes":10,"confirm_access":true}`
	for _, tc := range []struct {
		name, body, role, user string
		foreign, disabled      bool
		want, calls            int
	}{
		{"valid", valid, "OWNER", "owner", false, false, 201, 1},
		{"viewer", valid, "VIEWER", "owner", false, false, 403, 0},
		{"anonymous", valid, "OWNER", "", false, false, 401, 0},
		{"foreign inventory", valid, "OWNER", "owner", true, false, 403, 1},
		{"disabled", valid, "OWNER", "owner", false, true, 503, 0},
		{"spoofed identity", strings.TrimSuffix(valid, "}") + `,"business_id":"foreign"}`, "OWNER", "owner", false, false, 400, 0},
		{"no consent", strings.Replace(valid, `true`, `false`, 1), "OWNER", "owner", false, false, 400, 0},
		{"duration overflow", strings.Replace(valid, `:10`, `:9999999999999999999`, 1), "OWNER", "owner", false, false, 400, 0},
		{"public destination", strings.Replace(valid, "192.168.88.20", "8.8.8.8", 1), "OWNER", "owner", false, false, 403, 1},
		{"protected destination", strings.Replace(valid, "192.168.88.20", "10.77.0.2", 1), "OWNER", "owner", false, false, 403, 1},
		{"trailing JSON", valid + ` {}`, "OWNER", "owner", false, false, 400, 0},
		{"oversize", strings.Replace(valid, "192.168.88.20", strings.Repeat("a", 3000), 1), "OWNER", "owner", false, false, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := lanaccess.NewSessions(lanHandlerTransport{})
			defer sessions.Close()
			scope := &lanScopeStub{foreign: tc.foreign}
			h := &LANAccessHandler{Sessions: sessions, Scopes: scope}
			if tc.disabled {
				h.Scopes = nil
			}
			c, w := lanHandlerContext(tc.body, tc.role, tc.user)
			h.Create(c)
			if w.Code != tc.want || scope.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, scope.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable session response")
			}
			if tc.want == 201 && !strings.Contains(w.Body.String(), `"connectivity_verified":false`) {
				t.Fatal("claimed connectivity before dial")
			}
		})
	}
}
func TestLANAccessRevokeWorksWithoutInventory(t *testing.T) {
	sessions := lanaccess.NewSessions(lanHandlerTransport{})
	defer sessions.Close()
	h := &LANAccessHandler{Sessions: sessions, Scopes: &lanScopeStub{}}
	c, w := lanHandlerContext(`{"target_ip":"192.168.88.20","port":5000,"duration_minutes":10,"confirm_access":true}`, "OWNER", "owner")
	h.Create(c)
	var response struct {
		SessionID string `json:"session_id"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatal(w.Body.String())
	}
	h.Scopes = nil // Disconnected gateway / expired plan must not prevent cleanup.
	for _, user := range []string{"other-owner", "owner"} {
		c, w = lanHandlerContext("", "OWNER", user)
		c.Params = append(c.Params, gin.Param{Key: "sessionId", Value: response.SessionID})
		h.Revoke(c)
		c.Writer.WriteHeaderNow()
		want := 404
		if user == "owner" {
			want = 204
		}
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
