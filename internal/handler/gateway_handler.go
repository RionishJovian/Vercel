package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/provisioner/routeros"
)

type gatewayConfigurator interface {
	GatewayInterfaces(context.Context, string, string) ([]routeros.GatewayInterface, error)
	PreviewGateway(context.Context, string, string, routeros.GatewaySetup) (routeros.GatewayPlan, error)
	ApplyGateway(context.Context, string, string, routeros.GatewaySetup) (routeros.GatewayResult, error)
}

func (h *SecurityHandler) GatewaySetup(c *gin.Context) {
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	configurator, ok := h.quickConfig.(gatewayConfigurator)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live gateway configuration is unavailable"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	if c.Request.Method == http.MethodGet {
		interfaces, err := configurator.GatewayInterfaces(c.Request.Context(), businessID, deviceID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not read router interfaces"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"interfaces": interfaces})
		return
	}
	var request routeros.GatewaySetup
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway settings"})
		return
	}
	if c.Param("action") == "preview" {
		plan, err := configurator.PreviewGateway(c.Request.Context(), businessID, deviceID, request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"plan": plan})
		return
	}
	if c.Param("action") != "apply" {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown gateway action"})
		return
	}
	result, err := configurator.ApplyGateway(c.Request.Context(), businessID, deviceID, request)
	action, reason := "router.gateway_setup", "created and verified customer network"
	if err != nil {
		action = "router.gateway_setup.failed"
		reason = err.Error()
	}
	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.deviceRepo.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "result": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
