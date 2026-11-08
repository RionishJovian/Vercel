package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

// One route contract is used by production and its authenticated integration tests.
func RegisterRouterObservationRoutes(routers *gin.RouterGroup, h *RouterHandler) {
	routers.GET("/:id/observations", h.GetRouterObservations)
	routers.GET("/:id/discovery", h.GetRouterDiscovery)
	routers.GET("/:id/discovery-labels", h.GetDiscoveryLabels)
	routers.PUT("/:id/discovery-labels", auth.RequireMinRole(model.RoleAdmin), h.UpsertDiscoveryLabel)
	routers.POST("/:id/guests/disconnect", auth.RequireMinRole(model.RoleAdmin), h.DisconnectGuest)
	routers.POST("/:id/guests/block", auth.RequireMinRole(model.RoleAdmin), h.BlockGuest)
	routers.POST("/:id/guests/unblock", auth.RequireMinRole(model.RoleAdmin), h.UnblockGuest)
}
