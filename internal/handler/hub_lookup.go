package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

type hubInspector interface {
	InspectRouter(context.Context, hub.Identity) (hub.Peer, error)
}

// Labels describe app-side records, never proof of router authentication.
type HubConnectionState struct {
	Management string `json:"management"`
	Setup      string `json:"setup"`
}

func (h *VPSHandler) ConfigureHubConnectionState(read func(context.Context, string, string) (HubConnectionState, error)) {
	h.liveConnectionState = read
}

// ConfigureHubLookup enables only inspection, not enrollment or router writes.
func (h *VPSHandler) ConfigureHubLookup(client hubInspector) {
	h.liveHub = client
	h.livePlan = h.planActive
	h.liveDevice = func(c *gin.Context, businessID, deviceID string) (*model.Device, error) {
		var device *model.Device
		err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
			var err error
			device, err = h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
			return err
		})
		return device, err
	}
}

// LivePeer uses the actual Hub allocation. A configured peer is not necessarily
// online and a recent handshake is not evidence of LAN or REST reachability.
func (h *VPSHandler) LivePeer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	businessID, userID := auth.GetBusinessID(c), auth.GetUserID(c)
	if businessID == "" || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "gateway management permission required"})
		return
	}
	if h.liveHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Magic Hub lookup is not enabled", "code": "hub_lookup_disabled"})
		return
	}
	if !h.livePlan(businessID) {
		h.planDenied(c)
		return
	}
	device, err := h.liveDevice(c, businessID, c.Param("deviceId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if device == nil || device.BusinessID != businessID || device.DeviceID != c.Param("deviceId") || device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found in your tenant"})
		return
	}
	peer, err := h.liveHub.InspectRouter(c.Request.Context(), hub.Identity{TenantID: businessID, RouterID: device.DeviceID, RequestedBy: userID})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Magic Hub lookup unavailable; no connection changes were made", "code": "hub_lookup_failed"})
		return
	}
	// Defense in depth for alternate inspector implementations.
	if peer.TenantID != businessID || peer.RouterID != device.DeviceID {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Magic Hub identity mismatch"})
		return
	}
	h.observeVerifiedHandshake(c.Request.Context(), businessID, device.DeviceID, peer)
	status := "no_recent_handshake"
	if peer.State != "active" {
		status = "inactive"
	} else if peer.LastHandshakeAt != nil {
		age := time.Since(*peer.LastHandshakeAt)
		if age >= 0 && age <= 5*time.Minute {
			status = "recent_handshake"
		}
	}
	connection := HubConnectionState{Management: "unknown", Setup: "unknown"}
	if h.liveConnectionState != nil {
		read, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		value, e := h.liveConnectionState(read, businessID, device.DeviceID)
		cancel()
		if e == nil {
			connection = value
		}
	}
	c.JSON(http.StatusOK, gin.H{"device_id": device.DeviceID, "peer_id": peer.PeerID, "tunnel_address": peer.TunnelAddress, "state": peer.State, "handshake_status": status, "last_handshake_at": peer.LastHandshakeAt, "rx_bytes": peer.RXBytes, "tx_bytes": peer.TXBytes, "lan_reachability_verified": false, "connection": connection})
}
