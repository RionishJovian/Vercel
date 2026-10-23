package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/billing"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/telegram"
	"mega-magic/internal/voucher"
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

// findGuestPlan returns the owner-defined guest plan with the given key, or nil
// when it is not one of the plans the owner has listed for sale.
func findGuestPlan(plans []model.GuestPlan, key string) *model.GuestPlan {
	key = strings.TrimSpace(key)
	for _, p := range plans {
		if p.Key == key {
			return &p
		}
	}
	return nil
}

// GuestWifiHandler serves the OWNER-facing guest wifi surface:
//   - PROTECTED (operator-authenticated, tenant-scoped): the owner binds/test
//     their OWN Telegram bot token under their account, lists/decides their
//     guest wifi purchases (approve/reject exactly once), and reads config.
//   - BRIDGE: the same shared decide-core is used by the owner's per-business
//     Telegram bot so HTTP and bot act identically (single money path).
type GuestWifiHandler struct {
	db             *database.DB
	deviceRepo     *database.DeviceRepository
	botRepo        *database.BusinessTelegramBotRepository
	reqRepo        *database.GuestPurchaseRequestRepository
	configRepo     *database.PortalConfigRepository
	quotaRepo      *database.BusinessMeteredQuotaRepository
	receiptRoot    string
	graceCookieKey []byte
	svcFmt         func(token string) (telegram.Messenger, bool) // build messenger from an owner token
	graceRouter    interface {
		ConfigureGuestGrace(context.Context, string, string, routeros.GuestGrace) error
		DecideGuestAccess(context.Context, string, string, string, string, bool, int, int) error
	}
}

func (h *GuestWifiHandler) EnableGuestGrace(router interface {
	ConfigureGuestGrace(context.Context, string, string, routeros.GuestGrace) error
	DecideGuestAccess(context.Context, string, string, string, string, bool, int, int) error
}) {
	h.graceRouter = router
}

// NewGuestWifiHandler wires the handler. svcFmt builds a Messenger lazily from a
// per-owner token so the handler never holds other people's secrets.
func NewGuestWifiHandler(
	db *database.DB,
	deviceRepo *database.DeviceRepository,
	botRepo *database.BusinessTelegramBotRepository,
	reqRepo *database.GuestPurchaseRequestRepository,
	configRepo *database.PortalConfigRepository,
	quotaRepo *database.BusinessMeteredQuotaRepository,
	receiptRoot string,
	svcFmt func(token string) (telegram.Messenger, bool),
) *GuestWifiHandler {
	return &GuestWifiHandler{
		db:          db,
		deviceRepo:  deviceRepo,
		botRepo:     botRepo,
		reqRepo:     reqRepo,
		configRepo:  configRepo,
		quotaRepo:   quotaRepo,
		receiptRoot: receiptRoot,
		svcFmt:      svcFmt,
	}
}

