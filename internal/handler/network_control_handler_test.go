package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
