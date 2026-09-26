package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"mega-magic/internal/ai"
	"mega-magic/internal/auth"
	"mega-magic/internal/billing"
	"mega-magic/internal/cache"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/evaluator"
	"mega-magic/internal/fleet"
	"mega-magic/internal/handler"
	"mega-magic/internal/messaging"
	"mega-magic/internal/monitoring"
	"mega-magic/internal/notifier"
	"mega-magic/internal/probe"
	"mega-magic/internal/quota"
	"mega-magic/internal/recovery"
	"mega-magic/internal/telegram"
	"mega-magic/pkg/logger"
)

type Server struct {
	cfg            *config.Config
	log            *logger.Logger
	sugar          *zap.SugaredLogger
	http           *http.Server
	gin            *gin.Engine
	db             *database.DB
	cache          *cache.Client
	messaging      *messaging.Client
	authService    *auth.Service
	userRepo       *database.UserRepository
	authHandler    *handler.AuthHandler
	evaluator      *evaluator.Evaluator
	voucherHandler *handler.VoucherHandler
	monitorHandler *handler.MonitorHandler
	sweep          *monitoring.Sweep
	quotaSweep     *quota.Sweep
	recovery       *recovery.RecoveryService
	restricted     *database.DB
}

func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	sugar := log.Sugar()

	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("database connection: %w", err)
	}
	sugar.Info("Database connected")

	natsRepo := database.NewNATSSubjectRepository(db)
	messagingClient, err := messaging.NewClient(cfg.NATS, natsRepo)
	if err != nil {
		return nil, fmt.Errorf("messaging client: %w", err)
	}
	sugar.Info("Messaging client connected")

	cacheClient, err := cache.New(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("cache client: %w", err)
	}
	sugar.Info("Cache client connected")

	authService, err := auth.NewService(
		cfg.Security.JWTSecret,
		cfg.Security.JWTExpiry,
		cfg.Security.RefreshExpiry,
		cfg.Security.BcryptCost,
	)
	if err != nil {
		return nil, fmt.Errorf("auth service: %w", err)
	}
	sugar.Info("Auth service initialized")

	userRepo := database.NewUserRepository(db)

	// Notifier fans incident events out to tenant webhook endpoints.
	notifierr := notifier.New(database.NewWebhookRepository(db), db, sugar)

	authHandler := handler.NewAuthHandler(userRepo, authService, db)
	routerHandler := handler.NewRouterHandler(
		db,
		database.NewDeviceRepository(db),
		messagingClient,
	)
	telemetryHandler := handler.NewTelemetryHandler(
		db,
		database.NewTelemetrySampleRepository(db),
		cacheClient,
		messagingClient,
	)
	incidentHandler := handler.NewIncidentHandler(
		db,
		database.NewIncidentRepository(db),
		database.NewAlertPolicyRepository(db),
		database.NewDeviceRepository(db),
		notifierr,
	)
	webhookHandler := handler.NewWebhookHandler(db, database.NewWebhookRepository(db))
	// Voucher management for vistors (revenue features)
	voucherHandler := handler.NewVoucherHandler(db, database.NewVoucherRepository(db), database.NewBusinessMeteredQuotaRepository(db))
	portalHandler := handler.NewPortalHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewPortalConfigRepository(db),
		database.NewPortalSessionRepository(db),
		database.NewVoucherRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		// Option C: on voucher redemption, notify that business's OWN enabled
		// Telegram bot (informational). Fail closed: no binding / disabled /
		// feature off / send error => no notification, never fails the redeem.
		func(businessID, text string) {
			if !cfg.GuestWifi.Enabled {
				return
			}
			botRepo := database.NewBusinessTelegramBotRepository(db)
			bot, err := botRepo.GetByBusiness(businessID)
			if err != nil || bot == nil || !bot.Enabled || strings.TrimSpace(bot.BotToken) == "" {
				return
			}
			target := "0"
			if bot.TargetChatID != 0 {
				target = strconv.FormatInt(bot.TargetChatID, 10)
			}
			if msg, ok := telegram.ConfigMessenger(bot.BotToken, target, 30*time.Second); ok {
				_ = msg.SendMessage(target, text)
			}
		},
	)

	// Billing operator loop: a Telegram bot carries BOTH the outbound purchase
	// notification and the inbound approve/reject reply. It is flag-gated and
	// default-off; with no token the messenger is nil and the operator decides
	// on the protected web endpoint (the shared core either way).
	planDurations := billing.PlanDurations{}
	for k, pc := range cfg.Billing.Plans {
		planDurations[k] = pc.DurationMinutes
	}
	var billingMessenger telegram.Messenger
	if cfg.Billing.Telegram.Enabled && cfg.Billing.Telegram.BotToken != "" {
		baseOpt := telegram.WithPollTimeout(cfg.Billing.Telegram.PollTimeout)
		if cfg.Billing.Telegram.ChatID != 0 {
			baseOpt = func(b *telegram.Bot) {
				telegram.WithPollTimeout(cfg.Billing.Telegram.PollTimeout)(b)
				telegram.WithAllowChat(cfg.Billing.Telegram.ChatID)(b)
			}
		}
		billingMessenger = telegram.NewBot(cfg.Billing.Telegram.BotToken, baseOpt)
		sugar.Infow("Telegram billing bot configured", "enabled", true)
	}
	billingHandler := handler.NewBillingHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewPortalConfigRepository(db),
		database.NewPurchaseRequestRepository(db),
		database.NewVoucherRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		billingMessenger,
		planDurations,
		cfg.Billing.ReceiptDir,
	)

	// Guest wifi (Option C): the handler serves the WEB surface (guest purchase,
	// owner approve/reject, owner bot binding) which is ALWAYS available — shop
	// owners can run the whole flow from the web, no flag. The per-owner Telegram
	// bot bridge (the "second apple") is the ONLY piece gated on the flag below.
	// Two apples: web + bot both work; neither is forced on the user.
	guestSvcFmt := func(token string) (telegram.Messenger, bool) {
		return telegram.ConfigMessenger(token, "", 30*time.Second)
	}
	guestWifiHandler := handler.NewGuestWifiHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewBusinessTelegramBotRepository(db),
		database.NewGuestPurchaseRequestRepository(db),
		database.NewPortalConfigRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		cfg.GuestWifi.ReceiptDir,
		guestSvcFmt,
	)

	// Probe + monitoring: peer reachability classification and state fan-out.
	probeService := probe.NewProbeService(cacheClient, 5*time.Minute)
	monitoringService := monitoring.NewMonitoringService(
		database.NewPeerHeartbeatRepository(db),
		database.NewTelemetrySampleRepository(db),
		cacheClient,
		messagingClient,
		probeService,
	)
	monitorHandler := handler.NewMonitorHandler(db, database.NewDeviceRepository(db), monitoringService)

	// Fleet aggregate: read-only per-tenant peer-health overview.
	fleetHandler := handler.NewFleetHandler(fleet.NewFleetService(
		database.NewDeviceRepository(db),
		monitoringService,
	))

	// Topology: read-only per-tenant network map + explicit device actions
	// (reboot). Plan-gated; the reboot endpoint only queues an intent.
	topologyHandler := handler.NewTopologyHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		messagingClient,
	)

	// VPS: read-only WireGuard peer-provisioning (hub config, per-device peer,
	// reachability). Endpoint host comes from config; empty => omitted from JSON.
	vpsHandler := handler.NewVPSHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		monitoringService,
		cfg.WireGuard,
		cfg.VPS.EndpointHost,
	)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggerMiddleware(sugar))

	// Keystone: RLS enforcement is OFF by default. When on, protected requests run
	// on a restricted tenant connection bound to `authenticated`.
	var tenantMW gin.HandlerFunc
	var restrictedPool *database.DB
	if cfg.Database.RLSEnforce {
		rp, err := database.NewRestricted(cfg.Database)
		if err != nil {
			return nil, fmt.Errorf("restricted tenant pool: %w", err)
		}
		restrictedPool = rp
		tenantMW = auth.TenantConnMiddleware(rp)
		sugar.Warn("RLS enforcement ENABLED — protected requests use tenant-bound connections")
	} else {
		sugar.Info("RLS enforcement disabled — owner pool, no connection pinning (default)")
	}

	setupRoutes(r, authService, authHandler, routerHandler, telemetryHandler, incidentHandler, webhookHandler, voucherHandler, portalHandler, billingHandler, guestWifiHandler, monitorHandler, fleetHandler, topologyHandler, vpsHandler, messagingClient, cacheClient, db, tenantMW)

	// Start the policy evaluator: watches NATS telemetry and auto-opens incidents.
	evaluator := evaluator.New(
		messagingClient,
		database.NewAlertPolicyRepository(db),
		database.NewIncidentRepository(db),
		notifierr,
		sugar,
	)
	if err := evaluator.Start(); err != nil {
		return nil, fmt.Errorf("evaluator start: %w", err)
	}
	sugar.Info("Policy evaluator started")

	// Start the state sweep: periodically re-evaluates every peer's reachability
	// independently of any request, feeding stale/offline states to the evaluator.
	sweepWorker := monitoring.NewSweep(database.NewDeviceRepository(db), monitoringService, sugar, cfg.Monitoring.StateSweepInterval)
	sweepWorker.Start()
	sugar.Infow("State sweep started", "interval", cfg.Monitoring.StateSweepInterval)

	// Quota-ENFORCEMENT sweep: detects lapsed Sovereign passes and emits
	// quota.pass.expired. Started once here; it never touches HTTP.
	quotaSweep := quota.NewSweep(database.NewBusinessMeteredQuotaRepository(db), messagingClient, sugar, cfg.Billing.QuotaEnforcementInterval)
	quotaSweep.Start()
	sugar.Infow("Quota enforcement sweep started", "interval", cfg.Billing.QuotaEnforcementInterval)

	// AI/scanning service: consumes telemetry.ingest, meters AI tokens, emits
	// ai.finding. Construction is unconditional; the worker only subscribes when
	// ai.enabled=true (Start is a no-op otherwise).
	aiConfig := ai.Config{
		Enabled:          cfg.AI.Enabled,
		Mode:             cfg.AI.Mode,
		OllamaURL:        cfg.AI.OllamaURL,
		CostPerAnalysis:  cfg.AI.CostPerAnalysis,
		OpenRouterURL:    cfg.AI.OpenRouterURL,
		OpenRouterAPIKey: cfg.AI.OpenRouterAPIKey,
		OpenRouterModel:  cfg.AI.OpenRouterModel,
	}
	aiService := ai.New(messagingClient, database.NewBusinessMeteredQuotaRepository(db), db, ai.NewClient(aiConfig), aiConfig, sugar).
		WithNotifier(aiIncidentNotifier(cfg, notifierr, sugar))
	if err := aiService.Start(); err != nil {
		return nil, fmt.Errorf("ai service start: %w", err)
	}
	sugar.Infow("AI service wired", "enabled", cfg.AI.Enabled, "mode", cfg.AI.Mode)

	// Telegram operator purchase bridge: continuous inbound loop that answers
	// operator "approve <id>" / "reject <id>" replies against the shared money
	// core. Only starts when the billing telegram bot is configured.
	if bm, ok := billingMessenger.(*telegram.Bot); ok {
		target := ""
		if cfg.Billing.Telegram.ChatID != 0 {
			target = strconv.FormatInt(cfg.Billing.Telegram.ChatID, 10)
		}
		bridge := telegram.NewPurchaseBridge(bm, billingMessenger, target, billingHandler)
		go func() { _ = bridge.Run(context.Background()) }()
		sugar.Info("Telegram purchase approve/reject bridge started (inbound loop)")
	}

	// Guest wifi owner-bot supervisor (the "second apple"): one inbound loop per
	// ENABLED owner bot, re-scanned on the configured interval. THE ONLY guest-wifi
	// piece gated on the flag — the web surface above is always on. Off by default.
	if cfg.GuestWifi.Enabled {
		supervisor := NewGuestWifiBridgeSupervisor(
			db,
			database.NewBusinessTelegramBotRepository(db),
			guestWifiHandler,
			func(token string) (telegram.Messenger, bool) {
				return telegram.ConfigMessenger(token, "", 30*time.Second)
			},
		)
		_ = supervisor // retained: loop runs in background below
		go func() {
			supervisor.Bump() // immediate first scan
			for {
				time.Sleep(cfg.GuestWifi.BridgeScanInterval)
				supervisor.Bump()
			}
		}()
		sugar.Infow("Guest wifi owner-bot supervisor started", "interval", cfg.GuestWifi.BridgeScanInterval.String())
	}

	// Start the recovery worker: consumes peer.recover (from the evaluator),
	// runs the a/b/c self-healing ladder, emits recovery.result / escalate.
	recoveryService := recovery.New(messagingClient, cacheClient, monitoringService, cfg.Recovery, sugar)
	if err := recoveryService.Start(); err != nil {
		return nil, fmt.Errorf("recovery start: %w", err)
	}
	sugar.Infow("Recovery worker started", "enabled", cfg.Recovery.Enabled)

	// Wire the live provisioner apply path (Slice 1): flag-gated OFF by default.
	// When enabled, peer.recover intents reach the real RouterOS Applier through
	// the approve -> apply -> verify -> rollback gate. A failure here must fail
	// startup loudly.
	if err := startLiveApplier(cfg, messagingClient, sugar); err != nil {
		return nil, err
	}

	return &Server{
		cfg:   cfg,
		log:   log,
		sugar: sugar,
		gin:   r,
		http: &http.Server{
			Addr:           fmt.Sprintf(":%d", cfg.Server.Port),
			Handler:        r,
			ReadTimeout:    cfg.Server.ReadTimeout,
			WriteTimeout:   cfg.Server.WriteTimeout,
			MaxHeaderBytes: 1 << 20,
		},
		db:             db,
		cache:          cacheClient,
		messaging:      messagingClient,
		authService:    authService,
		userRepo:       userRepo,
		authHandler:    authHandler,
		evaluator:      evaluator,
		voucherHandler: voucherHandler,
		sweep:          sweepWorker,
		quotaSweep:     quotaSweep,
		recovery:       recoveryService,
		restricted:     restrictedPool,
	}, nil
}

