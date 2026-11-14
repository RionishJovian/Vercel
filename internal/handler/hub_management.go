package handler

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

// ConfigureManagementAuthentication injects a verify-before-save boundary.
// The callback must verify a pinned SSH host and persist only encrypted secrets.
func (h *VPSHandler) ConfigureManagementAuthentication(fn func(context.Context, string, string, string, string, string, int, string) error) {
	h.managementAuth = fn
}

func (h *VPSHandler) ConfigureAutomaticManagement(prepare func(context.Context, string, string, string, string) (string, error), verify func(context.Context, string, string, string, string) error) {
	h.automaticPrepare, h.automaticVerify = prepare, verify
}

// FinishGatewayConnection accepts no client-supplied credentials or address.
// The Hub supplies both the tunnel allocation and the expected router key.
func (h *VPSHandler) FinishGatewayConnection(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user, id := auth.GetBusinessID(c), auth.GetUserID(c), c.Param("deviceId")
	if biz == "" || user == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(403, gin.H{"error": "gateway management permission required"})
		return
	}
	if h.automaticVerify == nil || h.liveHub == nil || h.liveDevice == nil || h.livePlan == nil {
		c.JSON(503, gin.H{"error": "automatic management unavailable"})
		return
	}
	if !h.livePlan(biz) {
		h.planDenied(c)
		return
	}
	d, err := h.liveDevice(c, biz, id)
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if d == nil || d.BusinessID != biz || d.DeviceID != id || d.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway not found"})
		return
	}
	switch err := h.completeConnection(c.Request.Context(), biz, user, id); {
	case errors.Is(err, errHubNoRecentHandshake):
		c.JSON(409, gin.H{"code": "hub_handshake_unverified", "error": "No recent Magic Hub handshake is verified. The router may be offline or its tunnel may be unreachable."})
	case errors.Is(err, errHubInvalidAddress):
		c.JSON(502, gin.H{"error": "invalid Hub address"})
	case err != nil:
		c.JSON(409, gin.H{"code": "hub_management_unverified", "error": "Management not verified yet. Paste the complete setup script on the router; pasting it again is safe."})
	default:
		c.JSON(200, gin.H{"state": "management_authenticated", "device_id": id, "lan_reachability_verified": false})
	}
}

var (
	errHubNoRecentHandshake = errors.New("no recent Magic Hub handshake")
	errHubInvalidAddress    = errors.New("invalid Hub address")
)

// completeConnection verifies the router over its tunnel and saves the connection.
// The Hub supplies both the tunnel address and the expected router key.
func (h *VPSHandler) completeConnection(parent context.Context, biz, requester, id string) error {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	p, err := h.liveHub.InspectRouter(ctx, hub.Identity{TenantID: biz, RouterID: id, RequestedBy: requester})
	if err != nil || p.TenantID != biz || p.RouterID != id || p.State != "active" || p.LastHandshakeAt == nil || time.Since(*p.LastHandshakeAt) < 0 || time.Since(*p.LastHandshakeAt) > 5*time.Minute {
		return errHubNoRecentHandshake
	}
	h.observeVerifiedHandshake(ctx, biz, id, p)
	addr, err := netip.ParsePrefix(p.TunnelAddress)
	if err != nil || !addr.Addr().Is4() || !addr.Addr().IsPrivate() || addr.Bits() != 32 {
		return errHubInvalidAddress
	}
	return h.automaticVerify(ctx, biz, id, addr.Addr().String(), p.RouterPublicKey)
}

// CompletePendingConnection is the system task's way in: a pasted script connects
// the router with nobody watching. It reports whether the router has connected.
func (h *VPSHandler) CompletePendingConnection(ctx context.Context, biz, id string) (bool, error) {
	if h.automaticVerify == nil || h.liveHub == nil || h.livePlan == nil || !h.livePlan(biz) {
		return false, nil
	}
	err := h.completeConnection(ctx, biz, "hub-auto-connect", id)
	if errors.Is(err, errHubNoRecentHandshake) {
		return false, nil // The script may not have been pasted yet.
	}
	return err == nil, err
}

func (h *VPSHandler) AuthenticateGateway(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user, id := auth.GetBusinessID(c), auth.GetUserID(c), c.Param("deviceId")
	if biz == "" || user == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(403, gin.H{"error": "gateway management permission required"})
		return
	}
	if h.managementAuth == nil || h.liveHub == nil || h.liveDevice == nil || h.livePlan == nil {
		c.JSON(503, gin.H{"error": "management authentication unavailable"})
		return
	}
	if !h.livePlan(biz) {
		h.planDenied(c)
		return
	}
	device, err := h.liveDevice(c, biz, id)
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if device == nil || device.BusinessID != biz || device.DeviceID != id || device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway not found"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Port        int    `json:"port"`
		Fingerprint string `json:"fingerprint"`
		Confirm     bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&body) != nil || !body.Confirm || body.Port < 1 || body.Port > 65535 || strings.TrimSpace(body.Username) == "" || len(body.Username) > 128 || len(body.Password) == 0 || len(body.Password) > 512 || routeros.ValidateHostKeyFingerprint(body.Fingerprint) != nil {
		c.JSON(400, gin.H{"error": "confirm credentials and a valid SHA256 SSH host-key fingerprint"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	peer, err := h.liveHub.InspectRouter(ctx, hub.Identity{TenantID: biz, RouterID: id, RequestedBy: user})
	if err != nil || peer.TenantID != biz || peer.RouterID != id || peer.State != "active" || peer.LastHandshakeAt == nil || time.Since(*peer.LastHandshakeAt) < 0 || time.Since(*peer.LastHandshakeAt) > 5*time.Minute {
		c.JSON(409, gin.H{"error": "a verified recent Magic Hub handshake is required"})
		return
	}
	address, err := netip.ParsePrefix(peer.TunnelAddress)
	if err != nil || !address.Addr().Is4() || !address.Addr().IsPrivate() || address.Bits() != 32 {
		c.JSON(502, gin.H{"error": "invalid Hub management allocation"})
		return
	}
	// Neither a browser-supplied address nor arbitrary commands reach this callback.
	if h.managementAuth(ctx, biz, id, address.Addr().String(), body.Username, body.Password, body.Port, body.Fingerprint) != nil {
		c.JSON(502, gin.H{"error": "SSH verification or secure save failed. Check the host fingerprint, credentials and tunnel firewall. Credentials were not confirmed."})
		return
	}
	c.JSON(200, gin.H{"state": "management_authenticated", "device_id": id, "lan_reachability_verified": false})
}
