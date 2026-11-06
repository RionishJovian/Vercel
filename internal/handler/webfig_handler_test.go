package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/webfig"
)

const (
	webFigDevice  = "595dc44a-754d-487e-a92b-6b3b07927ba1"
	webFigOwner   = "business-a"
	webFigForeign = "business-b"
)

type webFigTargets struct{}

func (webFigTargets) TargetForDevice(_ context.Context, businessID, deviceID string) (*routeros.DeviceTarget, error) {
	if businessID != webFigOwner || deviceID != webFigDevice {
		return nil, routeros.ErrNoTarget
	}
	return &routeros.DeviceTarget{DeviceID: deviceID, Addr: "10.77.0.9:22"}, nil
}

type webFigFake struct {
	state         routeros.WebFigAccess
	reads, writes int
	lastBusiness  string
}

func (f *webFigFake) WebFigAccessState(_ context.Context, businessID, _, _, _ string) (routeros.WebFigAccess, error) {
	f.reads++
	f.lastBusiness = businessID
	return f.state, nil
}
func (f *webFigFake) EnableWebFigAccess(_ context.Context, _, _, _, _ string) (routeros.WebFigAccess, error) {
	f.writes++
	f.state.Ready = true
	return f.state, nil
}
func (f *webFigFake) DisableWebFigAccess(_ context.Context, _, _, _, _ string) (routeros.WebFigAccess, error) {
	f.writes++
	return f.state, nil
}

func webFigServer(t *testing.T, fake *webFigFake, role, business string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &SecurityHandler{}
	h.webFig = &WebFigControl{router: fake, targets: webFigTargets{}, store: webfig.NewStore(), host: "webfig.test", sourceCIDR: "10.77.0.1/32",
		record: func(*gin.Context, string, string, string, string) {}}
	r := gin.New()
	group := r.Group("/routers", func(c *gin.Context) {
		c.Set(auth.ContextKeyUserRole, role)
		c.Set(auth.ContextKeyBusinessID, business)
		c.Set(auth.ContextKeyUserID, "user-1")
		c.Set("auth_session_id", "session-1")
	})
	RegisterWebFigRoutes(group, h)
	return r
}

