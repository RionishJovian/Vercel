package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The switch sits in front of everything else, so nothing reaches a router while it is off.
func TestDeviceControlKindsHeldBySwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/routers/:id/network/:action", (&SecurityHandler{}).NetworkControl)
	send := func(action, kind string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/network/"+action, strings.NewReader(`{"kind":"`+kind+`","values":{}}`)))
		return w
	}
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "true")
	for _, kind := range []string{"device_block", "device_unblock", "port_label", "port_state", "poe_cycle"} {
		for _, action := range []string{"preview", "apply"} {
			w := send(action, kind)
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "no router was changed") {
				t.Fatalf("%s %s: %d %s", action, kind, w.Code, w.Body)
			}
		}
	}
	// The general network switch does not open them, and they cannot be opened without it.
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "")
	t.Setenv("MM_DEVICE_CONTROLS_ENABLED", "true")
	if deviceControlsEnabled() {
		t.Fatal("device controls need the network switch too")
	}
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "true")
	if !deviceControlsEnabled() {
		t.Fatal("both switches should open device controls")
	}
}
