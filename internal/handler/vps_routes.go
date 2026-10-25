package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

// RegisterVPSRoutes registers the VPS peer-provisioning and Magic Hub onboarding routes.
// It is called from server.setupRoutes and from handler tests to share production route registration.
func RegisterVPSRoutes(
	vpsg *gin.RouterGroup,
	vpsHandler *VPSHandler,
) {
	// Read-only Hub config and peer info (plan-gated via handler.SubscriptionAccess in protected group)
	vpsg.GET("/config", vpsHandler.HubConfig)
	vpsg.GET("/peers/:deviceId", vpsHandler.Peer)
	vpsg.GET("/peers/:deviceId/health", vpsHandler.Health)

	// Read-only deployment availability used by the published Hub workflow.
	vpsg.GET("/enrollment-capability", auth.RequireMinRole(model.RoleAdmin), vpsHandler.EnrollmentCapability)

	// Live handshake requires Admin
	vpsg.GET("/peers/:deviceId/live", auth.RequireMinRole(model.RoleAdmin), vpsHandler.LivePeer)

	// Site assignment for Magic Hub onboarding (read + write) - Admin only
	vpsg.GET("/peers/:deviceId/site", auth.RequireMinRole(model.RoleAdmin), vpsHandler.GatewaySite)
	vpsg.PUT("/peers/:deviceId/site", auth.RequireMinRole(model.RoleAdmin), vpsHandler.GatewaySite)

	// Enrollment requires a site assignment; guard enforces tenant/site match.
	vpsg.POST("/peers/:deviceId/enroll", auth.RequireMinRole(model.RoleAdmin), vpsHandler.RequireGatewaySite, vpsHandler.EnrollGateway)
	vpsg.POST("/peers/:deviceId/quick-enroll", auth.RequireMinRole(model.RoleAdmin), vpsHandler.RequireGatewaySite, vpsHandler.QuickEnrollGateway)

	// Connection completion and management authentication require site assignment.
	vpsg.POST("/peers/:deviceId/finish", auth.RequireMinRole(model.RoleAdmin), vpsHandler.RequireGatewaySite, vpsHandler.FinishGatewayConnection)
	vpsg.GET("/peers/:deviceId/bootstrap", auth.RequireMinRole(model.RoleAdmin), vpsHandler.RequireGatewaySite, vpsHandler.GatewayBootstrap)
	vpsg.POST("/peers/:deviceId/authenticate", auth.RequireMinRole(model.RoleAdmin), vpsHandler.RequireGatewaySite, vpsHandler.AuthenticateGateway)
}
