package handler

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/voucher"
)

const (
	voucherCodeAttempts = 5 // unique-code retries before giving up
	voucherMaxBatch     = 1000
)

type VoucherHandler struct {
	db          *database.DB
	voucherRepo *database.VoucherRepository
	quotaRepo   *database.BusinessMeteredQuotaRepository
}

// NewVoucherHandler wires the voucher repo, the metered-quota repo used to apply
// a voucher's entitlement, and the DB handle needed to run redemption as one
// transaction (claim + entitlement atomically).
func NewVoucherHandler(db *database.DB, voucherRepo *database.VoucherRepository, quotaRepo *database.BusinessMeteredQuotaRepository) *VoucherHandler {
	return &VoucherHandler{db: db, voucherRepo: voucherRepo, quotaRepo: quotaRepo}
}

// Sentinel errors mapped to HTTP status codes by the converted handlers, so the
// transaction wrapper can distinguish them from real failures.
var (
	errVoucherNotFound     = errors.New("voucher not found")
	errVoucherNotDeletable = errors.New("redeemed vouchers cannot be deleted")
)

// httpError lets a value returned from inside a tenant transaction carry an exact
// HTTP status + message, so converting a handler to the tx path does not change
// its API responses.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// withTenantTx runs fn inside a transaction with the tenant RLS context set via
// database.WithTenant (SET LOCAL ROLE authenticated + app.current_business_id),
// committing on success and rolling back on any error. Every money-path query
// runs through here so it is reliably tenant-scoped.
func (h *VoucherHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// CreateVoucher generates new voucher codes for a business.
func (h *VoucherHandler) CreateVoucher(c *gin.Context) {
	var req struct {
		DeviceID        *string `json:"device_id,omitempty"`
		PlanKey         string  `json:"plan_key"`
		PlanLabel       string  `json:"plan_label"`
		PriceMMK        int     `json:"price_mmk"`
		DurationMinutes *int    `json:"duration_minutes,omitempty"`
		DeviceMAC       *string `json:"device_mac,omitempty"`
		Status          string  `json:"status,omitempty"`
		Count           int     `json:"count"` // number of vouchers to generate
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if req.Status == "" {
		req.Status = voucher.StatusActive
	}
	if req.Status != "active" && req.Status != "unused" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new vouchers must be active or unused"})
		return
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > voucherMaxBatch {
		count = voucherMaxBatch
	}

	now := time.Now().UTC()
	// Expiry is now + duration (or NULL when no duration) — NOT creation time.
	expiresAt := voucher.ComputeExpiry(now, req.DurationMinutes)

	vouchers := make([]*model.Voucher, 0, count)
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		for i := 0; i < count; i++ {
			created, err := h.createOneVoucher(tx, businessID, req.PlanKey, req.PlanLabel, req.PriceMMK,
				req.DurationMinutes, req.DeviceMAC, req.DeviceID, req.Status, expiresAt, now)
			if err != nil {
				return err
			}
			vouchers = append(vouchers, created)
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create voucher: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"vouchers": vouchers})
}

// createOneVoucher generates a unique code and inserts the voucher, retrying on
// a unique-constraint collision (codes are business-scoped-unique).
func (h *VoucherHandler) createOneVoucher(tx *sql.Tx, businessID, planKey, planLabel string, priceMMK int,
	durationMinutes *int, deviceMAC, deviceID *string, status string, expiresAt *time.Time, now time.Time,
) (*model.Voucher, error) {
	var lastErr error
	for attempt := 0; attempt < voucherCodeAttempts; attempt++ {
		code, err := voucher.GenerateCode(rand.Reader, voucher.DefaultCodeLength)
		if err != nil {
			return nil, err
		}
		v := &model.Voucher{
			BusinessID:      businessID,
			DeviceID:        deviceID,
			PlanKey:         planKey,
			PlanLabel:       planLabel,
			Code:            code,
			PriceMMK:        priceMMK,
			DurationMinutes: durationMinutes,
			DeviceMAC:       parseMAC(deviceMAC),
			Status:          status,
			// first_seen_at is stamped at redemption, not creation.
			FirstSeenAt: nil,
			ExpiresAt:   expiresAt,
		}
		created, err := h.voucherRepo.CreateVoucherTXN(tx, v)
		if err == nil {
			return created, nil
		}
		lastErr = err
		if !isUniqueViolation(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

// isUniqueViolation reports whether err is a postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// GetVoucherByCode retrieves a voucher for redemption.
func (h *VoucherHandler) GetVoucherByCode(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	code := c.Param("code")
	var found *model.Voucher
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		v, err := h.voucherRepo.GetVoucherByCodeTXN(tx, businessID, code)
		if err != nil {
			return err
		}
		found = v
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"voucher": found})
}

// ListVouchers lists all vouchers for the business.
func (h *VoucherHandler) ListVouchers(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var vouchers []*model.Voucher
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		vs, err := h.voucherRepo.ListVouchersByBusinessTXN(tx, businessID)
		if err != nil {
			return err
		}
		vouchers = vs
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"vouchers": vouchers})
}

