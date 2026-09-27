package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// MagicCoinHandler serves the Magic Coins feature: a per-user balance and the
// immutable ledger behind it. Ported from Mikromagic's magic_coin_* tables.
type MagicCoinHandler struct {
	db       *database.DB
	coinRepo *database.MagicCoinRepository
}

// NewMagicCoinHandler wires the coin repository.
func NewMagicCoinHandler(db *database.DB, coinRepo *database.MagicCoinRepository) *MagicCoinHandler {
	return &MagicCoinHandler{db: db, coinRepo: coinRepo}
}

// GetWallet returns the CALLING user's balance. A user with no wallet yet gets a
// zero balance rather than a 404, so the UI never has to special-case "new".
func (h *MagicCoinHandler) GetWallet(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	userID := auth.GetUserID(c)
	if businessID == "" || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var wallet *model.MagicCoinWallet
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		w, err := h.coinRepo.GetWalletTXN(tx, businessID, userID)
		if err != nil {
			return err
		}
		wallet = w
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if wallet == nil {
		// No wallet row: report an explicit zero balance for this user.
		c.JSON(http.StatusOK, gin.H{"wallet": gin.H{
			"user_id":     userID,
			"business_id": businessID,
			"balance":     0,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"wallet": wallet})
}

// ListTransactions returns the business's coin ledger, newest first. The ledger is
// append-only, so this doubles as the audit trail of every balance change.
func (h *MagicCoinHandler) ListTransactions(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	var txs []*model.MagicCoinTransaction
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, err := h.coinRepo.ListTransactionsTXN(tx, businessID, limit)
		if err != nil {
			return err
		}
		txs = list
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"transactions": txs, "count": len(txs)})
}

// Adjust performs a manual credit or debit (OWNER/ADMIN only — enforced at the
// route). Only manual_credit / manual_debit are accepted: every other kind is
// produced by its own flow and must never be fabricated by hand.
func (h *MagicCoinHandler) Adjust(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	actorID := auth.GetUserID(c)
	if businessID == "" || actorID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.MagicCoinAdjustRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be greater than zero"})
		return
	}
	if !isUUID(req.UserID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	kind := model.MagicCoinKind(req.Kind)
	delta := req.Amount
	if kind == model.CoinManualDebit {
		delta = -req.Amount
	}

	var t *model.MagicCoinTransaction
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		txRow, err := h.coinRepo.AdjustTXN(tx, businessID, req.UserID, delta, kind, req.Note, actorID)
		if err != nil {
			return err
		}
		t = txRow
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, database.ErrInsufficientCoins):
			c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient coin balance"})
		case errors.Is(err, database.ErrCoinUserNotInBusiness):
			c.JSON(http.StatusBadRequest, gin.H{"error": "user does not belong to this business"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"transaction": t})
}
