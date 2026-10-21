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
	"github.com/lib/pq"
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

// VoucherLoginSpec is one issued voucher installed on its gateway.
type VoucherLoginSpec struct {
	VoucherID string
	Code      string
	Minutes   int
	DataMB    int
}

// VoucherLoginState is the gateway's own accounting for one issued voucher.
type VoucherLoginState struct {
	VoucherID     string
	Exists        bool
	Disabled      bool
	UptimeSeconds int64
	BytesUsed     int64
	ActiveMAC     string
}

// VoucherLoginProvisioner installs issued codes on the RouterBOARD itself so
// it validates them locally at login time (ground-operation design).
type VoucherLoginProvisioner interface {
	ProvisionVoucherLogins(ctx context.Context, businessID, deviceID string, logins []VoucherLoginSpec) error
}

// VoucherLoginSyncer reads each voucher's live accounting back from the
// RouterBOARD through the Magic Hub (router -> hub -> app synchronization).
type VoucherLoginSyncer interface {
	ReadVoucherLogins(ctx context.Context, businessID, deviceID string, refs []VoucherLoginSpec) ([]VoucherLoginState, error)
}

type GuestVoucherHandler struct {
	DB            *database.DB
	Vouchers      *VoucherHandler
	ResolveDevice func(string) (*model.Device, error)
	PortalConfig  func(context.Context, string, string) (*model.PortalConfig, error)
	Redeemer      GuestVoucherRedeemer
	Recoverer     GuestAccessRecoverer
	Resumer       GuestAccessResumer
	// Ground-operation voucher login: the RouterBOARD validates issued codes
	// locally and the app synchronizes its accounting back through the hub.
	Provisioner VoucherLoginProvisioner
	Syncer      VoucherLoginSyncer
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
	// Ground-operation issuance: install every issued code on the RouterBOARD
	// itself so a guest can validate it locally, without the hosted site or
	// WAN. A gateway that cannot take the users cannot sell working codes, so
	// the issued rows are withdrawn again on failure.
	if h.Provisioner != nil {
		specs := make([]VoucherLoginSpec, 0, len(vouchers))
		for _, v := range vouchers {
			specs = append(specs, VoucherLoginSpec{VoucherID: v.ID, Code: v.Code, Minutes: b.Minutes, DataMB: b.DataMB})
		}
		if err := h.Provisioner.ProvisionVoucherLogins(c.Request.Context(), biz, b.DeviceID, specs); err != nil {
			ids := make([]string, 0, len(vouchers))
			for _, v := range vouchers {
				ids = append(ids, v.ID)
			}
			if rollback := h.withdrawIssuedVouchers(biz, ids); rollback != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "voucher codes could not be installed on the gateway and the issued records could not be withdrawn: " + rollback.Error()})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "voucher codes could not be installed on the gateway: " + err.Error()})
			return
		}
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
	pc, e := h.PortalConfig(c.Request.Context(), dev.BusinessID, dev.DeviceID)
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

// withdrawIssuedVouchers removes vouchers (and their grant rows) whose codes
// could not be installed on the gateway. Grants go first: they reference the
// voucher rows. A failure here is reported verbatim by the caller.
func (h *GuestVoucherHandler) withdrawIssuedVouchers(businessID string, voucherIDs []string) error {
	if h.DB == nil || len(voucherIDs) == 0 {
		return nil
	}
	if _, err := h.DB.Exec(`DELETE FROM guest_voucher_grants WHERE business_id=$1 AND voucher_id = ANY($2::uuid[])`, businessID, pq.Array(voucherIDs)); err != nil {
		return err
	}
	if _, err := h.DB.Exec(`DELETE FROM vouchers WHERE business_id=$1 AND id = ANY($2::uuid[])`, businessID, pq.Array(voucherIDs)); err != nil {
		return err
	}
	return nil
}

