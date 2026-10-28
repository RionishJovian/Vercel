package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/routertelemetry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type telemetryHandlerReader struct{ calls int }

func (r *telemetryHandlerReader) ReadTelemetry(_ context.Context, b, d string) (routeros.RouterTelemetry, error) {
	r.calls++
	return routeros.RouterTelemetry{DeviceID: d, ReadAt: time.Now().UTC(), Issues: []string{}}, nil
}
func TestRouterTelemetryRoleOwnershipAndCacheRecheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &telemetryHandlerReader{}
	service := routertelemetry.New(reader)
	owned := true
	business := "a"
	role := "ADMIN"
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(auth.ContextKeyBusinessID, business); c.Set(auth.ContextKeyUserRole, role) })
	router.GET("/routers/:id/telemetry", auth.RequireMinRole(model.RoleAdmin), func(c *gin.Context) {
		serveRouterTelemetry(c, service, func(c *gin.Context, b, d string) bool {
			if owned && b == "a" && d == "router" {
				return true
			}
			c.JSON(404, gin.H{"error": "device not found"})
			return false
		})
	})
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/routers/router/telemetry", nil))
		return w
	}
	if response := request(); response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"read_only":true`) {
		t.Fatal(response.Code, response.Body.String())
	}
	owned = false
	if request().Code != 404 || reader.calls != 1 {
		t.Fatal("removed device served cached reading")
	}
	business = "b"
	if request().Code != 404 || reader.calls != 1 {
		t.Fatal("cross-tenant request collected")
	}
	role = "VIEWER"
	if request().Code != 403 || reader.calls != 1 {
		t.Fatal("read role gate bypassed")
	}
}