func setupRoutes(r *gin.Engine, authService *auth.Service, authHandler *handler.AuthHandler, routerHandler *handler.RouterHandler, telemetryHandler *handler.TelemetryHandler, incidentHandler *handler.IncidentHandler, webhookHandler *handler.WebhookHandler, voucherHandler *handler.VoucherHandler, portalHandler *handler.PortalHandler, billingHandler *handler.BillingHandler, guestWifiHandler *handler.GuestWifiHandler, monitorHandler *handler.MonitorHandler, fleetHandler *handler.FleetHandler, topologyHandler *handler.TopologyHandler, vpsHandler *handler.VPSHandler, msg *messaging.Client, cache *cache.Client, db *database.DB, tenantMW gin.HandlerFunc) {
	r.GET("/", indexPage)
	r.GET("/health", healthCheck)
	r.GET("/ready", readinessCheck(&Server{cache: cache, db: db, messaging: msg}))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/version", version)

		// PUBLIC captive-portal endpoints — anonymous, NO auth middleware. The
		// tenant is resolved from the DEVICE the client is attached to (not from
		// the voucher code).
		portalPublic := v1.Group("/portal")
		{
			portalPublic.GET("/:deviceId/config", portalHandler.PublicConfig)
			portalPublic.POST("/:deviceId/redeem", portalHandler.PublicRedeem)
			portalPublic.GET("/status", portalHandler.PublicStatus)
			// Billing: guest submits a purchase request (device-anchored).
			portalPublic.POST("/:deviceId/purchase", billingHandler.PublicCreatePurchase)
			// Guest wifi (Option C): online-banking purchase with a free grace
			// window; the guest attaches the receipt within the window. Web path
			// is ALWAYS available (the Telegram bot is the separately-gated extra).
			portalPublic.POST("/:deviceId/guest-purchase", guestWifiHandler.PublicGuestPurchase)
			portalPublic.POST("/guest-purchase/:requestId/receipt", guestWifiHandler.PublicAttachReceipt)
		}

		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.RefreshToken)
		}

		protected := v1.Group("")
		protected.Use(auth.Middleware(authService))
		if tenantMW != nil {
			protected.Use(tenantMW)
		}
		{
			protected.GET("/me", authHandler.Me)

			routers := protected.Group("/routers")
			{
				routers.GET("", routerHandler.ListRouters)
				routers.GET("/:id", routerHandler.GetRouter)
				routers.POST("", routerHandler.CreateRouter)
				routers.PUT("/:id", routerHandler.UpdateRouter)
				routers.DELETE("/:id", routerHandler.DeleteRouter)
			}

			telemetry := protected.Group("/telemetry")
			{
				telemetry.POST("", telemetryHandler.Ingest)
				telemetry.GET("", telemetryHandler.List)
				telemetry.GET("/latest/:deviceId", telemetryHandler.Latest)
			}

			incidents := protected.Group("/incidents")
			{
				incidents.GET("", incidentHandler.ListOpen)
				incidents.GET("/device/:deviceId", incidentHandler.ListByDevice)
				incidents.POST("", incidentHandler.Create)
				incidents.POST("/:id/acknowledge", incidentHandler.Acknowledge)
				incidents.POST("/:id/resolve", incidentHandler.Resolve)
			}

			peers := protected.Group("/peers")
			{
				peers.GET("/:peerId/state", monitorHandler.PeerState)
			}

			// Read-only per-tenant fleet aggregate (peer-health overview).
			protected.GET("/fleet/health", fleetHandler.FleetHealth)
			protected.GET("/fleet/health/offline", fleetHandler.OfflineHealth)

			// Read-only per-tenant network topology map + explicit device action
			// (reboot, plan-gated — queues an intent for the flag-gated consumer).
			protected.GET("/topology/map", topologyHandler.Map)
			protected.POST("/topology/:deviceId/reboot", topologyHandler.Reboot)

			// VPS: read-only WireGuard peer-provisioning (plan-gated).
			vpsg := protected.Group("/vps")
			{
				vpsg.GET("/config", vpsHandler.HubConfig)
				vpsg.GET("/peers/:deviceId", vpsHandler.Peer)
				vpsg.GET("/peers/:deviceId/health", vpsHandler.Health)
			}

			policies := protected.Group("/alert-policies")
			{
				policies.GET("", incidentHandler.ListPolicies)
				policies.POST("", incidentHandler.CreatePolicy)
				policies.PUT("/:id", incidentHandler.UpdatePolicy)
				policies.DELETE("/:id", incidentHandler.DeletePolicy)
			}

			vouchers := protected.Group("/vouchers")
			{
				vouchers.GET("", voucherHandler.ListVouchers)
				vouchers.POST("", voucherHandler.CreateVoucher)
				vouchers.GET("/:id", voucherHandler.GetVoucherByCode) // Note: reused handler for redemption
				vouchers.PUT("/:id", voucherHandler.UpdateVoucherStatus)
				vouchers.DELETE("/:id", voucherHandler.DeleteVoucher)
				vouchers.POST("/:code/redeem", voucherHandler.RedeemVoucher) // For voucher redemption
			}

			portalStaff := protected.Group("/portal")
			{
				portalStaff.GET("/config", portalHandler.StaffGetConfig)
				portalStaff.PUT("/config", portalHandler.StaffUpdateConfig)
			}

			billing := protected.Group("/billing")
			{
				billing.GET("/requests", billingHandler.ListRequests)
				billing.POST("/approve/:requestId", billingHandler.ApprovePurchase)
				billing.POST("/reject/:requestId", billingHandler.RejectPurchase)
			}

			// Guest wifi (Option C): owner-facing WEB loop — approve/reject, list,
			// and bind their own bot. Always on (web is the always-available apple;
			// the Telegram bot bridge uses the same approve core, gated separately).
			gw := protected.Group("/guest-wifi")
			{
				gw.GET("/telegram-bot", guestWifiHandler.GetBotBinding)
				gw.PUT("/telegram-bot", guestWifiHandler.PutBotBinding)
				gw.POST("/telegram-bot/test", guestWifiHandler.TestBotBinding)
				gw.GET("/requests", guestWifiHandler.ListGuestRequests)
				gw.POST("/approve/:requestId", guestWifiHandler.ApproveGuest)
				gw.POST("/reject/:requestId", guestWifiHandler.RejectGuest)
			}

			webhooks := protected.Group("/webhooks")
			{
				webhooks.GET("", webhookHandler.List)
				webhooks.POST("", webhookHandler.Create)
				webhooks.PUT("/:id", webhookHandler.Update)
				webhooks.DELETE("/:id", webhookHandler.Delete)
				webhooks.GET("/deliveries", webhookHandler.ListDeliveries)
			}
		}
	}
}

