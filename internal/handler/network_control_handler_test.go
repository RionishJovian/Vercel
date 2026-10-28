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
