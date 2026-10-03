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

// Configure only after the Hub create-only contract has been staged and tested.
func (h *VPSHandler) ConfigureHubEnrollment(client hubEnroller) { h.enrollmentHub = client }

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
	peer, err := h.enrollmentHub.EnrollPeer(c.Request.Context(), hub.Identity{TenantID: biz, RouterID: device.DeviceID, RequestedBy: user}, body.PublicKey, body.Port)
	if err != nil || peer.TenantID != biz || peer.RouterID != device.DeviceID || peer.RouterPublicKey != body.PublicKey || peer.EnrollmentVersion != 1 || peer.State != "active" {
		c.JSON(502, gin.H{"error": "enrollment not confirmed; retry with the same public key, do not generate a replacement key"})
		return
	}
	scripts, err := hub.BuildRouterScripts(device.DeviceID, peer)
	if err != nil {
		c.JSON(502, gin.H{"error": "Hub allocation lacks safe script configuration; retry with the same router key"})
		return
	}
	c.JSON(200, gin.H{"peer_id": peer.PeerID, "device_id": device.DeviceID, "tunnel_address": peer.TunnelAddress,
		"hub_public_key": peer.HubPublicKey, "endpoint": peer.Endpoint, "allowed_ips": peer.AllowedIPs,
		"router_script": scripts.Setup, "disconnect_script": scripts.Disconnect,
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
	identity := hub.Identity{TenantID: biz, RouterID: device.DeviceID, RequestedBy: user}
	var peer hub.Peer
	var script string
	if h.liveHub != nil {
		peer, err = h.liveHub.InspectRouter(c.Request.Context(), identity)
		if err != nil && !errors.Is(err, hub.ErrPeerNotFound) {
			c.JSON(503, gin.H{"error": "Hub lookup unavailable; no enrollment changed"})
			return
		}
	}
	if peer.PeerID != "" {
		scripts, buildErr := hub.BuildRouterScripts(device.DeviceID, peer)
		if buildErr != nil {
			c.JSON(409, gin.H{"error": "existing enrollment requires repair"})
			return
		}
		script = scripts.Setup
	} else {
		privateKey, publicKey, err := hub.GenerateRouterKeypair()
		if err != nil {
			c.JSON(500, gin.H{"error": "could not generate router keypair"})
			return
		}
		peer, err = h.enrollmentHub.EnrollPeer(c.Request.Context(), identity, publicKey, body.Port)
		if err != nil || peer.TenantID != biz || peer.RouterID != device.DeviceID || peer.RouterPublicKey != publicKey || peer.EnrollmentVersion != 1 || peer.State != "active" {
			c.JSON(502, gin.H{"error": "enrollment not confirmed; retry the one-paste action"})
			return
		}
		script, err = hub.BuildQuickConnectScript(device.DeviceID, peer, privateKey)
		if err != nil {
			c.JSON(502, gin.H{"error": "Hub allocation lacks safe script configuration"})
			return
		}
	}
	script, err = h.automaticPrepare(c.Request.Context(), biz, device.DeviceID, peer.RouterPublicKey, script)
	if err != nil {
		c.JSON(503, gin.H{"error": "automatic management preparation failed"})
		return
	}
	c.JSON(200, gin.H{"peer_id": peer.PeerID, "device_id": device.DeviceID, "tunnel_address": peer.TunnelAddress,
		"hub_public_key": peer.HubPublicKey, "endpoint": peer.Endpoint, "allowed_ips": peer.AllowedIPs,
		"router_script": script, "state": "hub_peer_enrolled", "lan_reachability_verified": false,
		"router_configuration_required": true, "one_paste": true, "automatic_management": true})
}