// indexPage gives the bare domain root a friendly landing: a human-readable
// pointer that this is the Mega Magic API and where the health/version
// endpoints live. Without it a browser hitting `mikromagic.com/` gets a 404.
func indexPage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":     "mega-magic",
		"status":      "ok",
		"description": "Mega Magic API — mesh hardening, provisioning, recovery, billing and guest-wifi platform",
		"info": gin.H{
			"health":   "/health",
			"ready":    "/ready",
			"version":  "/api/v1/version",
			"api_base": "/api/v1",
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "mega-magic",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func readinessCheck(s *Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbOK := "ok"
		redisOK := "ok"
		natsOK := "ok"

		health := s.db.Ping()
		if health != nil {
			dbOK = fmt.Sprintf("error: %v", health)
		}

		if s.cache != nil && !s.cache.IsConnected() {
			redisOK = "disconnected"
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"checks": gin.H{
				"database": dbOK,
				"redis":    redisOK,
				"nats":     natsOK,
			},
		})
	}
}

func version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":   "0.1.0-dev",
		"build":     "development",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) Start() error {
	s.sugar.Infow("Starting HTTP server", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.sugar.Info("Shutting down HTTP server")
	if s.restricted != nil {
		_ = s.restricted.Close()
	}
	if s.messaging != nil {
		s.messaging.Close()
	}
	if s.cache != nil {
		if err := s.cache.Close(); err != nil {
			s.sugar.Warnw("Error closing cache", "error", err)
		}
	}
	return s.http.Shutdown(ctx)
}

func loggerMiddleware(log *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		log.Infow("HTTP request",
			"method", method,
			"path", path,
			"query", query,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", clientIP,
			"user_agent", c.Request.UserAgent(),
		)
	}
}

