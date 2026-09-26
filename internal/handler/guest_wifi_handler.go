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
	"mega-magic/internal/telegram"
)

// errGuestNotFound signals a guest purchase request that cannot be resolved.
var errGuestNotFound = errors.New("guest purchase request not found")

// maskToken renders a Telegram bot token masked for any read-back so a stored
// secret never round-trips to the client. Only the last 4 chars are shown.
func maskToken(token string) string {
	t := strings.TrimSpace(token)
	if len(t) <= 8 {
		return "••••"
	}
	return "••••••••" + t[len(t)-4:len(t)]
}

// GuestWifiHandler serves the OWNER-facing guest wifi surface:
//   - PROTECTED (operator-authenticated, tenant-scoped): the owner binds/test
//     their OWN Telegram bot token under their account, lists/decides their
//     guest wifi purchases (approve/reject exactly once), and reads config.
//   - BRIDGE: the same shared decide-core is used by the owner's per-business
//     Telegram bot so HTTP and bot act identically (single money path).
type GuestWifiHandler struct {
	db         *database.DB
	botRepo    *database.BusinessTelegramBotRepository
	reqRepo    *database.GuestPurchaseRequestRepository
	configRepo *database.PortalConfigRepository
	svcFmt     func(token string) (telegram.Messenger, bool) // build messenger from an owner token
}

// NewGuestWifiHandler wires the handler. svcFmt builds a Messenger lazily from a
// per-owner token so the handler never holds other people's secrets.
func NewGuestWifiHandler(
	db *database.DB,
	botRepo *database.BusinessTelegramBotRepository,
	reqRepo *database.GuestPurchaseRequestRepository,
	configRepo *database.PortalConfigRepository,
	svcFmt func(token string) (telegram.Messenger, bool),
) *GuestWifiHandler {
	return &GuestWifiHandler{
		db:         db,
		botRepo:    botRepo,
		reqRepo:    reqRepo,
		configRepo: configRepo,
		svcFmt:     svcFmt,
	}
}

// withTenantTx runs fn inside a tenant-scoped transaction (from the calling
// connection: pinned owner-tx under enforcement, else an opened tx).
func (h *GuestWifiHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// ---------------------------------------------------------------------------
// Owner bot binding (d2)
// ---------------------------------------------------------------------------

// GetBotBinding: GET /api/v1/guest-wifi/telegram-bot — returns the owner's own
// bot binding with the token MASKED. 200 with an enabled=false shell when none.
func (h *GuestWifiHandler) GetBotBinding(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var bot *model.BusinessTelegramBot
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		b, e := h.botRepo.GetByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		bot = b
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	out := gin.H{"enabled": false, "target_chat_id": 0, "bot_token": ""}
	if bot != nil {
		out = gin.H{
			"enabled":        bot.Enabled,
			"target_chat_id": bot.TargetChatID,
			"bot_token":      maskToken(bot.BotToken),
		}
	}
	c.JSON(http.StatusOK, out)
}

// PutBotBinding: PUT /api/v1/guest-wifi/telegram-bot — binds the owner's OWN bot
// token under their account. Token is stored server-side only; never echoed back.
func (h *GuestWifiHandler) PutBotBinding(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body struct {
		BotToken     string `json:"bot_token"`
		TargetChatID int64  `json:"target_chat_id"`
		Enabled      bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token := strings.TrimSpace(body.BotToken)
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token required"})
		return
	}
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		return h.botRepo.Upsert(&model.BusinessTelegramBot{
			BusinessID:   businessID,
			BotToken:     token,
			TargetChatID: body.TargetChatID,
			Enabled:      body.Enabled,
		})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": body.Enabled, "target_chat_id": body.TargetChatID, "bot_token": maskToken(token)})
}

// TestBotBinding: POST /api/v1/guest-wifi/telegram-bot/test — verifies the
// owner's bound token is live via the Telegram getMe probe, WITHOUT sending.
func (h *GuestWifiHandler) TestBotBinding(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var bot *model.BusinessTelegramBot
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		b, e := h.botRepo.GetByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		bot = b
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if bot == nil || strings.TrimSpace(bot.BotToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no bot token bound"})
		return
	}
	// Build the transport from the token and probe getMe.
	msg, ok := h.svcFmt(bot.BotToken)
	if !ok || msg == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot build transport", "ok": false})
		return
	}
	fine, detail := msg.VerifyToken()
	c.JSON(http.StatusOK, gin.H{"ok": fine, "detail": detail})
}