func webFigCall(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var webFigRoutes = []struct{ method, suffix, body string }{
	{http.MethodGet, "", ""},
	{http.MethodPost, "/access", `{"confirm":"ALLOW_WEBFIG"}`},
	{http.MethodDelete, "/access", ""},
	{http.MethodPost, "/session", ""},
}

func TestWebFigRefusesRolesBelowAdmin(t *testing.T) {
	t.Setenv("MM_WEBFIG_ACCESS", "open")
	fake := &webFigFake{state: routeros.WebFigAccess{Ready: true, Version: "7.25"}}
	for _, role := range []model.UserRole{model.RoleViewer, "", "GUEST"} {
		r := webFigServer(t, fake, string(role), webFigOwner)
		for _, route := range webFigRoutes {
			if w := webFigCall(r, route.method, "/routers/"+webFigDevice+"/webfig"+route.suffix, route.body); w.Code != http.StatusForbidden {
				t.Fatalf("role %q %s %s: %d", role, route.method, route.suffix, w.Code)
			}
		}
	}
	if fake.reads != 0 || fake.writes != 0 {
		t.Fatalf("router contacted: %+v", fake)
	}
}

func TestWebFigHidesAnotherTenantsRouter(t *testing.T) {
	t.Setenv("MM_WEBFIG_ACCESS", "open")
	fake := &webFigFake{state: routeros.WebFigAccess{Ready: true, Version: "7.25"}}
	r := webFigServer(t, fake, string(model.RoleOwner), webFigForeign)
	for _, route := range webFigRoutes {
		if w := webFigCall(r, route.method, "/routers/"+webFigDevice+"/webfig"+route.suffix, route.body); w.Code != http.StatusNotFound {
			t.Fatalf("%s %s: %d %s", route.method, route.suffix, w.Code, w.Body)
		}
	}
	if fake.reads != 0 || fake.writes != 0 {
		t.Fatalf("router contacted: %+v", fake)
	}
}

func TestWebFigSwitchOffAndLocked(t *testing.T) {
	fake := &webFigFake{state: routeros.WebFigAccess{Ready: true, Version: "7.25"}}
	r := webFigServer(t, fake, string(model.RoleOwner), webFigOwner)
	path := "/routers/" + webFigDevice + "/webfig"
	for _, tc := range []struct {
		env, access string
		code        int
	}{{"", "off", http.StatusServiceUnavailable}, {"locked", "locked", http.StatusForbidden}} {
		t.Setenv("MM_WEBFIG_ACCESS", tc.env)
		w := webFigCall(r, http.MethodGet, path, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"access":"`+tc.access+`"`) {
			t.Fatalf("status %q: %d %s", tc.env, w.Code, w.Body)
		}
		if w := webFigCall(r, http.MethodPost, path+"/session", ""); w.Code != tc.code {
			t.Fatalf("open %q: %d", tc.env, w.Code)
		}
		if w := webFigCall(r, http.MethodPost, path+"/access", `{"confirm":"ALLOW_WEBFIG"}`); w.Code != tc.code {
			t.Fatalf("allow %q: %d", tc.env, w.Code)
		}
	}
	if fake.reads != 0 || fake.writes != 0 {
		t.Fatalf("router contacted: %+v", fake)
	}
}

func TestWebFigAllowNeedsTheConfirmation(t *testing.T) {
	t.Setenv("MM_WEBFIG_ACCESS", "open")
	fake := &webFigFake{}
	r := webFigServer(t, fake, string(model.RoleOwner), webFigOwner)
	path := "/routers/" + webFigDevice + "/webfig/access"
	if w := webFigCall(r, http.MethodPost, path, `{}`); w.Code != http.StatusBadRequest || fake.writes != 0 {
		t.Fatalf("unconfirmed: %d writes=%d", w.Code, fake.writes)
	}
	if w := webFigCall(r, http.MethodPost, path, `{"confirm":"ALLOW_WEBFIG"}`); w.Code != 200 || fake.writes != 1 {
		t.Fatalf("confirmed: %d writes=%d", w.Code, fake.writes)
	}
}

func TestWebFigOpenRefusesUnreadyAndVulnerableRouters(t *testing.T) {
	t.Setenv("MM_WEBFIG_ACCESS", "open")
	path := "/routers/" + webFigDevice + "/webfig/session"
	for _, tc := range []struct {
		state routeros.WebFigAccess
		code  string
	}{
		{routeros.WebFigAccess{Ready: false, Version: "7.25", Plan: []string{"add rule"}}, "webfig_needs_setup"},
		{routeros.WebFigAccess{Ready: true, Version: "7.23.3"}, "webfig_update_required"},
		{routeros.WebFigAccess{Ready: true, Version: "7.24.1"}, "webfig_update_required"},
		{routeros.WebFigAccess{Ready: true, Version: ""}, "webfig_update_required"},
	} {
		r := webFigServer(t, &webFigFake{state: tc.state}, string(model.RoleOwner), webFigOwner)
		w := webFigCall(r, http.MethodPost, path, "")
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "ticket=") {
			t.Fatalf("%+v: %d %s", tc.state, w.Code, w.Body)
		}
	}
}

func TestWebFigOpenBindsTicketToTenantAndTunnelAddress(t *testing.T) {
	t.Setenv("MM_WEBFIG_ACCESS", "open")
	fake := &webFigFake{state: routeros.WebFigAccess{Ready: true, Version: "7.25", Scheme: "http", Port: 80}}
	r := webFigServer(t, fake, string(model.RoleOwner), webFigOwner)
	w := webFigCall(r, http.MethodPost, "/routers/"+webFigDevice+"/webfig/session", "")
	var body struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.HasPrefix(body.URL, "https://webfig.test/__mm/open?ticket=") || fake.lastBusiness != webFigOwner {
		t.Fatalf("url=%q business=%q", body.URL, fake.lastBusiness)
	}
}

func TestWebFigRecheckEndsAccessWhenThingsChange(t *testing.T) {
	grant := webfig.Grant{BusinessID: webFigOwner, DeviceID: webFigDevice, Host: "10.77.0.9"}
	t.Setenv("MM_WEBFIG_ACCESS", "open")
	if err := webFigStillAllowed(context.Background(), grant, "OWNER", webFigTargets{}); err != nil {
		t.Fatal(err)
	}
	if webFigStillAllowed(context.Background(), grant, "VIEWER", webFigTargets{}) == nil {
		t.Fatal("demoted user kept access")
	}
	moved := grant
	moved.BusinessID = webFigForeign
	if webFigStillAllowed(context.Background(), moved, "OWNER", webFigTargets{}) == nil {
		t.Fatal("router outside the tenant kept access")
	}
	readdressed := grant
	readdressed.Host = "10.77.0.10"
	if webFigStillAllowed(context.Background(), readdressed, "OWNER", webFigTargets{}) == nil {
		t.Fatal("changed address kept access")
	}
	t.Setenv("MM_WEBFIG_ACCESS", "locked")
	if webFigStillAllowed(context.Background(), grant, "OWNER", webFigTargets{}) == nil {
		t.Fatal("locking did not end the session")
	}
}
