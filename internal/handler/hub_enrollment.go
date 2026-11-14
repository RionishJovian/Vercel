package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

type hubEnroller interface {
	EnrollPeer(context.Context, hub.Identity, string, int) (hub.Peer, error)
}

// hubPeerRemover ends a Hub allocation so a router that never connected can start over.
type hubPeerRemover interface {
	RemovePeer(context.Context, hub.Identity, string) error
}

// ErrHubManagementConfigured protects an existing saved connection from being
// overwritten by onboarding. It does not assert that the router is reachable.
var ErrHubManagementConfigured = errors.New("management already configured; use existing connection")

// Configure only after the Hub create-only contract has been staged and tested.
func (h *VPSHandler) ConfigureHubEnrollment(client hubEnroller) {
	h.enrollmentHub = client
	if remover, ok := client.(hubPeerRemover); ok {
		h.enrollmentRemover = remover
	}
}

// GatewayBootstrap produces a router-local key step only after proving tenant
// ownership and rollout readiness. It never issues a private key or Hub peer.
func (h *VPSHandler) GatewayBootstrap(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user := auth.GetBusinessID(c), auth.GetUserID(c)
	if biz == "" || user == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(403, gin.H{"error": "gateway management permission required"})
		return
	}
	if h.enrollmentHub == nil || h.liveDevice == nil || h.livePlan == nil {
		c.JSON(503, gin.H{"error": "Magic Hub enrollment is not enabled"})
		return
	}
	if !h.livePlan(biz) {
		h.planDenied(c)
		return
	}
	device, err := h.liveDevice(c, biz, c.Param("deviceId"))
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if device == nil || device.BusinessID != biz || device.DeviceID != c.Param("deviceId") || device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway not found in your tenant"})
		return
	}
	script, err := hub.BootstrapScript(device.DeviceID)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid gateway identity"})
		return
	}
	c.JSON(200, gin.H{"bootstrap_script": script, "state": "router_key_required", "router_configuration_required": true})
}

func (h *VPSHandler) EnrollGateway(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user := auth.GetBusinessID(c), auth.GetUserID(c)
	if biz == "" || user == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(403, gin.H{"error": "gateway management permission required"})
		return
	}
	if h.enrollmentHub == nil || h.liveDevice == nil || h.livePlan == nil {
		c.JSON(503, gin.H{"error": "Magic Hub enrollment is not enabled"})
		return
	}
	if !h.livePlan(biz) {
		h.planDenied(c)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var body struct {
		PublicKey string `json:"router_public_key"`
		Port      int    `json:"management_rest_port"`
		Confirm   bool   `json:"confirm_enrollment"`
	}
	if c.ShouldBindJSON(&body) != nil || !body.Confirm || body.Port < 1 || body.Port > 65535 {
		c.JSON(400, gin.H{"error": "confirm enrollment with a router public key and management port"})
		return
	}
	key, err := base64.StdEncoding.DecodeString(body.PublicKey)
	zero := true
	for _, b := range key {
		if b != 0 {
			zero = false
		}
	}
	if err != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != body.PublicKey || zero {
		c.JSON(400, gin.H{"error": "valid WireGuard public key required; never submit a private key"})
		return
	}
	device, err := h.liveDevice(c, biz, c.Param("deviceId"))
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if device == nil || device.BusinessID != biz || device.DeviceID != c.Param("deviceId") || device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway not found in your tenant"})
		return
	}
	identity := hub.Identity{TenantID: biz, RouterID: device.DeviceID, RequestedBy: user}
	ctx, cancel := context.WithTimeout(c.Request.Context(), enrollmentDeadline)
	defer cancel()
	unlock, lockErr := h.enrollLocks.Lock(ctx, biz+"/"+device.DeviceID)
	if lockErr != nil {
		c.JSON(503, gin.H{"error": "another setup for this router is still running; wait a moment and retry", "code": "hub_enrollment_busy"})
		return
	}
	defer unlock()
	peer, err := h.enrollmentHub.EnrollPeer(ctx, identity, body.PublicKey, body.Port)
	if err != nil || peer.TenantID != biz || peer.RouterID != device.DeviceID || peer.RouterPublicKey != body.PublicKey || peer.EnrollmentVersion != 1 || peer.State != "active" {
		c.JSON(502, gin.H{"error": "enrollment not confirmed; retry with the same public key, do not generate a replacement key"})
		return
	}
	if e := h.recordAllocation(ctx, identity, peer, false); e != nil {
		c.JSON(502, gin.H{"error": "Hub allocation conflicts with another router; no setup was issued", "code": "hub_allocation_conflict"})
		return
	}
	scripts, err := hub.BuildRouterScripts(device.DeviceID, peer)
	if err != nil {
		c.JSON(502, gin.H{"error": "Hub allocation lacks safe script configuration; retry with the same router key"})
		return
	}
	disconnect, err := hub.BuildDisconnectScript(device.DeviceID, peer, hub.ManagementUserName(biz, device.DeviceID))
	if err != nil {
		c.JSON(502, gin.H{"error": "Hub allocation lacks safe script configuration; retry with the same router key"})
		return
	}
	c.JSON(200, gin.H{"peer_id": peer.PeerID, "device_id": device.DeviceID, "tunnel_address": peer.TunnelAddress,
		"hub_public_key": peer.HubPublicKey, "endpoint": peer.Endpoint, "allowed_ips": peer.AllowedIPs,
		"router_script": scripts.Setup, "disconnect_script": disconnect,
		"state": "hub_peer_enrolled", "lan_reachability_verified": false, "router_configuration_required": true})
}

