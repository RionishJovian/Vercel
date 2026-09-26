package handler

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/billing"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
)

// errBankRefDuplicate signals a bank reference already used by a live request.
var errBankRefDuplicate = errors.New("bank reference already used for a live purchase")

// maxReceiptBytes caps an uploaded receipt photo.
const maxReceiptBytes = 5 << 20 // 5 MiB

// BillingHandler serves the payment-request surface:
//   - PUBLIC (anonymous, device-anchored): a guest on a device picks a plan and
//     uploads a payment receipt; the request is created pending.
//   - PROTECTED (operator-authenticated): the operator approves/rejects a
//     request. Approve grants the plan entitlement EXACTLY ONCE.
//
// Telegram is NOTIFICATION-ONLY in this slice (decision b): the operator is
// notified, and approves/rejects via the protected web endpoint. A real bot
// callback transport is a later, documented step.
type BillingHandler struct {
	db          *database.DB
	deviceRepo  *database.DeviceRepository
	configRepo  *database.PortalConfigRepository
	requestRepo *database.PurchaseRequestRepository
	voucherRepo *database.VoucherRepository
	quotaRepo   *database.BusinessMeteredQuotaRepository
	messenger   telegram.Messenger
	receiptRoot string
	plans       billing.PlanDurations
}

func NewBillingHandler(
	db *database.DB,
	deviceRepo *database.DeviceRepository,
	configRepo *database.PortalConfigRepository,
	requestRepo *database.PurchaseRequestRepository,
	voucherRepo *database.VoucherRepository,
	quotaRepo *database.BusinessMeteredQuotaRepository,
	messenger telegram.Messenger,
	plans billing.PlanDurations,
	receiptRoot string,
) *BillingHandler {
	return &BillingHandler{
		db:          db,
		deviceRepo:  deviceRepo,
		configRepo:  configRepo,
		requestRepo: requestRepo,
		voucherRepo: voucherRepo,
		quotaRepo:   quotaRepo,
		messenger:   messenger,
		plans:       plans,
		receiptRoot: receiptRoot,
	}
}

// notifyOperator is best-effort: a missing/!ok messenger simply means no
// notification (the approve path still works on the web endpoint).
func (h *BillingHandler) notifyOperator(businessID string, req *model.PurchaseRequest) {
	if h.messenger == nil {
		return
	}
	text := "MEGA MAGIC - purchase request awaiting approval\n" +
		"request: " + req.ID + "\n" +
		"business: " + businessID + "\n" +
		"plan: " + req.PlanKey + "\n" +
		"amount: " + strconv.Itoa(req.Amount) + "\n" +
		"status: " + req.Status + "\n" +
		"action: approve or reject on the billing endpoint"
	_ = h.messenger.SendMessage(businessID, text)
}

// PublicCreatePurchase: POST /api/v1/portal/:deviceId/purchase (multipart).
// The tenant anchor is the DEVICE (anonymous guest has no login).
func (h *BillingHandler) PublicCreatePurchase(c *gin.Context) {
	deviceID := c.Param("deviceId")
	dev, err := h.deviceRepo.GetDeviceByID(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if dev == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	planKey := c.PostForm("plan_key")
	if planKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan_key required"})
		return
	}
	// Fail closed on an unknown plan BEFORE any state change.
	if _, err := billing.ResolvePlan(planKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Plan-driven duration, REQUIRED (no magic default): resolved from config.
	duration, err := h.plans.DurationMinutes(planKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// bank_reference is REQUIRED: a real paid transfer must identify the purchase.
	bankRef := strings.TrimSpace(c.PostForm("bank_reference"))
	if bankRef == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bank_reference required"})
		return
	}

	amount := 0
	if v := c.PostForm("amount"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			amount = n
		}
	}

	var req *model.PurchaseRequest
	err = withDeviceTenant(h.db, dev.BusinessID, func(tx *sql.Tx) error {
		// Guarded uniqueness: one live (non-rejected) request per bank reference.
		existing, e := h.requestRepo.FindLiveByBankReferenceTXN(tx, dev.BusinessID, bankRef)
		if e != nil {
			return e
		}
		if existing != nil {
			return errBankRefDuplicate
		}
		r, e := h.requestRepo.CreateRequestTXN(tx, &model.PurchaseRequest{
			BusinessID:      dev.BusinessID,
			DeviceID:        &deviceID,
			PlanKey:         planKey,
			Amount:          amount,
			BankReference:   bankRef,
			DurationMinutes: duration,
			Status:          billing.StatusPending,
		})
		if e != nil {
			return e
		}
		req = r
		return nil
	})
	if err != nil {
		if errors.Is(err, errBankRefDuplicate) || isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "bank_reference already used for a live purchase"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create purchase request"})
		return
	}

	// Store the receipt photo tenant-scoped (0600), then bind its path.
	if fileHeader, ferr := c.FormFile("receipt"); ferr == nil && fileHeader != nil {
		if f, oerr := fileHeader.Open(); oerr == nil {
			data, rerr := io.ReadAll(io.LimitReader(f, maxReceiptBytes))
			_ = f.Close()
			if rerr == nil {
				if path, serr := billing.SaveReceipt(h.receiptRoot, dev.BusinessID, req.ID, data); serr == nil {
					_ = withDeviceTenant(h.db, dev.BusinessID, func(tx *sql.Tx) error {
						return h.requestRepo.SetReceiptPathTXN(tx, dev.BusinessID, req.ID, path)
					})
				}
			}
		}
	}

	h.notifyOperator(dev.BusinessID, req)

	c.JSON(http.StatusCreated, gin.H{
		"request_id": req.ID,
		"status":     req.Status,
		"message":    "purchase request submitted for operator approval",
	})
}