// ---------------------------------------------------------------------------
// Guest purchase decide-core (d4) — SHARED by HTTP + owner bot bridge
// ---------------------------------------------------------------------------

// ResolveRequestBusiness returns the business that owns a guest request id
// (used by the owner's bot bridge, which only has a request id). Satisfies the
// telegram.PurchaseDecider interface for the shared approve/reject bridge.
func (h *GuestWifiHandler) ResolveRequestBusiness(requestID string) (string, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	r, err := h.reqRepo.GetByIDOwnerTXN(tx, requestID)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", errGuestNotFound
	}
	return r.BusinessID, nil
}

// DecideGuest approves (true) or rejects (false) a guest wifi purchase EXACTLY
// ONCE (single-wind) under the owning business's tenant scope. Approve grants
// the plan's data duration as a wifi time extension per the portal config.
// The business param is authoritative (HTTP: owner JWT; bridge: owner-scope).
func (h *GuestWifiHandler) DecideGuest(txSrc func() (*sql.Tx, error), businessID, requestID string, approve bool) error {
	tx, err := txSrc()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = database.WithTenant(tx, businessID, func() error {
		r, e := h.reqRepo.GetByIDTXN(tx, businessID, requestID)
		if e != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if r == nil {
			return &httpError{status: http.StatusNotFound, msg: "guest purchase request not found"}
		}
		if r.Status != "pending" {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}
		if !approve {
			_, e := h.reqRepo.RejectRequestTXN(tx, businessID, requestID, time.Now().UTC())
			if e != nil {
				return e
			}
			return nil
		}
		claimed, e := h.reqRepo.ApproveRequestTXN(tx, businessID, requestID, time.Now().UTC())
		if e != nil {
			return e
		}
		if claimed == nil {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// DecidePurchaseRequest satisfies telegram.PurchaseDecider so the owner's own
// per-business bot bridge can decide on its own connection.
func (h *GuestWifiHandler) DecidePurchaseRequest(businessID, requestID string, approve bool) error {
	return h.DecideGuest(func() (*sql.Tx, error) { return h.db.Begin() }, businessID, requestID, approve)
}

// ---------------------------------------------------------------------------
// Owner protected web endpoints (d2/d4)
// ---------------------------------------------------------------------------

// ListGuestRequests: GET /api/v1/guest-wifi/requests — the owner's guest loop.
func (h *GuestWifiHandler) ListGuestRequests(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var reqs []*model.GuestPurchaseRequest
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		rs, e := h.reqRepo.ListByBusinessTXN(tx, businessID, 50)
		if e != nil {
			return e
		}
		reqs = rs
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": reqs, "count": len(reqs)})
}

// ApproveGuest: POST /api/v1/guest-wifi/approve/:requestId (owner web).
func (h *GuestWifiHandler) ApproveGuest(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("requestId")
	err := h.DecideGuest(func() (*sql.Tx, error) { return beginTenantTx(c, h.db) }, businessID, id, true)
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"applied": true, "request": id})
}

// RejectGuest: POST /api/v1/guest-wifi/reject/:requestId (owner web).
func (h *GuestWifiHandler) RejectGuest(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("requestId")
	err := h.DecideGuest(func() (*sql.Tx, error) { return beginTenantTx(c, h.db) }, businessID, id, false)
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"applied": true, "request": id})
}