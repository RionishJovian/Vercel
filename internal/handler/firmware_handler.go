package handler

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/provisioner/routeros"
)

type firmwareUpgrader interface {
	PreviewFirmware(context.Context, string, string) (routeros.FirmwareReview, error)
	ApplyFirmware(context.Context, string, string, string) (routeros.FirmwareResult, error)
}

// firmwareMode is the MM_FIRMWARE_UPGRADE_ENABLED switch: "true" allows preview and apply,
// "preview" allows only the read-only preview, anything else holds both.
func firmwareMode() string {
	switch v := os.Getenv("MM_FIRMWARE_UPGRADE_ENABLED"); v {
	case "true", "preview":
		return v
	}
	return "held"
}

// FirmwareControl previews and applies a RouterBOARD firmware upgrade. Roles are checked at routing,
// the device lookup is tenant-bound, and audit text carries no router output or credentials.
func (h *SecurityHandler) FirmwareControl(c *gin.Context) {
	action, mode := c.Param("action"), firmwareMode()
	if action == "status" {
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		if !h.deviceExists(c, auth.GetBusinessID(c), c.Param("id")) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"mode": mode})
		return
	}
	if action != "preview" && action != "apply" {
		c.Status(http.StatusNotFound)
		return
	}
	if c.Request.Method != http.MethodPost {
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	switch {
	case mode == "held":
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firmware upgrades are held until they have been tested on a router; no router was changed"})
		return
	case mode == "preview" && action == "apply":
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Firmware upgrades are in preview-only mode: the router was checked but nothing can be applied yet; no router was changed"})
		return
	}
	var request struct {
		Confirmation string `json:"confirmation"`
	}
	if action == "apply" {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		if c.ShouldBindJSON(&request) != nil || request.Confirmation == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Review the upgrade before applying it"})
			return
		}
	}
	b, d := auth.GetBusinessID(c), c.Param("id")
	upgrader, ok := h.quickConfig.(firmwareUpgrader)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "firmware upgrade is unavailable"})
		return
	}
	if !h.deviceExists(c, b, d) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Second)
	defer cancel()
	if action == "preview" {
		review, err := upgrader.PreviewFirmware(ctx, b, d)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"review": review})
		return
	}
	result, err := upgrader.ApplyFirmware(ctx, b, d, request.Confirmation)
	event, reason := "router.firmware.upgrade_started", "Board firmware written and restart scheduled; the owner must confirm the router reports the new version"
	if err != nil {
		event, reason = "router.firmware.failed", "Board firmware upgrade did not complete; check the router before trying again"
	}
	auditErr := runInTenant(c, h.db, b, func(tx *sql.Tx) error {
		return h.deviceRepo.RecordChangeTXN(tx, b, d, auth.GetUserRole(c), event, reason)
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "result": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result, "audit_recorded": auditErr == nil})
}