// ApprovePurchase: POST /api/v1/billing/approve/:requestId (operator).
func (h *BillingHandler) ApprovePurchase(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("requestId")

	// The operator-authorised decision still routes through the SHARED core so
	// HTTP and Telegram exercise the exact same grant path. Business comes from
	// the operator JWT; the core confirms the request belongs to it in-tenant.
	res, err := h.DecidePurchase(func() (*sql.Tx, error) {
		return beginTenantTx(c, h.db)
	}, businessID, id, true)
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"request": res.Request, "applied": res.Applied})
}

// RejectPurchase: POST /api/v1/billing/reject/:requestId (operator). Grants nothing.
func (h *BillingHandler) RejectPurchase(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("requestId")

	res, err := h.DecidePurchase(func() (*sql.Tx, error) {
		return beginTenantTx(c, h.db)
	}, businessID, id, false)
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"request": res.Request})
}

// ListRequests: GET /api/v1/billing/requests (operator).
func (h *BillingHandler) ListRequests(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var requests []*model.PurchaseRequest
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		rs, e := h.requestRepo.ListRequestsByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		requests = rs
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": requests, "count": len(requests)})
}

// withTenantTx is used by the protected (operator) endpoints.
func (h *BillingHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// DecideResult is the outcome of a purchase decision — reused by both the HTTP
// handlers and the Telegram bridge so there is exactly ONE money path.
type DecideResult struct {
	Request *model.PurchaseRequest
	Applied gin.H // non-nil only on approve (the entitlement granted)
}

// DecidePurchase is the shared decide-core — the single money path used by BOTH
// the HTTP operator handlers and the Telegram approve bridge. It takes an
// already-resolved businessID (HTTP: from the operator JWT; Telegram bridge:
// from an owner-scope read on a SEPARATE connection). ALL request reads happen
// INSIDE the tenant scope (WithTenant), so this is safe under RLS on both a
// pinned (HTTP) and a plain-owner (bridge) connection.
//
// It never trusts a caller-supplied business beyond confirming the request
// actually belongs to it inside the tenant tx.
func (h *BillingHandler) DecidePurchase(begin func() (*sql.Tx, error), businessID, requestID string, approve bool) (DecideResult, error) {
	var res DecideResult
	tx, err := begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Run the single-wind decision inside the tenant scope so request reads are
	// RLS-visible. The business is the authoritative owner of the request row.
	err = database.WithTenant(tx, businessID, func() error {
		r, e := h.requestRepo.GetRequestByIDTXN(tx, businessID, requestID)
		if e != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if r == nil {
			return &httpError{status: http.StatusNotFound, msg: "purchase request not found"}
		}
		if r.Status != billing.StatusPending {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}

		if !approve {
			claimed, e := h.requestRepo.RejectRequestTXN(tx, businessID, requestID, now)
			if e != nil {
				return e
			}
			res.Request = claimed
			return nil
		}

		// Fail closed on unknown plan / no duration BEFORE any state change.
		effect, e := billing.ResolvePlan(r.PlanKey)
		if e != nil {
			return e
		}
		if r.DurationMinutes <= 0 {
			return &httpError{status: http.StatusBadRequest, msg: "request has no duration (fail closed)"}
		}
		claimed, e := h.requestRepo.ApproveRequestTXN(tx, businessID, requestID, now)
		if e != nil {
			return e
		}
		if claimed == nil {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}
		ap, e := applyPlanEffectTx(tx, h.quotaRepo, businessID, effect, r.DurationMinutes, now)
		if e != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "failed to grant entitlement: " + e.Error()}
		}
		res.Request = claimed
		res.Applied = ap
		return nil
	})
	if err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

// ResolveRequestBusiness looks up the owning business of a request on an owner
// connection (NOT a tenant-pinned one — owner reads on a pinned tx would be
// filtered by RLS). Used by the Telegram bridge, which only has a request id.
func (h *BillingHandler) ResolveRequestBusiness(requestID string) (string, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	r, err := h.requestRepo.GetRequestByIDOwnerTXN(tx, requestID)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", billing.ErrNotFound
	}
	return r.BusinessID, nil
}

// DecidePurchaseRequest satisfies telegram.PurchaseDecider: decides on an owner
// tx (the bridge already resolved the business on its own owner read, so no
// tenant pin is needed).
func (h *BillingHandler) DecidePurchaseRequest(businessID, requestID string, approve bool) error {
	_, err := h.DecidePurchase(func() (*sql.Tx, error) { return h.db.Begin() }, businessID, requestID, approve)
	if err != nil {
		return err
	}
	return nil
}
