package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
)

// Retired ingestion must reject valid-looking measurements as well as malformed
// bodies before touching a database, cache or publisher, for every app role.
func TestLegacyTelemetryIngestCannotPublishClientMeasurements(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"VIEWER", "ADMIN", "OWNER", "DEVELOPER"} {
		for _, body := range []string{`{"subject_kind":"router","subject_id":"fake","cpu_pct":99,"wan_state":"down"}`, `invalid`, ``} {
			t.Run(role+"/"+body, func(t *testing.T) {
				r := gin.New()
				r.POST("/telemetry", func(c *gin.Context) {
					c.Set(auth.ContextKeyUserRole, role)
					c.Set(auth.ContextKeyBusinessID, "tenant")
				}, (&TelemetryHandler{}).Ingest)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/telemetry", strings.NewReader(body)))
				if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), `"code":"telemetry_ingest_retired"`) {
					t.Fatalf("client measurement accepted: %d %s", w.Code, w.Body.String())
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("retirement response must not be cached")
				}
			})
		}
	}
}
