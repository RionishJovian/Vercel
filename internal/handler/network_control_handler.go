package handler

import (
	"context"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/provisioner/routeros"
	"net/http"
	"os"
	"time"
)

type networkConfigurator interface {
	ReadNetwork(context.Context, string, string) (routeros.NetworkInventory, error)
	PreviewNetwork(context.Context, string, string, routeros.NetworkRequest) (routeros.NetworkReview, error)
	ApplyNetwork(context.Context, string, string, routeros.NetworkRequest) (routeros.NetworkResult, error)
}

// NetworkControl accepts typed operations only. Roles are checked at routing,
// device lookup and transport are tenant-bound, and audit text contains no
// browser values, RouterOS command output, usernames or credentials.
func (h *SecurityHandler) NetworkControl(c *gin.Context) {
	enabled := os.Getenv("MM_NETWORK_CONTROLS_ENABLED") == "true"
	if (c.Param("action") == "preview" || c.Param("action") == "apply") && !enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Network changes are held for router acceptance; this release has not enabled production network writes"})
		return
	}
	var request routeros.NetworkRequest
	if c.Param("action") == "preview" || c.Param("action") == "apply" {
		if c.Request.Method != http.MethodPost {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		if c.ShouldBindJSON(&request) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid network request"})
			return
		}
		if (request.Kind == "wifi" || request.Kind == "wireless") && os.Getenv("MM_NETWORK_WIFI_CONTROLS_ENABLED") != "true" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Wi-Fi writes are held for physical-radio acceptance; no router was changed"})
			return
		}
	}
	b, d := auth.GetBusinessID(c), c.Param("id")
	controller, ok := h.quickConfig.(networkConfigurator)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "network control is unavailable"})
		return
	}
	if !h.deviceExists(c, b, d) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Second)
	defer cancel()
	switch c.Param("action") {
	case "overview":
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		reader, ok := controller.(interface {
			ReadRouterOverview(context.Context, string, string) (routeros.NetworkInventory, error)
		})
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Router overview is unavailable"})
			return
		}
		inventory, err := reader.ReadRouterOverview(ctx, b, d)
		if err != nil {
			respondRouterReadError(c, err, "Could not read router health safely")
			return
		}
		inventory.WriteEnabled = enabled
		c.JSON(http.StatusOK, gin.H{"inventory": inventory})
	case "inventory":
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		inventory, err := controller.ReadNetwork(ctx, b, d)
		if err != nil {
			respondRouterReadError(c, err, "Could not read network inventory safely")
			return
		}
		inventory.WriteEnabled = enabled
		inventory.WiFiWriteEnabled = enabled && os.Getenv("MM_NETWORK_WIFI_CONTROLS_ENABLED") == "true"
		c.JSON(http.StatusOK, gin.H{"inventory": inventory})
	case "preview", "apply":
		if c.Request.Method != http.MethodPost {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		if c.Param("action") == "preview" {
			review, err := controller.PreviewNetwork(ctx, b, d, request)
			if err != nil {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"review": review})
			return
		}
		result, err := controller.ApplyNetwork(ctx, b, d, request)
		action := "router.network.applied"
		reason := "Typed network operation applied and readback verified"
		if err != nil {
			action = "router.network.failed"
			reason = "Typed network operation failed; inspect backup and rollback state"
		}
		auditErr := runInTenant(c, h.db, b, func(tx *sql.Tx) error {
			return h.deviceRepo.RecordChangeTXN(tx, b, d, auth.GetUserRole(c), action, reason)
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "result": result})
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result, "audit_recorded": auditErr == nil})
	default:
		c.Status(http.StatusNotFound)
	}
}

// Missing management setup is an expected prerequisite, not an upstream outage.
// Never forward transport/DB errors: they can contain private connection facts.
func respondRouterReadError(c *gin.Context, err error, fallback string) {
	if errors.Is(err, routeros.ErrNoTarget) || errors.Is(err, routeros.ErrIncompleteTarget) || errors.Is(err, routeros.ErrHostKeyUnpinned) {
		c.JSON(http.StatusConflict, gin.H{
			"code":  "router_management_required",
			"error": "This router has no usable saved management connection. Complete Connect Magic Hub, then refresh status. No router settings were changed.",
		})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": fallback})
}
