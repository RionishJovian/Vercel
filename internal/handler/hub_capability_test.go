package handler

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"net/http/httptest"
	"testing"
)

func TestEnrollmentCapabilityFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		configure  func(*VPSHandler)
		available  bool
	}{
		{"disabled", "hub_lookup_disabled", func(*VPSHandler) {}, false},
		{"lookup only", "hub_enrollment_disabled", func(h *VPSHandler) { h.liveHub = managementInspector{}; h.livePlan = func(string) bool { return true } }, false},
		{"no automatic management", "automatic_management_disabled", func(h *VPSHandler) {
			h.liveHub = managementInspector{}
			h.enrollmentHub = &enrollmentStub{}
			h.liveDevice = func(*gin.Context, string, string) (*model.Device, error) { return nil, nil }
			h.livePlan = func(string) bool { return true }
		}, false},
		{"plan required", "plan_required", func(h *VPSHandler) {
			h.liveHub = managementInspector{}
			h.enrollmentHub = &enrollmentStub{}
			h.liveDevice = func(*gin.Context, string, string) (*model.Device, error) { return nil, nil }
			h.livePlan = func(string) bool { return false }
			h.automaticPrepare = func(context.Context, string, string, string, string) (string, error) { return "", nil }
			h.automaticVerify = func(context.Context, string, string, string, string) error { return nil }
		}, false},
		{"ready", "ready", func(h *VPSHandler) {
			h.liveHub = managementInspector{}
			h.enrollmentHub = &enrollmentStub{}
			h.liveDevice = func(*gin.Context, string, string) (*model.Device, error) { return nil, nil }
			h.livePlan = func(string) bool { return true }
			h.automaticPrepare = func(context.Context, string, string, string, string) (string, error) { return "", nil }
			h.automaticVerify = func(context.Context, string, string, string, string) error { return nil }
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &VPSHandler{}
			tc.configure(h)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Set(auth.ContextKeyBusinessID, "tenant")
			h.EnrollmentCapability(c)
			var body struct {
				Available bool   `json:"available"`
				Code      string `json:"code"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Code != tc.code || body.Available != tc.available {
				t.Fatal(w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("capability response cacheable")
			}
		})
	}
}

func TestEnrollmentCapabilityHTTPAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &VPSHandler{}
	r := gin.New()
	group := r.Group("/api/v1/vps", auth.Middleware(testAuthService()))
	RegisterVPSRoutes(group, h)
	for _, tc := range []struct {
		name, role string
		status     int
	}{{"anonymous", "", 401}, {"viewer", "VIEWER", 403}, {"admin", "ADMIN", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/vps/enrollment-capability", nil)
			if tc.role != "" {
				req.Header.Set("Authorization", "Bearer "+makeTestToken(t, "user-admin", "tenant", tc.role))
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("expected %d got %d: %s", tc.status, w.Code, w.Body.String())
			}
			if tc.status == 200 {
				var b map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
					t.Fatal(err)
				}
				if len(b) != 6 || b["available"] != false || b["lookup_enabled"] != false || b["code"] != "hub_lookup_disabled" {
					t.Fatalf("unexpected capability data: %v", b)
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("availability must not be cached")
				}
			}
		})
	}
}
