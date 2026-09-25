package handler

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/billing"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
)

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
}

func NewBillingHandler(
	db *database.DB,
	deviceRepo *database.DeviceRepository,
	configRepo *database.PortalConfigRepository,
	requestRepo *database.PurchaseRequestRepository,
	voucherRepo *database.VoucherRepository,
	quotaRepo *database.BusinessMeteredQuotaRepository,
	messenger telegram.Messenger,
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

	amount := 0
	if v := c.PostForm("amount"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			amount = n
		}
	}
	bankRef := c.PostForm("bank_reference")

	var req *model.PurchaseRequest
	err = withDeviceTenant(h.db, dev.BusinessID, func(tx *sql.Tx) error {
		var bank *string
		if bankRef != "" {
			bank = &bankRef
		}
		r, e := h.requestRepo.CreateRequestTXN(tx, &model.PurchaseRequest{
			BusinessID:    dev.BusinessID,
			DeviceID:      &deviceID,
			PlanKey:       planKey,
			Amount:        amount,
			BankReference: bank,
			Status:        billing.StatusPending,
		})
		if e != nil {
			return e
		}
		req = r
		return nil
	})
	if err != nil {
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

	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()

	var (
		decided *model.PurchaseRequest
		applied gin.H
	)
	now := time.Now().UTC()
	err = database.WithTenant(tx, businessID, func() error {
		r, e := h.requestRepo.GetRequestByIDTXN(tx, businessID, id)
		if e != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if r == nil {
			return &httpError{status: http.StatusNotFound, msg: "purchase request not found"}
		}
		if r.Status != billing.StatusPending {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}

		// Fail closed on an unknown plan BEFORE any state change.
		effect, e := billing.ResolvePlan(r.PlanKey)
		if e != nil {
			return &httpError{status: http.StatusBadRequest, msg: e.Error()}
		}
		minutes, e := billing.PlanDurationMinutes(r.PlanKey)
		if e != nil {
			return &httpError{status: http.StatusBadRequest, msg: e.Error()}
		}

		// Single-wind transition: only one approve can flip pending -> approved.
		claimed, e := h.requestRepo.ApproveRequestTXN(tx, businessID, id, now)
		if e != nil {
			return e
		}
		if claimed == nil {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}

		// Grant via the SHARED money-path core (no second grant path).
		ap, e := applyPlanEffectTx(tx, h.quotaRepo, businessID, effect, minutes, now)
		if e != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "failed to grant entitlement: " + e.Error()}
		}
		decided, applied = claimed, ap
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize approval"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"request": decided, "applied": applied})
}

// RejectPurchase: POST /api/v1/billing/reject/:requestId (operator). Grants nothing.
func (h *BillingHandler) RejectPurchase(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("requestId")

	var decided *model.PurchaseRequest
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		r, e := h.requestRepo.GetRequestByIDTXN(tx, businessID, id)
		if e != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if r == nil {
			return &httpError{status: http.StatusNotFound, msg: "purchase request not found"}
		}
		if r.Status != billing.StatusPending {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}
		claimed, e := h.requestRepo.RejectRequestTXN(tx, businessID, id, time.Now().UTC())
		if e != nil {
			return e
		}
		if claimed == nil {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}
		decided = claimed
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"request": decided})
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
