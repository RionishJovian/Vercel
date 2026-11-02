package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func firmwareRoutes() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSecurityHandler(nil, nil)
	r := gin.New()
	r.POST("/routers/:id/firmware/:action", h.FirmwareControl)
	r.GET("/routers/:id/firmware/:action", h.FirmwareControl)
	return r
}

func TestFirmwareUpgradeHeldByDefaultBeforeAnyRouterLookup(t *testing.T) {
	t.Setenv("MM_FIRMWARE_UPGRADE_ENABLED", "")
	r := firmwareRoutes()
	for _, action := range []string{"preview", "apply"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/firmware/"+action, strings.NewReader(`{"confirmation":"x"}`)))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "held until") || !strings.Contains(w.Body.String(), "no router was changed") {
			t.Fatalf("unsafe default: %s %d %s", action, w.Code, w.Body.String())
		}
	}
}

func TestFirmwareUpgradeOnlyExactValuesEnableAnything(t *testing.T) {
	for _, value := range []string{"TRUE", "1", "yes", "enabled", " true", "previews"} {
		t.Setenv("MM_FIRMWARE_UPGRADE_ENABLED", value)
		w := httptest.NewRecorder()
		firmwareRoutes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/firmware/apply", strings.NewReader(`{"confirmation":"x"}`)))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "held until") {
			t.Fatalf("%q enabled firmware upgrades: %d %s", value, w.Code, w.Body.String())
		}
	}
}

func TestFirmwareUpgradePreviewOnlyModeNeverApplies(t *testing.T) {
	t.Setenv("MM_FIRMWARE_UPGRADE_ENABLED", "preview")
	w := httptest.NewRecorder()
	firmwareRoutes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/firmware/apply", strings.NewReader(`{"confirmation":"x"}`)))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "preview-only") || !strings.Contains(w.Body.String(), "no router was changed") {
		t.Fatalf("preview-only mode applied: %d %s", w.Code, w.Body.String())
	}
}

func TestFirmwareUpgradeApplyNeedsAReviewedConfirmation(t *testing.T) {
	t.Setenv("MM_FIRMWARE_UPGRADE_ENABLED", "true")
	for _, body := range []string{"", "{}", `{"confirmation":""}`, "not json"} {
		w := httptest.NewRecorder()
		firmwareRoutes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/firmware/apply", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %q was accepted: %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestFirmwareUpgradeRejectsWrongMethodsAndUnknownActions(t *testing.T) {
	t.Setenv("MM_FIRMWARE_UPGRADE_ENABLED", "true")
	r := firmwareRoutes()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/routers/device/firmware/apply", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET apply: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/routers/device/firmware/reboot", strings.NewReader("{}")))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown action: %d", w.Code)
	}
}
