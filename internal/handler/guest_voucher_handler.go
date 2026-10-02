package handler

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

var guestVoucherUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type GuestVoucherRedeemer interface {
	Redeem(context.Context, guestaccess.ProvisionRequest) (model.GuestVoucherGrant, error)
}

type GuestAccessRecoverer interface {
	Restore(context.Context, guestaccess.RecoveryRequest) (guestaccess.RecoveryClaim, error)
}

type GuestAccessResumer interface {
	Resume(context.Context, guestaccess.RecoveryRequest) (guestaccess.ResumeResult, error)
}

type GuestVoucherHandler struct {
	DB            *database.DB
	Vouchers      *VoucherHandler
	ResolveDevice func(string) (*model.Device, error)
	PortalConfig  func(context.Context, string) (*model.PortalConfig, error)
	Redeemer      GuestVoucherRedeemer
	Recoverer     GuestAccessRecoverer
	Resumer       GuestAccessResumer
}

type guestVoucherBatch struct {
	DeviceID string `json:"device_id"`
	Label    string `json:"plan_label"`
	PriceMMK int    `json:"price_mmk"`
	Minutes  int    `json:"duration_minutes"`
	DataMB   int    `json:"data_mb"`
	Count    int    `json:"count"`
}

func (b guestVoucherBatch) valid() bool {
	return guestVoucherUUID.MatchString(b.DeviceID) && strings.TrimSpace(b.Label) != "" && len(b.Label) <= 100 &&
		b.PriceMMK >= 0 && b.PriceMMK <= 1000000000 && b.Minutes >= 1 && b.Minutes <= 525600 &&
		b.DataMB >= 1 && b.DataMB <= 1048576 && b.Count >= 1 && b.Count <= voucherMaxBatch
}

// Issue is a separate explicit product. Validity begins at first reservation,
// never printing time. Both the code and immutable guest allowance commit once.
func (h *GuestVoucherHandler) Issue(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var b guestVoucherBatch
	if c.ShouldBindJSON(&b) != nil || !b.valid() {
		c.JSON(400, gin.H{"error": "valid gateway, label, time, data and batch count required"})
		return
	}
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if h.DB == nil || h.Vouchers == nil {
		c.JSON(503, gin.H{"error": "guest voucher issuance unavailable"})
		return
	}
	var vouchers []*model.Voucher
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		dev, e := database.NewDeviceRepository(h.DB).GetDeviceByIDTXN(tx, b.DeviceID)
		if e != nil {
			return e
		}
		if dev == nil || dev.BusinessID != biz || dev.DeploymentLayer != "MAIN_GATEWAY" {
			return guestaccess.ErrProvisionDenied
		}
		for i := 0; i < b.Count; i++ {
			v, e := h.Vouchers.createOneVoucher(tx, biz, "guest-internet", strings.TrimSpace(b.Label), b.PriceMMK, &b.Minutes, nil, &b.DeviceID, "unused", nil, time.Now().UTC())
			if e != nil {
				return e
			}
			if e = (database.GuestVoucherRepository{}).IssueTXN(tx, biz, v.ID, b.DeviceID, b.Minutes, b.DataMB); e != nil {
				return e
			}
			vouchers = append(vouchers, v)
		}
		return nil
	})
	if err != nil {
		c.JSON(409, gin.H{"error": "guest vouchers could not be issued for this gateway"})
		return
	}
	c.JSON(201, gin.H{"vouchers": vouchers, "product": "guest-internet", "data_mb": b.DataMB, "validity_starts": "first_use"})
}

// Redeem is device-anchored and never calls the business-pass redemption core.
// The quota service returns success only after router and ledger readback.
func (h *GuestVoucherHandler) Redeem(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var b struct {
		Code      string `json:"code"`
		ClientMAC string `json:"client_mac"`
		Consent   bool   `json:"consent"`
	}
	if c.ShouldBindJSON(&b) != nil {
		c.JSON(400, gin.H{"error": "invalid guest voucher request"})
		return
	}
	mac, e := net.ParseMAC(b.ClientMAC)
	code := strings.ToUpper(strings.TrimSpace(b.Code))
	id := c.Param("deviceId")
	if !guestVoucherUUID.MatchString(id) || code == "" || len(code) > 128 || e != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		c.JSON(400, gin.H{"error": "valid gateway, voucher code and client MAC required"})
		return
	}
	if h.ResolveDevice == nil || h.PortalConfig == nil || h.Redeemer == nil {
		c.JSON(503, gin.H{"error": "guest voucher access unavailable"})
		return
	}
	dev, e := h.ResolveDevice(id)
	if e != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if dev == nil || dev.DeviceID != id || dev.BusinessID == "" || dev.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway unavailable"})
		return
	}
	pc, e := h.PortalConfig(c.Request.Context(), dev.BusinessID)
	if e != nil {
		c.JSON(503, gin.H{"error": "portal configuration unavailable"})
		return
	}
	if pc == nil || !pc.VoucherLoginEnabled {
		c.JSON(403, gin.H{"error": "voucher login is disabled"})
		return
	}
	if pc.ConsentRequired && !b.Consent {
		c.JSON(400, gin.H{"error": "accept the hotspot terms before connecting"})
		return
	}
	g, e := h.Redeemer.Redeem(c.Request.Context(), guestaccess.ProvisionRequest{BusinessID: dev.BusinessID, DeviceID: dev.DeviceID, Code: code, ClientMAC: strings.ToUpper(mac.String())})
	if e != nil {
		status := http.StatusForbidden
		if errors.Is(e, database.ErrGuestVoucherBusy) {
			status = 409
		}
		if errors.Is(e, guestaccess.ErrProvisionNeedsRepair) {
			status = 503
		}
		c.JSON(status, gin.H{"error": "guest access could not be confirmed", "code": "guest_voucher_unavailable"})
		return
	}
	if g.BusinessID != dev.BusinessID || g.DeviceID != dev.DeviceID || g.ClientMAC != strings.ToUpper(mac.String()) || g.State != "active" || g.ExpiresAt == nil || !time.Now().Before(*g.ExpiresAt) || g.DataMB < 1 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guest access verification incomplete"})
		return
	}
	if !setGuestRecoveryCookie(c, g, time.Now()) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guest recovery verification incomplete"})
		return
	}
	c.JSON(200, gin.H{"product": "guest-internet", "state": g.State, "expires_at": g.ExpiresAt, "data_mb": g.DataMB, "message": "guest internet access verified"})
}
