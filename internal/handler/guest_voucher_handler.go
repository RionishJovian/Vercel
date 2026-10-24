package handler

import (
	"context"
	"errors"
	"fmt"
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

// VoucherLoginSpec is one issued voucher installed on its gateway.
type VoucherLoginSpec struct {
	VoucherID string
	Code      string
	Minutes   int
	DataMB    int
}

// VoucherLoginState is the gateway's own accounting for one issued voucher.
// FirstUseEpoch/FirstMAC are the router-persisted first-use anchor (epoch
// seconds and client identity) written once by the shared expiry guard.
type VoucherLoginState struct {
	VoucherID     string
	Exists        bool
	Disabled      bool
	UptimeSeconds int64
	BytesUsed     int64
	ActiveMAC     string
	FirstUseEpoch int64
	FirstMAC      string
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
	ResellerID string `json:"reseller_id,omitempty"`
	BatchID    string `json:"batch_id"`
	DeviceID   string `json:"device_id"`
	Label      string `json:"plan_label"`
	PriceMMK   int    `json:"price_mmk"`
	Minutes    int    `json:"duration_minutes"`
	DataMB     int    `json:"data_mb"`
	Count      int    `json:"count"`
}

func (b guestVoucherBatch) valid() bool {
	return (b.ResellerID == "" || guestVoucherUUID.MatchString(b.ResellerID)) && guestVoucherUUID.MatchString(b.DeviceID) && strings.TrimSpace(b.Label) != "" && len(b.Label) <= 100 &&
		b.PriceMMK >= 0 && b.PriceMMK <= 1000000000 && b.Minutes >= 1 && b.Minutes <= 525600 &&
		b.DataMB >= 1 && b.DataMB <= 1048576 && b.Count >= 1 && b.Count <= GuestVoucherIssueBatchLimit
}

// Issue saves a stable, recoverable installation batch before router I/O.
// Guest validity and revenue are unaffected by issuance or printing.
func (h *GuestVoucherHandler) Issue(c *gin.Context) {
	h.issueGuestVoucherBatch(c)
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
	results, e := h.syncGatewayVouchers(c.Request.Context(), biz, b.DeviceID, time.Now().UTC())
	if e != nil {
		switch {
		case errors.Is(e, errVoucherLoginUnavailable):
			c.JSON(503, gin.H{"error": "voucher login synchronization unavailable"})
		case errors.Is(e, errVoucherRecordsUnavailable):
			c.JSON(503, gin.H{"error": "voucher records unavailable"})
		case errors.Is(e, errGatewayAccountingRead):
			c.JSON(http.StatusBadGateway, gin.H{"error": "gateway accounting could not be read"})
		default:
			c.JSON(409, gin.H{"error": "voucher login synchronization could not be applied"})
		}
		return
	}
	if len(results) == 0 {
		c.JSON(200, gin.H{"synced": []any{}, "router": "no issued vouchers on this gateway"})
		return
	}
	c.JSON(200, gin.H{"synced": results})
}

// Sync failure classes shared by the admin endpoint and the periodic worker.
var (
	errVoucherLoginUnavailable   = errors.New("voucher login synchronization unavailable")
	errVoucherRecordsUnavailable = errors.New("voucher records unavailable")
	errGatewayAccountingRead     = errors.New("gateway accounting read failed")
)

// syncGatewayVouchers is the router -> hub -> app synchronization core shared by
// the admin endpoint and the periodic system worker. It reads the RouterBOARD's
// live accounting for every voucher issued on this gateway and applies it to the
// app's records: router-observed usage marks the voucher used, binds the
// observed client and starts validity, anchors the immutable activation
// timestamp while the allowance is still live, and propagates router-side
// expiry without touching that timestamp. All transitions run inside the same
// RLS tenant transaction and CHECK/trigger contract as the hosted flow.
func (h *GuestVoucherHandler) syncGatewayVouchers(ctx context.Context, biz, deviceID string, now time.Time) ([]gin.H, error) {
	if h.DB == nil || h.Vouchers == nil || h.Syncer == nil {
		return nil, errVoucherLoginUnavailable
	}
	specs, e := h.gatewayVoucherLoginSpecs(ctx, biz, deviceID)
	if e != nil {
		return nil, fmt.Errorf("%w: %v", errVoucherRecordsUnavailable, e)
	}
	if len(specs) == 0 {
		return []gin.H{}, nil
	}
	states, e := h.readGatewayVoucherAccounting(ctx, biz, deviceID, specs)
	if e != nil {
		return nil, fmt.Errorf("%w: %v", errGatewayAccountingRead, e)
	}
	results := make([]gin.H, 0, len(states))
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	grants := database.GuestVoucherRepository{}
	err = database.WithTenant(tx, biz, func() error {
		for _, observation := range states {
			applied := []string{}
			firstUseStatus := ""
			// Missing accounts are unavailable evidence, not proof of expiry.
			if observation.Exists {
				// Match hosted redemption's lock order: voucher, then grant.
				var voucherStatus string
				if e := tx.QueryRowContext(ctx, `SELECT status FROM vouchers WHERE business_id=$1 AND id=$2 FOR UPDATE`, biz, observation.VoucherID).Scan(&voucherStatus); e != nil {
					return e
				}
				grant, e := grants.GetGrantTXN(tx, biz, observation.VoucherID)
				if e != nil {
					return e
				}
				if grant == nil || grant.DeviceID != deviceID {
					return database.ErrGuestVoucherUnavailable
				}
				usage := observation.UptimeSeconds > 0 || observation.BytesUsed > 0 || observation.FirstUseEpoch > 0
				eligible := voucherStatus == "unused" || voucherStatus == "active" || voucherStatus == "used"
				// The router-persisted anchor is the immutable first-use
				// fact: it survives short sessions and Hub/app outages, and
				// validity is counted from it, not from this sync. Counters
				// alone can mark observed usage, but cannot invent first use.
				anchor := now
				if observation.FirstUseEpoch > 0 {
					anchor = time.Unix(observation.FirstUseEpoch, 0).UTC()
				}
				clientMAC := observation.FirstMAC

				routerEvidence := observation.FirstUseEpoch > 0 && observation.FirstMAC != ""
				if usage && eligible && grant.State == "unused" && !routerEvidence {
					firstUseStatus = "unverified"
				}
				if usage && eligible && voucherStatus != "used" {
					claimed, e := h.Vouchers.voucherRepo.ClaimVoucherByRouterTXN(tx, biz, observation.VoucherID, anchor)
					if e != nil {
						return e
					}
					if !claimed {
						return database.ErrGuestVoucherUnavailable
					}
					applied = append(applied, "voucher:used")
				}
				exhausted := observation.Disabled || observation.UptimeSeconds >= int64(grant.Minutes)*60 || observation.BytesUsed >= int64(grant.DataMB)*1000000
				switch {
				case eligible && usage && routerEvidence && grant.State == "unused":
					// Router evidence records the historical first use and
					// its client identity. A live session alone cannot tell
					// us the original login time after an app/Hub outage.
					// Never fabricate a MAC or timestamp without either.
					if e := grants.SyncClaimTXN(tx, biz, observation.VoucherID, clientMAC, anchor, grant.Minutes); e != nil {
						return e
					}
					if e := grants.SyncActivateTXN(tx, biz, observation.VoucherID, anchor); e != nil {
						return e
					}
					applied = append(applied, "grant:claimed", "grant:activated")
				case grant.State == "active" && (exhausted || !eligible || grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt)):
					if e := grants.SyncExpireTXN(tx, biz, observation.VoucherID); e != nil {
						return e
					}
					applied = append(applied, "grant:expired")
				}
			}
			results = append(results, gin.H{"voucher_id": observation.VoucherID, "exists": observation.Exists, "disabled": observation.Disabled,
				"uptime_seconds": observation.UptimeSeconds, "bytes_used": observation.BytesUsed, "active_mac": observation.ActiveMAC, "first_use_status": firstUseStatus, "applied": applied})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

// SyncBusinessVouchers synchronizes the live voucher-login accounting of every
// gateway of this business once. The periodic system worker calls it per tenant
// (router -> hub -> app synchronization for ground-operation vouchers).
func (h *GuestVoucherHandler) SyncBusinessVouchers(ctx context.Context, biz string) (int, error) {
	if h.DB == nil || h.Vouchers == nil || h.Syncer == nil {
		return 0, errVoucherLoginUnavailable
	}
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var gateways []string
	if err := database.WithTenant(tx, biz, func() error {
		rows, err := tx.QueryContext(ctx, `SELECT device_id::text FROM devices WHERE deployment_layer='MAIN_GATEWAY'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			gateways = append(gateways, id)
		}
		return rows.Err()
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	applied := 0
	var failures []error
	for _, id := range gateways {
		if err := ctx.Err(); err != nil {
			return applied, err
		}
		results, err := h.syncGatewayVouchers(ctx, biz, id, time.Now().UTC())
		if err != nil {
			failures = append(failures, fmt.Errorf("gateway %s: %w", id, err))
			continue
		}
		applied += len(results)
	}
	return applied, errors.Join(failures...)
}