// UpdateVoucherStatus updates voucher status (e.g., when used) and PERSISTS it.
func (h *VoucherHandler) UpdateVoucherStatus(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	var req struct {
		Status      string     `json:"status"`
		FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.FirstSeenAt != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "first use is recorded by redemption, not editable"})
		return
	}

	var updated *model.Voucher
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		// Ownership check inside the tenant scope: never allow a cross-tenant
		// edit by id.
		existing, err := h.voucherRepo.GetVoucherByIDTXN(tx, id)
		if err != nil {
			return err
		}
		if existing == nil || existing.BusinessID != businessID {
			return errVoucherNotFound
		}

		status := req.Status
		if status == "" {
			status = existing.Status
		}
		if !voucherStatusTransitionAllowed(existing.Status, status) {
			return &httpError{status: http.StatusConflict, msg: "voucher status cannot be reset or redeemed through an edit"}
		}

		upd, err := h.voucherRepo.UpdateVoucherStatusByBusinessTXN(tx, businessID, id, status, req.FirstSeenAt)
		if err != nil {
			return err
		}
		if upd == nil {
			return errVoucherNotFound
		}
		updated = upd
		return nil
	})
	if err != nil {
		var transitionErr *httpError
		if errors.As(err, &transitionErr) {
			respondErr(c, err)
			return
		}
		if errors.Is(err, errVoucherNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"voucher": updated})
}