// aiIncidentNotifier assembles the AI incident notifier: the existing webhook
// notifier PLUS an additive Telegram operator DM when explicitly configured
// (Note-1 compose — webhooks are never replaced). A mis-setup (enabled but empty
// bot_token/chat_id, or an unbuildable messenger) degrades to webhooks-only with
// a single warning. Default-off: with the flag unset this returns the webhook
// notifier unchanged.
func aiIncidentNotifier(cfg *config.Config, webhooks ai.IncidentNotifier, sugar *zap.SugaredLogger) ai.IncidentNotifier {
	if !cfg.AI.NotifyTelegramEnabled {
		return webhooks
	}
	if cfg.AI.NotifyTelegramBotToken == "" || cfg.AI.NotifyTelegramChatID == 0 {
		sugar.Warn("AI telegram notify enabled but bot_token/chat_id missing — degrading to webhooks only")
		return webhooks
	}
	target := strconv.FormatInt(cfg.AI.NotifyTelegramChatID, 10)
	msg, ok := telegram.ConfigMessenger(cfg.AI.NotifyTelegramBotToken, target, 30*time.Second)
	if !ok {
		sugar.Warn("AI telegram notify enabled but messenger not configured — degrading to webhooks only")
		return webhooks
	}
	sugar.Infow("AI incident Telegram notification configured", "enabled", true)
	return ai.NewFanout(webhooks, ai.NewTelegramNotifier(msg, target, sugar))
}
