package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

// guestActionsEnabled gates Kick, Ban MAC, unban and removing a voucher login.
// It stays off until docs/GUEST-ACTIONS-TRIAL-20261105.md has passed on a router.
func guestActionsEnabled() bool {
	return os.Getenv("MM_NETWORK_CONTROLS_ENABLED") == "true" && os.Getenv("MM_GUEST_ACTIONS_ENABLED") == "true"
}

const guestActionTimeout = 45 * time.Second

// GuestRouterActions are the owner's explicit changes to one gateway's guests.
type GuestRouterActions interface {
	DisconnectGuest(ctx context.Context, businessID, deviceID string, s routeros.GuestSession) error
	BlockGuestMAC(ctx context.Context, businessID, deviceID, mac string) error
	UnblockGuestMAC(ctx context.Context, businessID, deviceID, mac string) error
}

// EnableGuestActions supplies the router connector. The switch still has to be on.
func (h *RouterHandler) EnableGuestActions(actions GuestRouterActions) { h.guestActions = actions }

func (h *RouterHandler) guestActionsReady() bool {
	return h.guestActions != nil && guestActionsEnabled()
}

func guestActionsOff(c *gin.Context) {
	// Report unavailability as an error, never as a successful action.
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Guest actions are unavailable until the router action has passed acceptance", "code": "guest_actions_unavailable", "verified": false})
}

// guestGateway checks tenant ownership. It writes the response when it returns false.
func (h *RouterHandler) guestGateway(c *gin.Context, businessID, deviceID string) bool {
	var device *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		device = d
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return false
	}
	if device == nil || device.BusinessID != businessID {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return false
	}
	if h.guestActionsReady() && device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(http.StatusConflict, gin.H{"error": "guest actions need a main gateway", "verified": false})
		return false
	}
	return true
}

func (h *RouterHandler) finishGuestAction(c *gin.Context, businessID, deviceID, action, reason string, err error) {
	if err != nil {
		status, code := http.StatusBadGateway, "router_action_failed"
		switch {
		case errors.Is(err, routeros.ErrGuestSessionChanged):
			status, code = http.StatusConflict, "session_changed"
		case errors.Is(err, routeros.ErrGuestBindingForeign):
			status, code = http.StatusConflict, "binding_foreign"
		}
		c.JSON(status, gin.H{"error": err.Error(), "code": code, "verified": false})
		return
	}
	auditErr := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		return h.deviceRepo.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
	})
	c.JSON(http.StatusOK, gin.H{"verified": true, "audit_recorded": auditErr == nil})
}

type guestMACRequest struct {
	MAC     string `json:"mac" binding:"required"`
	Confirm bool   `json:"confirm" binding:"required"`
}

// BlockGuest handles Ban MAC; UnblockGuest removes a ban this app added.
func (h *RouterHandler) BlockGuest(c *gin.Context)   { h.guestMACAction(c, true) }
func (h *RouterHandler) UnblockGuest(c *gin.Context) { h.guestMACAction(c, false) }

func (h *RouterHandler) guestMACAction(c *gin.Context, block bool) {
	c.Header("Cache-Control", "no-store")
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	if !h.guestGateway(c, businessID, deviceID) {
		return
	}
	var req guestMACRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mac and confirmation required"})
		return
	}
	if !h.guestActionsReady() {
		guestActionsOff(c)
		return
	}
	mac := strings.ToUpper(strings.TrimSpace(req.MAC))
	ctx, cancel := context.WithTimeout(c.Request.Context(), guestActionTimeout)
	defer cancel()
	if block {
		h.finishGuestAction(c, businessID, deviceID, "router.guest.mac_blocked", "Owner blocked guest MAC "+mac, h.guestActions.BlockGuestMAC(ctx, businessID, deviceID, mac))
		return
	}
	h.finishGuestAction(c, businessID, deviceID, "router.guest.mac_unblocked", "Owner removed the block on guest MAC "+mac, h.guestActions.UnblockGuestMAC(ctx, businessID, deviceID, mac))
}

// RetireVoucherLogin handles Delete for a code on the router: the login is removed,
// and the voucher stays on record as revoked so batch recovery and sales history hold.
func (h *GuestVoucherHandler) RetireVoucherLogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, id := auth.GetBusinessID(c), strings.ToLower(c.Param("voucherId"))
	if biz == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if !guestVoucherUUID.MatchString(id) || c.ShouldBindJSON(&body) != nil || !body.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voucher and confirmation required"})
		return
	}
	if h.Retirer == nil || h.DB == nil || h.Vouchers == nil || !guestActionsEnabled() {
		guestActionsOff(c)
		return
	}
	var device, code string
	var refusal string
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		v, err := h.Vouchers.voucherRepo.GetVoucherByIDTXN(tx, id)
		if err != nil {
			return err
		}
		if v == nil || v.BusinessID != biz || v.DeviceID == nil {
			refusal = "not_found"
			return nil
		}
		if v.Status != "unused" {
			refusal = "only an unused code can be deleted"
			return nil
		}
		// A batch still installing could put this login back, so it must have finished.
		var batch, state string
		if err := tx.QueryRow(`SELECT batch_id,install_state FROM guest_voucher_issue_items WHERE business_id=$1 AND voucher_id=$2`, biz, id).Scan(&batch, &state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				refusal = "this code is not part of a batch"
				return nil
			}
			return err
		}
		var unfinished int
		if err := tx.QueryRow(`SELECT count(*) FROM guest_voucher_issue_items WHERE business_id=$1 AND batch_id=$2 AND install_state<>'verified'`, biz, batch).Scan(&unfinished); err != nil {
			return err
		}
		if unfinished > 0 {
			refusal = "finish this batch before deleting its codes"
			return nil
		}
		device, code = *v.DeviceID, v.Code
		return nil
	})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "voucher records unavailable"})
		return
	}
	switch refusal {
	case "":
	case "not_found":
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	default:
		c.JSON(http.StatusConflict, gin.H{"error": refusal, "verified": false})
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), guestActionTimeout)
	defer cancel()
	if err := h.Retirer.RemoveVoucherLogin(ctx, biz, device, VoucherLoginSpec{VoucherID: id, Code: code}); err != nil {
		if errors.Is(err, routeros.ErrVoucherLoginInUse) {
			c.JSON(http.StatusConflict, gin.H{"error": "this code has been used on the router", "code": "voucher_in_use", "verified": false})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "the router did not confirm the code was removed", "verified": false})
		return
	}
	// If this fails the login is already gone, and the same request can be retried.
	var updated bool
	err = runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		v, err := h.Vouchers.voucherRepo.UpdateVoucherStatusByBusinessTXN(tx, biz, id, "revoked", nil)
		if err != nil {
			return err
		}
		updated = v != nil
		if updated {
			return database.NewDeviceRepository(h.DB).RecordChangeTXN(tx, biz, device, auth.GetUserRole(c), "router.guest.voucher_removed", "Owner deleted unused voucher "+id)
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "the code was removed from the router; retry to finish the record", "verified": true})
		return
	}
	if !updated {
		c.JSON(http.StatusConflict, gin.H{"error": "the voucher changed while it was being deleted", "verified": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified": true, "status": "revoked"})
}
