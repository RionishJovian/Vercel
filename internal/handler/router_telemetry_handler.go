package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/routertelemetry"
)

// The retained interface has no router write methods. Inventory-only deployments
// can enable telemetry without enabling the recovery applier or guest actions.
func (h *SecurityHandler) enableRouterTelemetry(candidate any) {
	if reader, ok := candidate.(routertelemetry.Reader); ok {
		h.routerTelemetry = routertelemetry.New(reader)
	}
}
func (h *SecurityHandler) RouterTelemetry(c *gin.Context) {
	serveRouterTelemetry(c, h.routerTelemetry, h.deviceExists)
}
func serveRouterTelemetry(c *gin.Context, service *routertelemetry.Service, authorized func(*gin.Context, string, string) bool) {
	c.Header("Cache-Control", "no-store")
	business, device := auth.GetBusinessID(c), c.Param("id")
	if business == "" || device == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Sign in to read router telemetry", "code": "authentication_required"})
		return
	}
	// Re-check tenant ownership before serving any retained reading, including a
	// cache hit. A removed/reassigned device must not remain accessible via cache.
	if !authorized(c, business, device) {
		return
	}
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Router telemetry collector needs setup", "code": "telemetry_not_enabled"})
		return
	}
	view, err := service.Read(c.Request.Context(), business, device)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Telemetry refresh is retrying", "code": "telemetry_retrying"})
		return
	}
	c.Header("Retry-After", strconv.Itoa(view.RetryAfterSeconds))
	c.JSON(http.StatusOK, gin.H{"telemetry": view, "read_only": true, "router_changed": false})
}