// SyncVoucherLogins is the router -> hub -> app synchronization: it reads each
// issued voucher's live accounting from the RouterBOARD and applies it to the
// app's records. Usage observed on the router marks the voucher used, the
// grant's provisioning claim binds the observed client, activation anchors
// the immutable activation timestamp, and router-side expiry propagates.
func (h *GuestVoucherHandler) SyncVoucherLogins(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var b struct {
		DeviceID string `json:"device_id"`
	}
	if c.ShouldBindJSON(&b) != nil || !guestVoucherUUID.MatchString(b.DeviceID) {
		c.JSON(400, gin.H{"error": "valid gateway required"})
		return
	}
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if h.DB == nil || h.Vouchers == nil || h.Syncer == nil || h.ResolveDevice == nil {
		c.JSON(503, gin.H{"error": "voucher login synchronization unavailable"})
		return
	}
	dev, e := h.ResolveDevice(b.DeviceID)
	if e != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if dev == nil || dev.DeviceID != b.DeviceID || dev.BusinessID != biz || dev.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway unavailable"})
		return
	}
	list, e := h.Vouchers.voucherRepo.ListVouchersByBusiness(biz)
	if e != nil {
		c.JSON(503, gin.H{"error": "voucher records unavailable"})
		return
	}
	now := time.Now().UTC()
	specs := []VoucherLoginSpec{}
	byVoucher := map[string]*model.Voucher{}
	for _, v := range list {
		if v.DeviceID == nil || *v.DeviceID != b.DeviceID || v.DurationMinutes == nil {
			continue
		}
		specs = append(specs, VoucherLoginSpec{VoucherID: v.ID, Code: v.Code, Minutes: *v.DurationMinutes})
		byVoucher[v.ID] = v
	}
	if len(specs) == 0 {
		c.JSON(200, gin.H{"synced": []any{}, "router": "no issued vouchers on this gateway"})
		return
	}
	states, e := h.Syncer.ReadVoucherLogins(c.Request.Context(), biz, b.DeviceID, specs)
	if e != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gateway accounting could not be read: " + e.Error()})
		return
	}
	results := make([]gin.H, 0, len(states))
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		for _, s := range states {
			v := byVoucher[s.VoucherID]
			if v == nil {
				continue
			}
			applied := []string{}
			usage := s.UptimeSeconds > 0 || s.BytesUsed > 0
			if usage && v.Status != "used" {
				if _, e := h.Vouchers.voucherRepo.ClaimVoucherByRouterTXN(tx, biz, v.ID, now); e != nil {
					return e
				}
				applied = append(applied, "voucher:used")
			}
			grants := database.GuestVoucherRepository{}
			grant, e := grants.GetGrantTXN(tx, biz, v.ID)
			if e != nil {
				return e
			}
			if grant != nil {
				limitSeconds := int64(*v.DurationMinutes) * 60
				switch {
				case usage && s.ActiveMAC != "" && grant.State == "unused":
					if e := grants.SyncClaimTXN(tx, biz, v.ID, s.ActiveMAC, now, *v.DurationMinutes); e != nil {
						return e
					}
					applied = append(applied, "grant:claimed")
					// Activation anchors the immutable timestamp only while the
					// allowance is still live; an exhausted or disabled voucher
					// records its expiry without inventing an activation.
					if !s.Disabled && s.UptimeSeconds < limitSeconds {
						if e := grants.SyncActivateTXN(tx, biz, v.ID, now); e != nil {
							return e
						}
						applied = append(applied, "grant:activated")
					}
				case grant.State == "active" && (s.Disabled || s.UptimeSeconds >= limitSeconds):
					if e := grants.SyncExpireTXN(tx, biz, v.ID); e != nil {
						return e
					}
					applied = append(applied, "grant:expired")
				}
			}
			results = append(results, gin.H{"voucher_id": v.ID, "code": v.Code, "exists": s.Exists, "disabled": s.Disabled,
				"uptime_seconds": s.UptimeSeconds, "bytes_used": s.BytesUsed, "active_mac": s.ActiveMAC, "applied": applied})
		}
		return nil
	})
	if err != nil {
		c.JSON(409, gin.H{"error": "voucher login synchronization could not be applied"})
		return
	}
	c.JSON(200, gin.H{"synced": results})
}
