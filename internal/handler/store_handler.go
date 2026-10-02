package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/store"
	"mega-magic/internal/telegram"
)

type StoreHandler struct {
	SubscriptionOnly bool
	db               *database.DB
	purchases        *database.StorePurchaseRepository
	pricing          *database.PricingRepository
	messenger        telegram.Messenger
	operatorChat     string
}

func NewStoreHandler(db *database.DB, purchases *database.StorePurchaseRepository, pricing *database.PricingRepository, messenger telegram.Messenger, operatorChat string) *StoreHandler {
	return &StoreHandler{db: db, purchases: purchases, pricing: pricing, messenger: messenger, operatorChat: operatorChat}
}

type createStorePurchaseRequest struct {
	ProductKey     string `json:"product_key" binding:"required"`
	CoinCount      int    `json:"coin_count"`
	BankAccountID  string `json:"bank_account_id" binding:"required"`
	BankReference  string `json:"bank_reference" binding:"required"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

func (h *StoreHandler) Create(c *gin.Context) {
	var in createStorePurchaseRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkout request"})
		return
	}
	in.BankReference, in.IdempotencyKey = strings.TrimSpace(in.BankReference), strings.TrimSpace(in.IdempotencyKey)
	if h.SubscriptionOnly && in.ProductKey != store.ProductMonthly && in.ProductKey != store.ProductAnnual {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only monthly and annual plans are available", "code": "product_disabled"})
		return
	}
	if in.BankReference == "" || len(in.BankReference) > 120 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 120 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bank reference or idempotency key"})
		return
	}
	pricing, err := h.pricing.GetPromo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store pricing unavailable"})
		return
	}
	product, err := store.ResolveProduct(in.ProductKey, in.CoinCount, *pricing, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	businessID, userID := auth.GetBusinessID(c), auth.GetUserID(c)
	var purchase *model.StorePurchase
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		var e error
		purchase, e = h.purchases.CreateTXN(tx, businessID, userID, product, in.BankAccountID, in.BankReference, in.IdempotencyKey)
		return e
	})
	if err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "checkout already submitted"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.notify(purchase)
	c.JSON(http.StatusCreated, gin.H{"purchase": purchase, "message": "submitted for operator approval"})
}

func (h *StoreHandler) List(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	var purchases []*model.StorePurchase
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error { var e error; purchases, e = h.purchases.ListTXN(tx, businessID); return e })
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"purchases": purchases, "count": len(purchases)})
}

func (h *StoreHandler) CheckoutOptions(c *gin.Context) {
	accounts, err := h.purchases.ListEnabledBankAccounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "checkout options unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bank_accounts": accounts, "approval": "Telegram operator approval activates the purchase atomically"})
}

func (h *StoreHandler) Approve(c *gin.Context) { h.decideHTTP(c, true) }
func (h *StoreHandler) Reject(c *gin.Context)  { h.decideHTTP(c, false) }

func (h *StoreHandler) decideHTTP(c *gin.Context, approve bool) {
	businessID, actor := auth.GetBusinessID(c), auth.GetUserID(c)
	reason := ""
	if !approve {
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)
		reason = strings.TrimSpace(body.Reason)
		if reason == "" {
			reason = "Rejected by operator"
		}
	}
	var purchase *model.StorePurchase
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		var e error
		purchase, e = h.decidePurchaseTXN(tx, businessID, c.Param("purchaseId"), actor, approve, reason)
		return e
	})
	if err != nil {
		if errors.Is(err, database.ErrStorePurchaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "purchase not found"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"purchase": purchase, "activated": approve})
}

func (h *StoreHandler) notify(p *model.StorePurchase) {
	if h.messenger == nil || h.operatorChat == "" {
		return
	}
	// Resolve the display name with the same tenant boundary as the order.
	// A failed name lookup must not undo a submitted purchase or expose its UUID.
	businessName := "Business name unavailable"
	if tx, err := h.db.Begin(); err == nil {
		var name string
		err = database.WithTenant(tx, p.BusinessID, func() error {
			var e error
			name, e = database.NewVoucherResellerRepository(h.db).BusinessNameTXN(tx, p.BusinessID)
			return e
		})
		if err == nil && strings.TrimSpace(name) != "" {
			businessName = name
		}
		_ = tx.Rollback()
	}
	text := telegram.StoreApprovalMessage(p.ID, businessName, p.ServiceLabel, p.PriceMMK, value(p.BankReference))
	if buttons, ok := h.messenger.(telegram.ButtonMessenger); ok {
		_ = buttons.SendButtons(h.operatorChat, text, p.ID)
		return
	}
	_ = h.messenger.SendMessage(h.operatorChat, text+"\nReview this order in Mega Magic Store to approve or reject it.")
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// Telegram PurchaseDecider implementation. Approval includes activation in the
// same database transaction; an activation error leaves the purchase pending.
func (h *StoreHandler) ResolveRequestBusiness(requestID string) (string, error) {
	return h.purchases.ResolveBusiness(requestID)
}
func (h *StoreHandler) DecidePurchaseRequest(businessID, requestID string, approve bool) error {
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = database.WithTenant(tx, businessID, func() error {
		_, e := h.decidePurchaseTXN(tx, businessID, requestID, "", approve, "Rejected from Telegram")
		return e
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (h *StoreHandler) decidePurchaseTXN(tx *sql.Tx, businessID, requestID, actor string, approve bool, reason string) (*model.StorePurchase, error) {
	if h.SubscriptionOnly {
		return h.purchases.DecideSubscriptionTXN(tx, businessID, requestID, actor, approve, reason, time.Now().UTC())
	}
	return h.purchases.DecideTXN(tx, businessID, requestID, actor, approve, reason, time.Now().UTC())
}