// withTenantTx runs fn inside a tenant-scoped transaction (from the calling
// connection: pinned owner-tx under enforcement, else an opened tx).
func (h *GuestWifiHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// ---------------------------------------------------------------------------
// Guest-facing public purchase (d3) — anonymous, device-anchored
// ---------------------------------------------------------------------------

// PublicGuestPurchase: POST /api/v1/portal/:deviceId/guest-purchase.
// A guest on a device picks a wifi data plan sold via online banking. They get
// a free grace window (configurable per business) to make the bank transfer
// and attach the receipt. The request is created pending; the owner decides.
func (h *GuestWifiHandler) PublicGuestPurchase(c *gin.Context) {
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
	// Resolve the device's business and read its portal config (grace + plans)
	// in the device-derived tenant before ANY state change.
	var cfg *model.PortalConfig
	err = withDeviceTenant(h.db, dev.BusinessID, func(tx *sql.Tx) error {
		pc, e := h.configRepo.GetByDeviceTXN(tx, dev.BusinessID, deviceID)
		if e != nil {
			return e
		}
		cfg = pc
		return nil
	})
	if err != nil || cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "portal config unavailable"})
		return
	}
	if !cfg.MobilePaymentEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "mobile payment is disabled for this portal"})
		return
	}
	if cfg.ConsentRequired && c.PostForm("consent") != "true" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accept the hotspot terms before starting payment"})
		return
	}

	planKey := strings.TrimSpace(c.PostForm("plan_key"))
	if planKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan_key required"})
		return
	}
	// Fail closed: the plan must be one the OWNER listed in guest_plans.
	plan := findGuestPlan(cfg.GuestPlans, planKey)
	if plan == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan not available"})
		return
	}
	// Fail closed: no grace window => refuse (a guest needs the free session).
	graceMinutes := cfg.GuestGraceMinutes
	if graceMinutes <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "online purchase disabled for this business"})
		return
	}
	if cfg.GuestGraceDataMB < 1 || cfg.GuestGraceDataMB > 10240 || graceMinutes > 1440 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the hotspot owner must configure valid grace limits"})
		return
	}
	mac, macErr := net.ParseMAC(strings.TrimSpace(c.PostForm("client_mac")))
	if macErr != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid client_mac required"})
		return
	}
	clientMAC := strings.ToUpper(mac.String())
	if h.graceRouter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporary internet is unavailable; contact the hotspot owner"})
		return
	}

	now := time.Now().UTC()
	cookieClaim, cookieErr := h.readGuestGraceCookie(c, dev.BusinessID, deviceID, now)
	if cookieErr != nil {
		respondErr(c, cookieErr)
		return
	}
	graceStart := now
	graceExpires := voucher.ExtendExpiry(nil, now, graceMinutes)

	var req *model.GuestPurchaseRequest
	createdGrace := false
	err = withDeviceTenant(h.db, dev.BusinessID, func(tx *sql.Tx) error {
		// Keep lock order consistent with the non-cookie purchase path.
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, dev.BusinessID+":"+deviceID+":"+clientMAC); e != nil {
			return e
		}
		if cookieClaim != nil {
			// Lock the original request against concurrent owner decisions. The
			// cookie proves only possession; all state comes from tenant storage.
			prior, e := h.reqRepo.GetForDecisionTXN(tx, dev.BusinessID, cookieClaim.Request)
			if e != nil {
				return e
			}
			if e := checkGuestCookieRequest(prior, *cookieClaim, clientMAC, now); e != nil {
				return e
			}
		}
		// Serialize concurrent purchase clicks for this tenant/device/client. A
		// retry resumes the same allowance rather than receiving another 50 MB.
		previous, e := h.reqRepo.LatestGraceTXN(tx, dev.BusinessID, deviceID, clientMAC)
		if e != nil {
			return e
		}
		if previous != nil {
			resume, retryErr := guestGraceRetry(previous, now)
			if retryErr != nil {
				return retryErr
			}
			if resume && previous.PlanKey != plan.Key {
				return &httpError{status: http.StatusConflict, msg: "a purchase is already pending for this device"}
			}
			if !resume {
				// Remove only this failed request's router user, never a paid or
				// unrelated account. Close its pending decision before retrying.
				if e := h.graceRouter.DecideGuestAccess(c.Request.Context(), dev.BusinessID, deviceID, previous.ID, clientMAC, false, 0, 0); e != nil {
					return &httpError{status: http.StatusBadGateway, msg: "could not close the previous temporary session"}
				}
				if previous.Status == "pending" {
					if _, e := h.reqRepo.RejectRequestTXN(tx, dev.BusinessID, previous.ID, now); e != nil {
						return e
					}
				}
				previous = nil
			}
			if resume {
				req = previous
				graceExpires = *previous.GraceExpiresAt
				return nil
			}
		}
		r, e := h.reqRepo.CreateTXN(tx, &model.GuestPurchaseRequest{
			BusinessID:       dev.BusinessID,
			DeviceID:         &deviceID,
			ClientMAC:        clientMAC,
			PlanKey:          plan.Key,
			PurchasedMinutes: &plan.Minutes,
			Amount:           plan.Amount,
			DataMB:           plan.DataMB,
			GraceStartedAt:   &graceStart,
			GraceExpiresAt:   &graceExpires,
			GraceDataMB:      cfg.GuestGraceDataMB,
			Status:           "pending",
		})
		if e != nil {
			return e
		}
		req = r
		// PostgreSQL stores microsecond precision. Use its returned deadline for
		// both router provisioning and the response, including future resumes.
		graceExpires = *r.GraceExpiresAt
		if e := h.graceRouter.ConfigureGuestGrace(c.Request.Context(), dev.BusinessID, deviceID, routeros.GuestGrace{RequestID: r.ID, ClientMAC: clientMAC, Minutes: graceMinutes, DataMB: r.GraceDataMB, ExpiresAt: graceExpires}); e != nil {
			return &httpError{status: http.StatusBadGateway, msg: "temporary internet could not be configured; the hotspot owner should check router connectivity and MAC login settings"}
		}
		createdGrace = true
		return nil
	})
	if err != nil {
		if createdGrace && req != nil {
			// Compensate if persisting the pending request fails after the
			// router write. This cleanup is scoped by the request comment.
			_ = h.graceRouter.DecideGuestAccess(context.Background(), dev.BusinessID, deviceID, req.ID, clientMAC, false, 0, 0)
		}
		respondErr(c, err)
		return
	}

	h.writeGuestGraceCookie(c, dev.BusinessID, deviceID, req.ID, now)
	c.JSON(http.StatusCreated, gin.H{
		"request_id":       req.ID,
		"status":           req.Status,
		"plan":             plan.Label,
		"amount":           req.Amount,
		"grace_expires_at": graceExpires,
		"grace_data_mb":    req.GraceDataMB,
		"grace_minutes":    int(req.GraceExpiresAt.Sub(*req.GraceStartedAt) / time.Minute),
		"grace_configured": true,
		"message":          "attach your payment receipt within the grace window to request the time extension",
	})
}