// DeleteVoucher removes a voucher, but NEVER a redeemed (used) one.
func (h *VoucherHandler) DeleteVoucher(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		deleted, err := h.voucherRepo.DeleteVoucherIfUnusedTXN(tx, businessID, id)
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}
		// Distinguish "not ours / not found" from "redeemed, cannot delete".
		existing, gerr := h.voucherRepo.GetVoucherByIDTXN(tx, id)
		if gerr != nil {
			return gerr
		}
		if existing == nil || existing.BusinessID != businessID {
			return errVoucherNotFound
		}
		return errVoucherNotDeletable
	})
	if err != nil {
		switch {
		case errors.Is(err, errVoucherNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		case errors.Is(err, errVoucherNotDeletable):
			c.JSON(http.StatusConflict, gin.H{"error": "redeemed vouchers cannot be deleted"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

// RedeemVoucher handles voucher redemption: atomically claim, apply the plan's
// entitlement, and return the redeemed voucher — all inside ONE tenant-scoped
// transaction (RLS context + atomic claim + entitlement).
func (h *VoucherHandler) RedeemVoucher(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	code := c.Param("code")
	now := time.Now().UTC()

	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()

	var claimed *model.Voucher
	var applied gin.H
	err = database.WithTenant(tx, businessID, func() error {
		cl, ap, e := redeemVoucherTx(tx, h.voucherRepo, h.quotaRepo, businessID, code, now)
		if e != nil {
			return e
		}
		claimed, applied = cl, ap
		return nil
	})
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			c.JSON(he.status, gin.H{"error": he.msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize redemption"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"voucher": claimed,
		"applied": applied,
		"message": "voucher redeemed successfully",
	})
}

// redeemVoucherTx is the SHARED money-path redemption core, run inside a tx that
// is already RLS-bound to businessID. Both the owner endpoint (RedeemVoucher,
// business from JWT) and the anonymous device-anchored portal endpoint
// (business from the device) call it — payment logic lives in exactly one place.
func redeemVoucherTx(
	tx *sql.Tx,
	vrepo *database.VoucherRepository,
	qrepo *database.BusinessMeteredQuotaRepository,
	businessID, code string,
	now time.Time,
) (*model.Voucher, gin.H, error) {
	// Pre-check for a precise error (the claim below is the authoritative gate).
	existing, err := vrepo.GetVoucherByCodeTXN(tx, businessID, code)
	if err != nil {
		return nil, nil, err
	}
	if existing == nil {
		return nil, nil, &httpError{status: http.StatusNotFound, msg: "voucher not found"}
	}
	if reason := voucher.RedeemBlockReason(existing, now); reason != "" {
		return nil, nil, &httpError{status: http.StatusBadRequest, msg: reason}
	}

	// Fail closed on an unknown plan BEFORE claiming anything.
	effect, err := voucher.ResolvePlanEffect(existing.PlanKey)
	if err != nil {
		return nil, nil, &httpError{status: http.StatusBadRequest, msg: err.Error()}
	}
	if effect.ExtendWebfigPass && (existing.DurationMinutes == nil || *existing.DurationMinutes <= 0) {
		return nil, nil, &httpError{status: http.StatusBadRequest, msg: "voucher plan requires duration_minutes to grant a pass"}
	}

	// Atomic claim: only one concurrent redeemer can win.
	claimed, err := vrepo.ClaimVoucherTXN(tx, businessID, code, now)
	if err != nil {
		return nil, nil, err
	}
	if claimed == nil {
		return nil, nil, &httpError{status: http.StatusConflict, msg: "voucher is no longer redeemable"}
	}

	applied, err := applyPlanEffectTx(tx, qrepo, businessID, effect, *claimed.DurationMinutes, now)
	if err != nil {
		return nil, nil, &httpError{status: http.StatusInternalServerError, msg: "failed to apply voucher entitlement: " + err.Error()}
	}
	return claimed, applied, nil
}

// applyPlanEffectTx grants the plan entitlement inside the redemption tx and
// returns a summary. Reading the current pass and writing the new expiry use the
// same *sql.Tx so the whole redemption is atomic.
func applyPlanEffectTx(tx *sql.Tx, qrepo *database.BusinessMeteredQuotaRepository, businessID string, effect voucher.PlanEffect, durationMinutes int, now time.Time) (gin.H, error) {
	if qrepo == nil {
		return nil, errors.New("no quota repository configured")
	}
	if !effect.ExtendWebfigPass {
		return gin.H{"plan_key": effect.PlanKey, "effect": "none"}, nil
	}

	current, err := qrepo.GetByBusinessTXN(tx, businessID)
	if err != nil {
		return nil, err
	}
	var currentExpiry *time.Time
	if current != nil {
		currentExpiry = current.WebfigPassExpiresAt
	}
	newExpiry := voucher.ExtendExpiry(currentExpiry, now, durationMinutes)

	if current == nil {
		q := &model.BusinessMeteredQuota{BusinessID: businessID, WebfigPassExpiresAt: &newExpiry}
		if err := qrepo.UpsertTXN(tx, q); err != nil {
			return nil, err
		}
	} else if err := qrepo.ExtendWebfigPassTXN(tx, businessID, newExpiry); err != nil {
		return nil, err
	}

	return gin.H{
		"plan_key":                 effect.PlanKey,
		"effect":                   "extend_webfig_pass",
		"webfig_pass_expires_at":   newExpiry,
		"duration_minutes_applied": durationMinutes,
	}, nil
}
