package handler

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/guestaccess"
)

// Recover accepts ownership proof only from the private paid-access cookie.
// Tenant and gateway are resolved server-side. No grace grant, voucher redemption
// or business subscription mutation is part of this operation.
func (h *GuestVoucherHandler) Recover(c *gin.Context) {
	req, ok := h.paidGuestRequest(c, h.Recoverer != nil)
	if !ok {
		return
	}
	g, err := h.Recoverer.Restore(c.Request.Context(), req)
	if err != nil {
		status := 403
		if errors.Is(err, guestaccess.ErrRecoveryNeedsRepair) {
			status = 503
		}
		c.JSON(status, gin.H{"error": "paid access recovery could not be confirmed", "code": "guest_recovery_unavailable"})
		return
	}
	if g.BusinessID != req.BusinessID || g.DeviceID != req.DeviceID || g.VoucherID != req.VoucherID || g.NewMAC != req.NewMAC || g.OldMAC == g.NewMAC || !guestVoucherUUID.MatchString(g.TransferID) ||
		!time.Now().Before(g.ExpiresAt) || g.TotalBytes <= 0 || g.UsedBytes < 0 || g.UsedBytes >= g.TotalBytes {
		c.JSON(503, gin.H{"error": "paid access recovery verification incomplete"})
		return
	}
	// Allocation at transfer completion, not a live usage counter. Preserve the
	// original cookie and its expiry; do not mint fresh ownership or free time.
	c.JSON(200, gin.H{"product": "guest-internet", "state": "active", "expires_at": g.ExpiresAt,
		"allocated_bytes": g.TotalBytes - g.UsedBytes, "message": "paid connection recovery verified"})
}

// Resume only verifies the existing current binding. It does not log the guest
// into RouterOS, transfer ownership, renew a cookie or reset any quota.
func (h *GuestVoucherHandler) Resume(c *gin.Context) {
	req, ok := h.paidGuestRequest(c, h.Resumer != nil)
	if !ok {
		return
	}
	g, err := h.Resumer.Resume(c.Request.Context(), req)
	if err != nil {
		c.JSON(403, gin.H{"error": "paid binding could not be confirmed", "code": "guest_resume_unavailable"})
		return
	}
	if !time.Now().Before(g.ExpiresAt) || g.RemainingBytes <= 0 || g.RemainingBytes > 1048576000000 {
		c.JSON(503, gin.H{"error": "paid binding verification incomplete"})
		return
	}
	c.JSON(200, gin.H{"product": "guest-internet", "state": "binding_verified", "expires_at": g.ExpiresAt,
		"remaining_bytes": g.RemainingBytes, "message": "existing paid binding verified; hotspot login still required"})
}

func (h *GuestVoucherHandler) paidGuestRequest(c *gin.Context, available bool) (guestaccess.RecoveryRequest, bool) {
	c.Header("Cache-Control", "no-store")
	var b struct {
		ClientMAC string `json:"client_mac"`
		Consent   bool   `json:"consent"`
	}
	if c.ShouldBindJSON(&b) != nil {
		c.JSON(400, gin.H{"error": "invalid recovery request"})
		return guestaccess.RecoveryRequest{}, false
	}
	deviceID := c.Param("deviceId")
	mac, err := net.ParseMAC(b.ClientMAC)
	if !guestVoucherUUID.MatchString(deviceID) || err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		c.JSON(400, gin.H{"error": "valid gateway and client MAC required"})
		return guestaccess.RecoveryRequest{}, false
	}
	voucherID, secret, ok := readGuestRecoveryCookie(c.Request)
	if !ok {
		c.JSON(403, gin.H{"error": "paid access recovery unavailable", "code": "guest_recovery_unavailable"})
		return guestaccess.RecoveryRequest{}, false
	}
	if h.ResolveDevice == nil || h.PortalConfig == nil || !available {
		c.JSON(503, gin.H{"error": "paid access recovery unavailable"})
		return guestaccess.RecoveryRequest{}, false
	}
	dev, err := h.ResolveDevice(deviceID)
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return guestaccess.RecoveryRequest{}, false
	}
	if dev == nil || dev.DeviceID != deviceID || dev.BusinessID == "" || dev.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway unavailable"})
		return guestaccess.RecoveryRequest{}, false
	}
	pc, err := h.PortalConfig(c.Request.Context(), dev.BusinessID)
	if err != nil {
		c.JSON(503, gin.H{"error": "portal configuration unavailable"})
		return guestaccess.RecoveryRequest{}, false
	}
	if pc == nil || !pc.VoucherLoginEnabled {
		c.JSON(403, gin.H{"error": "voucher login is disabled"})
		return guestaccess.RecoveryRequest{}, false
	}
	if pc.ConsentRequired && !b.Consent {
		c.JSON(400, gin.H{"error": "accept the hotspot terms before reconnecting"})
		return guestaccess.RecoveryRequest{}, false
	}
	newMAC := strings.ToUpper(mac.String())
	return guestaccess.RecoveryRequest{
		BusinessID: dev.BusinessID, DeviceID: dev.DeviceID, VoucherID: voucherID, Secret: secret, NewMAC: newMAC,
	}, true
}