// PublicAttachReceipt: POST /api/v1/portal/guest-purchase/:requestId/receipt.
// The guest attaches the successful payment receipt, but ONLY within the live
// grace window (they had the free session to make the transfer). Outside the
// window the attach is refused (fail closed) so a stale receipt cannot claim it.
func (h *GuestWifiHandler) PublicAttachReceipt(c *gin.Context) {
	requestID := strings.TrimSpace(c.Param("requestId"))
	now := time.Now().UTC()

	// Resolve the request's business + grace window on an owner read (the guest
	// is anonymous; the request is device-anchored).
	var req *model.GuestPurchaseRequest
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	r, err := h.reqRepo.GetByIDOwnerTXN(tx, requestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	req = r
	if req == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}
	if req.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "request already decided"})
		return
	}
	// Grace-window gate: the receipt must arrive while the free session is live.
	if req.GraceExpiresAt == nil || !now.Before(*req.GraceExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"error": "grace window expired; contact the hotspot owner"})
		return
	}

	phone := strings.TrimSpace(c.PostForm("paid_phone"))
	last6 := strings.TrimSpace(c.PostForm("tx_last6"))
	if !validGuestPaymentDetails(phone, last6) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enter a valid payer phone and six transaction digits"})
		return
	}
	var savedPath string
	fileHeader, ferr := c.FormFile("receipt")
	if ferr != nil && !errors.Is(ferr, http.ErrMissingFile) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt upload"})
		return
	}
	if fileHeader != nil {
		f, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read receipt"})
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, maxReceiptBytes+1))
		_ = f.Close()
		if err != nil || len(data) == 0 || len(data) > maxReceiptBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "receipt must be a photo no larger than 5 MiB"})
			return
		}
		mime := http.DetectContentType(data)
		if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "receipt must be JPEG, PNG or WebP"})
			return
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err == nil {
			savedPath, err = billing.SaveReceipt(h.receiptRoot, req.BusinessID, req.ID+"-"+hex.EncodeToString(nonce[:]), data)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "receipt could not be saved; try again"})
			return
		}
	}
	err = database.WithTenant(tx, req.BusinessID, func() error {
		updated, err := h.reqRepo.SubmitPaymentDetailsTXN(tx, req.BusinessID, req.ID, phone, last6, savedPath)
		if err != nil {
			return err
		}
		if !updated {
			return &httpError{http.StatusGone, "payment window expired or request already decided"}
		}
		return nil
	})
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			c.JSON(he.status, gin.H{"error": he.msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "payment details could not be saved"})
		}
		return
	}

	// Ping the OWNER's own bot (per-business binding) so they know a guest
	// finished paying and is asking for the wifi extension. Best-effort: a
	// missing/inactive bot binding must NOT fail the guest's submission.
	h.notifyOwnerPurchase(req, phone, last6)

	c.JSON(http.StatusOK, gin.H{"request_id": req.ID, "status": req.Status, "receipt_attached": savedPath != "", "payment_details_saved": true})
}

