package handler

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/provisioner/routeros"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouterReadErrorsDistinguishSetupWithoutLeakingConnectionFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		err    error
		status int
		setup  bool
	}{
		{"unconfigured", routeros.ErrNoTarget, http.StatusConflict, true},
		{"wrapped", fmt.Errorf("private connection facts: %w", routeros.ErrNoTarget), http.StatusConflict, true},
		{"incomplete", routeros.ErrIncompleteTarget, http.StatusConflict, true},
		{"unpinned", routeros.ErrHostKeyUnpinned, http.StatusConflict, true},
		{"blocked", routeros.ErrTargetNotAllowed, http.StatusBadGateway, false},
		{"transport", errors.New("private connection facts"), http.StatusBadGateway, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondRouterReadError(c, tc.err, "Could not read network inventory safely")
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private connection facts") {
				t.Fatalf("unsafe error response: %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "router_management_required") != tc.setup {
				t.Fatalf("incorrect prerequisite classification: %s", w.Body.String())
			}
		})
	}
}

func TestNetworkWritesDefaultOffBeforeAnyRouterLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "")
	h := NewSecurityHandler(nil, nil)
	r := gin.New()
	r.POST("/routers/:id/network/:action", h.NetworkControl)
	for _, action := range []string{"preview", "apply"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/network/"+action, strings.NewReader(`{"kind":"wan"}`)))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "held for router acceptance") {
			t.Fatalf("unsafe default: %s %d %s", action, w.Code, w.Body.String())
		}
	}
}

func TestNetworkWiFiSeparatelyHeldBeforeAnyRouterLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "true")
	t.Setenv("MM_NETWORK_WIFI_CONTROLS_ENABLED", "")
	h := NewSecurityHandler(nil, nil)
	r := gin.New()
	r.POST("/routers/:id/network/:action", h.NetworkControl)
	for _, kind := range []string{"wifi", "wireless"} {
		for _, action := range []string{"preview", "apply"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/network/"+action, strings.NewReader(`{"kind":"`+kind+`"}`)))
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "physical-radio acceptance") {
				t.Fatalf("Wi-Fi hold bypassed: %d %s", w.Code, w.Body.String())
			}
		}
	}
}

func TestNetworkAddressAndVLANSeparatelyHeldBeforeAnyRouterLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "true")
	t.Setenv("MM_NETWORK_ADDRESS_CONTROLS_ENABLED", "")
	h := NewSecurityHandler(nil, nil)
	r := gin.New()
	r.POST("/routers/:id/network/:action", func(c *gin.Context) { c.Set("business_id", "biz-1"); h.NetworkControl(c) })
	for _, kind := range []string{"ip_address", "vlan"} {
		for _, action := range []string{"preview", "apply"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/network/"+action, strings.NewReader(`{"kind":"`+kind+`"}`)))
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "held until they have been tested on a router") {
				t.Fatalf("address/VLAN hold bypassed for %s %s: %d %s", kind, action, w.Code, w.Body.String())
			}
		}
	}
	// With the switch on, the request goes on to the normal path (here it stops at the
	// missing network controller instead of the hold), proving the hold is the only gate.
	t.Setenv("MM_NETWORK_ADDRESS_CONTROLS_ENABLED", "true")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/network/preview", strings.NewReader(`{"kind":"vlan"}`)))
	if strings.Contains(w.Body.String(), "held until they have been tested") {
		t.Fatalf("the hold still applies with the switch on: %s", w.Body.String())
	}
	// Other kinds are unaffected by this switch.
	t.Setenv("MM_NETWORK_ADDRESS_CONTROLS_ENABLED", "")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/network/preview", strings.NewReader(`{"kind":"dhcp"}`)))
	if strings.Contains(w.Body.String(), "held until they have been tested") {
		t.Fatalf("the address switch must not hold unrelated operations: %s", w.Body.String())
	}
}
