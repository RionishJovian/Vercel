package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/provisioner/routeros"
)

type portalDeployer interface {
	PortalProfiles(context.Context, string, string) ([]string, error)
	PreviewPortal(context.Context, string, string, routeros.PortalDeployment) (routeros.PortalDeploymentPlan, error)
	ApplyPortal(context.Context, string, string, routeros.PortalDeployment) (routeros.PortalDeploymentResult, error)
}

func (h *SecurityHandler) PortalDeployment(c *gin.Context) {
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	deployer, ok := h.quickConfig.(portalDeployer)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live portal deployment is unavailable"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	if c.Request.Method == http.MethodGet {
		profiles, err := deployer.PortalProfiles(c.Request.Context(), businessID, deviceID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not read HotSpot profiles; finish Magic Hub management connection first"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"profiles": profiles})
		return
	}
	var request routeros.PortalDeployment
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid portal deployment"})
		return
	}
	if c.Param("action") == "preview" {
		plan, err := deployer.PreviewPortal(c.Request.Context(), businessID, deviceID, request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"plan": plan})
		return
	}
	if c.Param("action") != "apply" {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown portal deployment action"})
		return
	}
	result, err := deployer.ApplyPortal(c.Request.Context(), businessID, deviceID, request)
	action, reason := "router.portal.deployed", "portal login page and private management readback verified"
	if err != nil {
		action, reason = "router.portal.deploy.failed", err.Error()
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