func validGuestPaymentDetails(phone, last6 string) bool {
	if len(phone) < 7 || len(phone) > 16 || len(last6) != 6 {
		return false
	}
	for i, r := range phone {
		if r == '+' && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, r := range last6 {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// notifyOwnerPurchase sends the owner's bound bot a verification prompt for a
// guest who submitted their MMQR payment details. Fail-open (never blocks the
// guest on a bot problem): no binding or send error is swallowed with `_ =`.
func (h *GuestWifiHandler) notifyOwnerPurchase(req *model.GuestPurchaseRequest, phone, last6 string) {
	var bot *model.BusinessTelegramBot
	err := withDeviceTenant(h.db, req.BusinessID, func(tx *sql.Tx) error {
		b, e := h.botRepo.GetByBusinessTXN(tx, req.BusinessID)
		if e != nil {
			return e
		}
		bot = b
		return nil
	})
	if err != nil || bot == nil || !bot.Enabled || strings.TrimSpace(bot.BotToken) == "" {
		return // fail open: no usable owner bot
	}
	target := "0"
	if bot.TargetChatID != 0 {
		target = strconv.FormatInt(bot.TargetChatID, 10)
	}
	msg, ok := h.svcFmt(bot.BotToken)
	if !ok || msg == nil {
		return
	}
	text := "🛜 Guest paid — approve?"
	text += "\nOrder: " + telegram.OrderReference(req.ID)
	if req.PlanKey != "" {
		text += "\nPlan: " + req.PlanKey
	}
	if req.Amount > 0 {
		text += "\nAmount: " + telegram.FormatMMK(req.Amount)
	}
	if phone != "" {
		text += "\nPaid phone: " + phone
	}
	if last6 != "" {
		text += "\nTx last 6: " + last6
	}
	// Prefer inline Approve/Reject buttons when the transport is the real Bot;
	// fall back to typed-text instructions otherwise (e.g. test fakes).
	if bot, okb := msg.(*telegram.Bot); okb {
		_ = bot.SendButtons(target, text, req.ID)
		return
	}
	_ = msg.SendMessage(target, text+"\n\nReview this payment in Mega Magic to approve or reject it.")
}

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
	out := gin.H{"enabled": false, "target_chat_id": 0, "approver_user_id": 0, "bot_token": ""}
	if bot != nil {
		out = gin.H{
			"enabled":          bot.Enabled,
			"target_chat_id":   bot.TargetChatID,
			"approver_user_id": bot.ApproverUserID,
			"bot_token":        maskToken(bot.BotToken),
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
		BotToken       string `json:"bot_token"`
		TargetChatID   int64  `json:"target_chat_id"`
		ApproverUserID int64  `json:"approver_user_id"`
		Enabled        bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.ApproverUserID < 0 || body.ApproverUserID > 9007199254740991 || (body.Enabled && (body.ApproverUserID == 0 || body.TargetChatID == 0)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a positive Telegram user ID and an approval chat ID before enabling approvals"})
		return
	}
	token := strings.TrimSpace(body.BotToken)
	if token == "" {
		// An empty token means "keep the already-bound secret". This lets an
		// owner change the chat or enabled state from a masked profile form without
		// making the API echo the stored token back to the browser.
		err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
			existing, e := h.botRepo.GetByBusinessTXN(tx, businessID)
			if e != nil {
				return e
			}
			if existing != nil {
				token = strings.TrimSpace(existing.BotToken)
			}
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token required for the first binding"})
			return
		}
	}
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		return h.botRepo.UpsertTXN(tx, &model.BusinessTelegramBot{
			BusinessID:     businessID,
			BotToken:       token,
			TargetChatID:   body.TargetChatID,
			ApproverUserID: body.ApproverUserID,
			Enabled:        body.Enabled,
		})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": body.Enabled, "target_chat_id": body.TargetChatID, "approver_user_id": body.ApproverUserID, "bot_token": maskToken(token)})
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
		r, e := h.reqRepo.GetForDecisionTXN(tx, businessID, requestID)
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
			if r.DeviceID != nil {
				if h.graceRouter == nil {
					return &httpError{status: http.StatusServiceUnavailable, msg: "router access is unavailable"}
				}
				if e := h.graceRouter.DecideGuestAccess(context.Background(), businessID, *r.DeviceID, r.ID, r.ClientMAC, false, 0, 0); e != nil {
					return e
				}
			}
			_, e := h.reqRepo.RejectRequestTXN(tx, businessID, requestID, time.Now().UTC())
			if e != nil {
				return e
			}
			return nil
		}
		// Never reconstruct the paid duration from a mutable plan or convert
		// historical guest purchases into business Hub/WebFig access.
		if r.DeviceID == nil || r.PurchasedMinutes == nil {
			return &httpError{status: 409, msg: "Historical purchase lacks an immutable router-bound allowance; manual review required"}
		}
		minutes := *r.PurchasedMinutes
		if minutes < 0 || minutes > 525600 || r.DataMB < 0 || r.DataMB > 1048576 || (minutes == 0 && r.DataMB == 0) {
			return &httpError{status: 409, msg: "Purchased allowance is invalid; manual review required"}
		}
		now := time.Now().UTC()
		claimed, e := h.reqRepo.ApproveRequestTXN(tx, businessID, requestID, now)
		if e != nil {
			return e
		}
		if claimed == nil {
			return &httpError{status: http.StatusConflict, msg: "request already decided"}
		}
		if h.graceRouter == nil {
			return &httpError{status: http.StatusServiceUnavailable, msg: "router access is unavailable"}
		}
		return h.graceRouter.DecideGuestAccess(context.Background(), businessID, *r.DeviceID, r.ID, r.ClientMAC, true, minutes, r.DataMB)
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
