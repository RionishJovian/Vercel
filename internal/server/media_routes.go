package server

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
	"strconv"
	"time"
)

// Registered only after an explicit feature flag and schema check. Connector
// and owner surfaces never share authentication middleware or credentials.
func registerMediaRoutes(r *gin.Engine, service *auth.Service, db *database.DB, cfg *config.Config, access ...gin.HandlerFunc) error {
	var ok bool
	if e := db.QueryRow(`SELECT count(*)=2 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN ('magic_media_connections','magic_media_jobs') AND c.relrowsecurity AND c.relforcerowsecurity`).Scan(&ok); e != nil {
		return e
	}
	if !ok {
		return bridge.ErrUnavailable
	}
	// The dashboard operation requires the additive constraint migration. Do not
	// start an opted-in release that would advertise jobs the database rejects.
	if e := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.magic_media_jobs'::regclass AND conname='magic_media_jobs_operation_check' AND position('dashboard' in pg_get_constraintdef(oid))>0)`).Scan(&ok); e != nil {
		return e
	}
	if !ok {
		return bridge.ErrUnavailable
	}
	h := handler.NewMediaHandler(bridge.NewPostgres(db))
	owner := r.Group("/api/v1/media", auth.CookieOrigin(), auth.Middleware(service), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleAdmin))
	owner.Use(access...)
	owner.POST("/pairing", h.Pair)
	owner.GET("/connection", h.Connection)
	owner.DELETE("/connection", h.Revoke)
	owner.POST("/jobs", h.Enqueue)
	owner.GET("/jobs/:id", h.Job)
	// Additive rollout: an unmigrated server keeps the existing connector usable,
	// but cannot report a successful publication.
	var publishingReady bool
	if e := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname='magic_media_publications' AND c.relrowsecurity AND c.relforcerowsecurity)`).Scan(&publishingReady); e != nil {
		return e
	}
	publishing := &handler.MediaPublicationHandler{Store: bridge.NewPostgres(db), Ready: publishingReady}
	owner.GET("/publication", publishing.Get)
	owner.PUT("/publication", publishing.Publish)
	owner.DELETE("/publication", publishing.Unpublish)
	r.GET("/api/v1/media-guest/:id", auth.BoundedAuthRateLimit(), publishing.Public)
	// Paid access is a separate explicit rollout gate; a migrated theme/ledger
	// alone must never solicit payment for an unverified playback service.
	salesReady := false
	if cfg.Media.PaidAccessEnabled {
		if !cfg.GuestWifi.Enabled || cfg.GuestWifi.ReceiptDir == "" {
			return errors.New("media paid access requires owner-bot supervisor and private receipt storage")
		}
		if e := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname='magic_media_passes' AND c.relrowsecurity AND c.relforcerowsecurity) AND (SELECT count(*)=4 FROM information_schema.columns WHERE table_schema='public' AND table_name='magic_media_passes' AND column_name IN ('review_id','receipt_hash','receipt_submitted_at','payment_details'))`).Scan(&salesReady); e != nil {
			return e
		}
		if !salesReady {
			return errors.New("media paid-access schema must be explicitly migrated")
		}
	}
	botRepo := database.NewBusinessTelegramBotRepository(db)
	getBot := func(ctx context.Context, bid string) (*model.BusinessTelegramBot, error) {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		var bot *model.BusinessTelegramBot
		e = database.WithTenant(tx, bid, func() error { var err error; bot, err = botRepo.GetByBusinessTXN(tx, bid); return err })
		if e != nil || bot == nil || !bot.Enabled || bot.BotToken == "" || bot.TargetChatID == 0 || bot.ApproverUserID <= 0 {
			return nil, errors.New("owner bot unavailable")
		}
		return bot, nil
	}
	purchases := &handler.MediaPurchaseHandler{Store: bridge.NewPostgres(db), Ready: salesReady, ReceiptRoot: cfg.GuestWifi.ReceiptDir,
		ReviewReady: func(ctx context.Context, bid string) bool { _, e := getBot(ctx, bid); return e == nil },
		Notify: func(ctx context.Context, bid string, pass bridge.MediaPass, receipt bridge.MediaReceipt, photo []byte) error {
			bot, e := getBot(ctx, bid)
			if e != nil {
				return e
			}
			messenger, ok := telegram.ConfigMessenger(bot.BotToken, "", 30*time.Second)
			if !ok {
				return errors.New("owner bot unavailable")
			}
			transport, ok := messenger.(telegram.ReceiptMessenger)
			if !ok {
				return errors.New("receipt transport unavailable")
			}
			return transport.SendReceipt(strconv.FormatInt(bot.TargetChatID, 10), "Magic Media receipt — review before approving\nOrder: "+telegram.OrderReference(receipt.ReviewID)+"\nAmount: "+telegram.FormatMMK(pass.Amount), receipt.ReviewID, photo)
		},
	}
	// The ledger flag alone cannot advertise playable LAN media.
	publishing.PurchasesEnabled = false
	playback := handler.NewMediaPlaybackHandler(bridge.NewPostgres(db), salesReady)
	// Catalog reachability is not proof of LAN playback. Do not solicit a bank
	// transfer until the owner-PC local delivery path has passed acceptance.
	purchases.PlaybackReady = func(context.Context, string, string) bool { return false }
	player := r.Group("/api/v1/media-playback", auth.BoundedPortalRateLimit(), auth.CookieOrigin())
	player.POST("/:id/:passId/session", playback.Session)
	player.GET("/:id/:passId/catalog/:library", playback.Catalog)
	player.GET("/:id/:passId/stream/:library/:item", playback.Stream)
	guest := r.Group("/api/v1/media-guest", auth.BoundedPortalRateLimit(), auth.CookieOrigin())
	guest.POST("/:id/checkout", purchases.Checkout)
	guest.GET("/:id/passes/:passId", purchases.Status)
	guest.POST("/:id/passes/:passId/receipt", purchases.Receipt)
	connectorSecurity := func(c *gin.Context) {
		if c.GetHeader("Origin") != "" {
			c.AbortWithStatus(403)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
	connector := r.Group("/api/v1/media-connector", auth.BoundedAuthRateLimit(), connectorSecurity)
	connector.POST("/redeem", h.Redeem)
	connector.POST("/poll", h.Poll)
	connector.POST("/jobs/:id/result", h.Complete)
	relayRoutes := r.Group("/api/v1/media-connector/relay", auth.BoundedMediaRelayRateLimit(), connectorSecurity)
	relayRoutes.POST("/poll", playback.Poll)
	relayRoutes.POST("/:task/check", playback.Check)
	relayRoutes.GET("/:task/socket", playback.Socket)
	return nil
}