// QuickEnrollGateway restores the original one-paste onboarding experience.
// The application creates a one-time router keypair in memory, enrolls only
// the public half with Hub, and returns a no-store script that installs the
// private half locally on the router. The private key is never persisted or
// sent to Hub.
func (h *VPSHandler) QuickEnrollGateway(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user := auth.GetBusinessID(c), auth.GetUserID(c)
	if biz == "" || user == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(403, gin.H{"error": "gateway management permission required"})
		return
	}
	if h.enrollmentHub == nil || h.liveDevice == nil || h.livePlan == nil {
		c.JSON(503, gin.H{"error": "Magic Hub enrollment is not enabled"})
		return
	}
	if h.automaticPrepare == nil {
		c.JSON(503, gin.H{"error": "automatic management enrollment unavailable"})
		return
	}
	if !h.livePlan(biz) {
		h.planDenied(c)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var body struct {
		Port    int  `json:"management_rest_port"`
		Confirm bool `json:"confirm_enrollment"`
	}
	if c.ShouldBindJSON(&body) != nil || !body.Confirm || body.Port < 1 || body.Port > 65535 {
		c.JSON(400, gin.H{"error": "confirm one-paste enrollment with a management port"})
		return
	}
	device, err := h.liveDevice(c, biz, c.Param("deviceId"))
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if device == nil || device.BusinessID != biz || device.DeviceID != c.Param("deviceId") || device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway not found in your tenant"})
		return
	}
	if h.automaticPreflight != nil {
		if e := h.automaticPreflight(c.Request.Context(), biz, device.DeviceID); e != nil {
			if errors.Is(e, ErrHubManagementConfigured) {
				c.JSON(409, gin.H{"error": "This router already has a saved management connection. Use Finish connection to verify it; no setup credentials were replaced.", "code": "hub_management_already_configured"})
				return
			}
			c.JSON(503, gin.H{"error": "One-paste preparation is unavailable on this server. No new Hub allocation was requested.", "code": "hub_management_preflight_failed"})
			return
		}
	}
	identity := hub.Identity{TenantID: biz, RouterID: device.DeviceID, RequestedBy: user}
	// One enrolment per router at a time: two concurrent pastes must not both
	// remove and re-create a peer, or one of them leaves an orphan allocation.
	ctx, cancel := context.WithTimeout(c.Request.Context(), enrollmentDeadline)
	defer cancel()
	unlock, lockErr := h.enrollLocks.Lock(ctx, biz+"/"+device.DeviceID)
	if lockErr != nil {
		c.JSON(503, gin.H{"error": "another setup for this router is still running; wait a moment and retry", "code": "hub_enrollment_busy"})
		return
	}
	defer unlock()
	var peer hub.Peer
	var script string
	freshPeer := false
	if h.liveHub != nil {
		peer, err = h.liveHub.InspectRouter(ctx, identity)
		if err != nil && !errors.Is(err, hub.ErrPeerNotFound) {
			c.JSON(503, gin.H{"error": "Hub lookup unavailable; no enrollment changed"})
			return
		}
	}
	if peer.PeerID != "" {
		if peer.TenantID != biz || peer.RouterID != device.DeviceID {
			c.JSON(502, gin.H{"error": "Magic Hub identity mismatch"})
			return
		}
		key, keyErr := h.quickEnrollmentKeys(biz, device.DeviceID, false)
		if keyErr != nil {
			c.JSON(503, gin.H{"error": "Pending installation could not be read safely; enrollment preserved"})
			return
		}
		if key.public != "" && key.public == peer.RouterPublicKey {
			script, err = hub.BuildQuickConnectScript(device.DeviceID, peer, key.private)
			if err != nil {
				c.JSON(502, gin.H{"error": "Hub allocation lacks safe script configuration"})
				return
			}
		} else {
			// A router that has handshaken is connected. Its server-held key was
			// purged on purpose at that handshake, and replacing the peer now would
			// cut off a working tunnel, so refuse instead of re-allocating.
			if peer.LastHandshakeAt != nil {
				c.JSON(409, gin.H{"error": "This router is already connected. Use Finish connection; its tunnel was not changed.", "code": "hub_peer_already_connected"})
				return
			}
			// This router never finished connecting (the preflight above refuses one with a
			// saved connection) and its setup key is gone. Give it a fresh allocation: the
			// new script corrects whatever an earlier paste left on the router.
			if h.enrollmentRemover == nil || h.automaticPreflight == nil {
				c.JSON(409, gin.H{"error": "existing router key cannot be recovered; enrollment preserved", "code": "hub_router_key_unavailable"})
				return
			}
			if err = h.enrollmentRemover.RemovePeer(ctx, identity, peer.PeerID); err != nil {
				c.JSON(503, gin.H{"error": "Hub could not replace the old allocation; try again"})
				return
			}
			peer = hub.Peer{}
		}
	}
	if peer.PeerID == "" {
		key, err := h.quickEnrollmentKeys(biz, device.DeviceID, true)
		if err != nil {
			c.JSON(503, gin.H{"error": "router key preparation temporarily unavailable"})
			return
		}
		peer, err = h.enrollmentHub.EnrollPeer(ctx, identity, key.public, body.Port)
		if err != nil || peer.TenantID != biz || peer.RouterID != device.DeviceID || peer.RouterPublicKey != key.public || peer.EnrollmentVersion != 1 || peer.State != "active" {
			c.JSON(502, gin.H{"error": "enrollment not confirmed; retry the one-paste action"})
			return
		}
		freshPeer = true
		script, err = hub.BuildQuickConnectScript(device.DeviceID, peer, key.private)
		if err != nil {
			c.JSON(502, gin.H{"error": "Hub allocation lacks safe script configuration"})
			return
		}
	}
	if e := h.recordAllocation(ctx, identity, peer, freshPeer); e != nil {
		c.JSON(502, gin.H{"error": "Hub allocation conflicts with another router; no setup was issued", "code": "hub_allocation_conflict"})
		return
	}
	script, err = h.automaticPrepare(ctx, biz, device.DeviceID, peer.RouterPublicKey, script)
	if err != nil {
		if errors.Is(err, ErrHubManagementConfigured) {
			c.JSON(409, gin.H{"error": "This router already has a saved management connection. Use Finish connection to verify it; no setup credentials were replaced.", "code": "hub_management_already_configured"})
			return
		}
		c.JSON(503, gin.H{"error": "automatic management preparation failed"})
		return
	}
	c.JSON(200, gin.H{"peer_id": peer.PeerID, "device_id": device.DeviceID, "tunnel_address": peer.TunnelAddress,
		"hub_public_key": peer.HubPublicKey, "endpoint": peer.Endpoint, "allowed_ips": peer.AllowedIPs,
		"router_script": script, "state": "hub_peer_enrolled", "lan_reachability_verified": false,
		"router_configuration_required": true, "one_paste": true, "automatic_management": true})
}
