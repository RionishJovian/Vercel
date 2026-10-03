package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	"mega-magic/internal/hub"
	"mega-magic/internal/messaging"
	"mega-magic/internal/model"
	"mega-magic/internal/monitoring"
	"mega-magic/internal/notifier"
	"mega-magic/internal/probe"
	"mega-magic/internal/quota"
	"mega-magic/internal/recovery"
	"mega-magic/internal/telegram"
	"mega-magic/pkg/logger"
)

type Server struct {
	cfg               *config.Config
	log               *logger.Logger
	sugar             *zap.SugaredLogger
	http              *http.Server
	gin               *gin.Engine
	db                *database.DB
	cache             *cache.Client
	messaging         *messaging.Client
	authService       *auth.Service
	userRepo          *database.UserRepository
	authHandler       *handler.AuthHandler
	evaluator         *evaluator.Evaluator
	voucherHandler    *handler.VoucherHandler
	monitorHandler    *handler.MonitorHandler
	sweep             *monitoring.Sweep
	quotaSweep        *quota.Sweep
	recovery          *recovery.RecoveryService
	restricted        *database.DB
	guestRepairMu     sync.Mutex
	guestRepairCancel context.CancelFunc
	guestRepairDone   chan struct{}
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
	if err := database.CheckAuthSessionSchema(db); err != nil {
		return nil, err
	}
	authService.UseSessions(db)
	sugar.Info("Auth service initialized")

	userRepo := database.NewUserRepository(db)

	// Notifier fans incident events out to tenant webhook endpoints.
	notifierr := notifier.New(database.NewWebhookRepository(db), db, sugar)

	authHandler := handler.NewAuthHandler(userRepo, authService, db)
	allowRepo := database.NewDeviceAllowanceRepository(db)
	routerHandler := handler.NewRouterHandler(
		db,
		database.NewDeviceRepository(db),
		allowRepo,
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
	// Platform (DEVELOPER) cross-tenant window — the platform operator's lens.
	platformHandler := handler.NewPlatformHandler(
		database.NewPlatformBusinessRepository(db),
		database.NewPlatformIncidentRepository(db),
		db,
		authService,
	)
	// Voucher management for vistors (revenue features)
	voucherHandler := handler.NewVoucherHandler(db, database.NewVoucherRepository(db), database.NewBusinessMeteredQuotaRepository(db))
	// Magic Coins — per-user balance + immutable ledger (ported from Mikromagic).
	magicCoinHandler := handler.NewMagicCoinHandler(db, database.NewMagicCoinRepository(db))
	// Sites — physical locations with the ported geo data (location/timezone/lat/lng).
	siteHandler := handler.NewSiteHandler(db, database.NewSiteRepository(db), allowRepo)
	// Router connections — saved management endpoints ported from Mikromagic.
	connHandler := handler.NewRouterConnectionHandler(db, database.NewRouterConnectionRepository(db))
	// Device allowances + requests — granted capacity and its approval workflow.
	allowHandler := handler.NewDeviceAllowanceHandler(db, allowRepo)
	// Syslog — the ported router event log (read-only, paged).
	syslogHandler := handler.NewSyslogHandler(db, database.NewSyslogRepository(db))
	// Terminal templates — the saved RouterOS REST call library.
	templateHandler := handler.NewTerminalTemplateHandler(db, database.NewTerminalTemplateRepository(db))
	// Voucher resellers + printed-voucher layout (ported from Mikromagic).
	resellerHandler := handler.NewVoucherResellerHandler(db, database.NewVoucherResellerRepository(db))
	// Fleet scans — the ported AI scan history (read-only).
	scanHandler := handler.NewFleetScanRunHandler(db, database.NewFleetScanRunRepository(db))
	// AI usage — token history behind the AI features (read-only).
	usageRepo := database.NewAIUsageRepository(db)
	usageHandler := handler.NewAIUsageHandler(db, usageRepo)
	// Magic Dude — a read-only tenant-aware operations assistant. It intentionally
	// receives no messaging or provisioner dependency, so chat can never mutate a router.
	var magicDudeClient ai.MagicDudeClient = ai.NewOpenRouterClient(cfg.AI.OpenRouterURL, cfg.AI.OpenRouterAPIKey, cfg.AI.OpenRouterModel)
	if cfg.AI.MagicDudeProvider == "gemini" {
		magicDudeClient = ai.NewGeminiChatClient(cfg.AI.GeminiAPIKey, cfg.AI.GeminiModel)
	}
	magicDudeHandler := handler.NewMagicDudeHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewIncidentRepository(db),
		usageRepo,
		magicDudeClient,
	)
	// Terminal history — commands run against routers (read-only).
	terminalHistoryHandler := handler.NewTerminalHistoryHandler(db, database.NewTerminalHistoryRepository(db))
	// Unlock keys — the Magic Coin purchase ledger (read-only).
	unlockKeyHandler := handler.NewUnlockKeyHandler(db, database.NewUnlockKeyRepository(db))
	// Translation overrides — a GLOBAL table, so reads are open and writes are
	// platform-only (see the route block for why).
	i18nHandler := handler.NewI18nHandler(db, database.NewI18nRepository(db))
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
	operatorChat := ""
	if cfg.Billing.Telegram.ChatID != 0 {
		operatorChat = strconv.FormatInt(cfg.Billing.Telegram.ChatID, 10)
	}
	storeHandler := handler.NewStoreHandler(
		db,
		database.NewStorePurchaseRepository(db),
		database.NewPricingRepository(db),
		billingMessenger,
		operatorChat,
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
	if err := guestWifiHandler.EnableGuestCookies(cfg.Security.JWTSecret); err != nil {
		return nil, err
	}

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

	if cfg.VPS.HubLookupEnabled {
		client, err := hub.New(hub.Options{URL: cfg.VPS.HubURL, KeyID: cfg.VPS.HubKeyID, SigningKey: cfg.VPS.HubSigningKey})
		if err != nil {
			return nil, fmt.Errorf("invalid Magic Hub lookup configuration")
		}
		vpsHandler.ConfigureHubLookup(client)
		if cfg.VPS.HubEnrollmentEnabled {
			vpsHandler.ConfigureHubEnrollment(client)
			if err := configureHubManagement(vpsHandler, db, cfg); err != nil {
				return nil, err
			}
		}
	}
	r := gin.New()
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		return nil, err
	}
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

	securityHandler := handler.NewSecurityHandler(db, database.NewDeviceRepository(db))
	backupHandler := handler.NewBackupHandler(db, database.NewDeviceRepository(db))
	teamHandler := handler.NewTeamHandler(db, database.NewUserRepository(db), authService)
	pulseHandler := handler.NewPulseHandler(db, database.NewDeviceRepository(db), database.NewIncidentRepository(db))
	incidentActionHandler := handler.NewIncidentActionHandler(db, database.NewIncidentRepository(db), database.NewDeviceRepository(db), messagingClient)
	if cfg.Billing.SubscriptionOnly {
		if err := database.CheckSubscriptionSchema(db); err != nil {
			return nil, fmt.Errorf("subscription schema must be explicitly migrated before enabling: %w", err)
		}
	}
	configureSubscriptionHandlers(cfg.Billing, authHandler, routerHandler, siteHandler, storeHandler, topologyHandler, vpsHandler, magicDudeHandler)

	setupRoutes(r, authService, authHandler, routerHandler, telemetryHandler, incidentHandler, webhookHandler, voucherHandler, portalHandler, billingHandler, storeHandler, guestWifiHandler, monitorHandler, fleetHandler, topologyHandler, vpsHandler, platformHandler, securityHandler, backupHandler, teamHandler, pulseHandler, incidentActionHandler, magicCoinHandler, siteHandler, connHandler, allowHandler, syslogHandler, templateHandler, resellerHandler, scanHandler, usageHandler, magicDudeHandler, terminalHistoryHandler, unlockKeyHandler, i18nHandler, messagingClient, cacheClient, db, tenantMW)
	if err := registerGuestVoucherRoutes(r, cfg, db, authService, voucherHandler); err != nil {
		return nil, fmt.Errorf("guest voucher routes: %w", err)
	}
	portalHandler.GuestVoucherAccess = cfg.GuestWifi.VoucherAccessEnabled

	// Start the policy evaluator: watches NATS telemetry and auto-opens incidents.
	if cfg.Media.ConnectorEnabled {
		if err := registerMediaRoutes(r, authService, db, handler.SubscriptionAccess(db, cfg.Billing.SubscriptionOnly, cfg.Billing.ExistingAccountPolicy)); err != nil {
			return nil, fmt.Errorf("media connector schema/routes: %w", err)
		}
	}

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
	var quotaSweep *quota.Sweep
	if !cfg.Billing.SubscriptionOnly {
		quotaSweep = quota.NewSweep(database.NewBusinessMeteredQuotaRepository(db), messagingClient, sugar, cfg.Billing.QuotaEnforcementInterval)
		quotaSweep.Start()
		sugar.Infow("Quota enforcement sweep started", "interval", cfg.Billing.QuotaEnforcementInterval)
	}

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
		decider := telegram.CompositePurchaseDecider{Deciders: []telegram.PurchaseDecider{storeHandler, billingHandler}}
		bridge := telegram.NewPurchaseBridge(bm, billingMessenger, target, decider)
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
	if err := startLiveApplier(cfg, messagingClient, sugar, monitoringService, db, backupHandler, securityHandler, incidentActionHandler, guestWifiHandler); err != nil {
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

func setupRoutes(r *gin.Engine, authService *auth.Service, authHandler *handler.AuthHandler, routerHandler *handler.RouterHandler, telemetryHandler *handler.TelemetryHandler, incidentHandler *handler.IncidentHandler, webhookHandler *handler.WebhookHandler, voucherHandler *handler.VoucherHandler, portalHandler *handler.PortalHandler, billingHandler *handler.BillingHandler, storeHandler *handler.StoreHandler, guestWifiHandler *handler.GuestWifiHandler, monitorHandler *handler.MonitorHandler, fleetHandler *handler.FleetHandler, topologyHandler *handler.TopologyHandler, vpsHandler *handler.VPSHandler, platformHandler *handler.PlatformHandler, securityHandler *handler.SecurityHandler, backupHandler *handler.BackupHandler, teamHandler *handler.TeamHandler, pulseHandler *handler.PulseHandler, incidentActionHandler *handler.IncidentActionHandler, magicCoinHandler *handler.MagicCoinHandler, siteHandler *handler.SiteHandler, connHandler *handler.RouterConnectionHandler, allowHandler *handler.DeviceAllowanceHandler, syslogHandler *handler.SyslogHandler, templateHandler *handler.TerminalTemplateHandler, resellerHandler *handler.VoucherResellerHandler, scanHandler *handler.FleetScanRunHandler, usageHandler *handler.AIUsageHandler, magicDudeHandler *handler.MagicDudeHandler, historyHandler *handler.TerminalHistoryHandler, unlockHandler *handler.UnlockKeyHandler, i18nH *handler.I18nHandler, msg *messaging.Client, cache *cache.Client, db *database.DB, tenantMW gin.HandlerFunc) {
	r.GET("/", indexPage)
	r.GET("/health", healthCheck)
	r.GET("/ready", readinessCheck(&Server{cache: cache, db: db, messaging: msg}))

	tv1 := r.Group("/api/v1")
	{
		tv1.GET("/version", version)

		// Public price list — anonymous, NO auth. pricing_promo is a
		// platform-global table (no business_id, RLS disabled) holding published
		// prices and capacity tiers: the same class of non-tenant information as
		// /version, and deliberately public because it backs the pricing page.
		tv1.GET("/pricing", handler.NewPricingHandler(db).Get)

		// PUBLIC captive-portal endpoints — anonymous, NO auth middleware. The
		// tenant is resolved from the DEVICE the client is attached to (not from
		// the voucher code).
		portalPublic := tv1.Group("/portal", auth.BoundedPortalRateLimit(), boundedPortalBody())
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

		authGroup := tv1.Group("/auth", auth.CookieOrigin(), auth.BoundedAuthRateLimit())
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.RefreshToken)
			authGroup.POST("/logout", authHandler.Logout)
		}

		protected := tv1.Group("")
		protected.Use(auth.CookieOrigin(), auth.Middleware(authService))
		// Runs immediately after the token is resolved, so a suspended tenant is
		// refused even with a token minted before the suspension. Placed before the
		// role gate so suspension outranks role: an OWNER of a suspended business
		// is refused just like a VIEWER.
		protected.Use(auth.RequireActiveTenant(db))
		if tenantMW != nil {
			protected.Use(tenantMW)
		}
		// C3 baseline: every tenant route requires at least VIEWER in the role
		// hierarchy (unknown roles are refused). Stricter per-group minimums below.
		protected.Use(auth.RequireMinRole(model.RoleViewer))
		protected.Use(handler.SubscriptionAccess(db, authHandler.SubscriptionOnly, authHandler.SubscriptionExistingPolicy))
		{
			protected.GET("/subscription", handler.SubscriptionStatus)
			protected.GET("/me", authHandler.Me)
			protected.GET("/me/profile", authHandler.Profile)
			protected.PUT("/me/profile", authHandler.UpdateProfile)
			protected.POST("/me/password", authHandler.ChangePassword)
			protected.GET("/me/profile/:kind", authHandler.ProfileAsset)
			protected.POST("/me/profile/:kind", authHandler.PutProfileAsset)
			protected.DELETE("/me/profile/:kind", authHandler.DeleteProfileAsset)

			// Backward-compatible CoreBit/The Dude route. It deliberately reuses
			// the real tenant-scoped topology handler instead of returning a static
			// success marker. Registering it inside `protected` ensures auth, tenant
			// suspension, role checks and optional RLS connection pinning all apply.
			registerGeoTopologyRoute(protected, topologyHandler.Map)

			routers := protected.Group("/routers")
			{
				routers.GET("", routerHandler.ListRouters)
				routers.GET("/:id/capabilities", securityHandler.RouterCapabilities)
				routers.GET("/:id/quick-config", securityHandler.ReadQuickConfig)
				routers.PUT("/:id/quick-config", auth.RequireMinRole(model.RoleAdmin), securityHandler.ApplyQuickConfig)
				routers.POST("/:id/gateway/:action", auth.RequireMinRole(model.RoleAdmin), securityHandler.GatewaySetup)
				routers.GET("/:id/gateway/interfaces", securityHandler.GatewaySetup)
				routers.GET("/:id/portal/profiles", auth.RequireMinRole(model.RoleAdmin), securityHandler.PortalDeployment)
				routers.POST("/:id/portal/:action", auth.RequireMinRole(model.RoleAdmin), securityHandler.PortalDeployment)
				routers.POST("/:id/webfig/secure", auth.RequireMinRole(model.RoleAdmin), securityHandler.ConfigureWebFig)
				routers.GET("/:id", routerHandler.GetRouter)
				// Router write is OWNER/ADMIN (C3 hierarchy; explicit per RM).
				routers.POST("", auth.RequireMinRole(model.RoleAdmin), routerHandler.CreateRouter)
				routers.PUT("/:id", auth.RequireMinRole(model.RoleAdmin), routerHandler.UpdateRouter)
				routers.DELETE("/:id", auth.RequireMinRole(model.RoleAdmin), routerHandler.DeleteRouter)
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
				// Stage 1A: apply an incident's suggested_action (high-risk => ADMIN).
				incidents.POST("/:id/action", auth.RequireMinRole(model.RoleAdmin), incidentActionHandler.Apply)
			}

			peers := protected.Group("/peers")
			{
				peers.GET("/:peerId/state", monitorHandler.PeerState)
			}

			// Read-only per-tenant fleet aggregate (peer-health overview).
			protected.GET("/fleet/health", fleetHandler.FleetHealth)
			protected.GET("/fleet/health/offline", fleetHandler.OfflineHealth)
			protected.POST("/devices/:id/security/verify", securityHandler.VerifyDevice)
			protected.POST("/devices/:id/security/harden", auth.RequireMinRole(model.RoleAdmin), securityHandler.HardenManagement)

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
				vpsg.GET("/peers/:deviceId/live", auth.RequireMinRole(model.RoleAdmin), vpsHandler.LivePeer)
				vpsg.POST("/peers/:deviceId/enroll", auth.RequireMinRole(model.RoleAdmin), vpsHandler.EnrollGateway)
				vpsg.POST("/peers/:deviceId/quick-enroll", auth.RequireMinRole(model.RoleAdmin), vpsHandler.QuickEnrollGateway)
				vpsg.POST("/peers/:deviceId/finish", auth.RequireMinRole(model.RoleAdmin), vpsHandler.FinishGatewayConnection)
				vpsg.GET("/peers/:deviceId/bootstrap", auth.RequireMinRole(model.RoleAdmin), vpsHandler.GatewayBootstrap)
				vpsg.POST("/peers/:deviceId/authenticate", auth.RequireMinRole(model.RoleAdmin), vpsHandler.AuthenticateGateway)
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
				vouchers.POST("", auth.RequireMinRole(model.RoleOperator), voucherHandler.CreateVoucher)
				vouchers.GET("/:code", voucherHandler.GetVoucherByCode)
				vouchers.PUT("/:id", auth.RequireMinRole(model.RoleOperator), voucherHandler.UpdateVoucherStatus)
				vouchers.DELETE("/:id", auth.RequireMinRole(model.RoleAdmin), voucherHandler.DeleteVoucher)
				vouchers.POST("/:code/redeem", voucherHandler.RedeemVoucher) // For voucher redemption
			}

			coins := protected.Group("/magic-coins")
			{
				coins.GET("/wallet", magicCoinHandler.GetWallet)
				coins.GET("/transactions", magicCoinHandler.ListTransactions)
				// Manual adjustments move money, so they are ADMIN+ only.
				coins.POST("/adjust", auth.RequireMinRole(model.RoleAdmin), magicCoinHandler.Adjust)
			}

			storePurchases := protected.Group("/store/purchases")
			{
				storePurchases.GET("", storeHandler.List)
				storePurchases.POST("", storeHandler.Create)
				storePurchases.POST("/:purchaseId/approve", auth.RequireMinRole(model.RoleOperator), storeHandler.Approve)
				storePurchases.POST("/:purchaseId/reject", auth.RequireMinRole(model.RoleOperator), storeHandler.Reject)
			}
			protected.GET("/store/checkout-options", storeHandler.CheckoutOptions)

			sites := protected.Group("/sites")
			{
				sites.GET("", siteHandler.ListSites)
				// Editing a site moves routers between locations, so ADMIN+ only.
				sites.POST("", auth.RequireMinRole(model.RoleAdmin), siteHandler.CreateSite)
				sites.PUT("/:id", auth.RequireMinRole(model.RoleAdmin), siteHandler.UpdateSite)
				sites.DELETE("/:id", auth.RequireMinRole(model.RoleAdmin), siteHandler.DeleteSite)
			}

			conns := protected.Group("/router-connections")
			{
				conns.GET("", connHandler.ListRouterConnections)
				// A connection is a management credential path into the fleet, so ADMIN+ only.
				conns.POST("", auth.RequireMinRole(model.RoleAdmin), connHandler.CreateRouterConnection)
				conns.PUT("/:id", auth.RequireMinRole(model.RoleAdmin), connHandler.UpdateRouterConnection)
				conns.DELETE("/:id", auth.RequireMinRole(model.RoleAdmin), connHandler.DeleteRouterConnection)
			}

			// Capacity: reading the allowance is fine for anyone; asking for more
			// is not privileged, but DECIDING is.
			protected.GET("/device-allowances", allowHandler.GetAllowance)
			requests := protected.Group("/device-requests")
			{
				requests.GET("", allowHandler.ListRequests)
				requests.POST("", allowHandler.CreateRequest)
				// Capacity is a platform grant. A tenant owner must never be able to
				// approve their own request and mint capacity.
				requests.POST("/:id/decide", auth.RequireRole(string(model.RoleDeveloper)), allowHandler.DecideRequest)
			}

			// Router event log — read-only, so any authenticated role may browse it.
			protected.GET("/syslog-events", syslogHandler.ListEvents)

			templates := protected.Group("/terminal-templates")
			{
				templates.GET("", templateHandler.ListTemplates)
				// The library is shipped content; only ADMIN+ may add or edit, and
				// built-ins are refused outright by the repo.
				templates.POST("", auth.RequireMinRole(model.RoleAdmin), templateHandler.CreateTemplate)
				templates.PUT("/:id", auth.RequireMinRole(model.RoleAdmin), templateHandler.UpdateTemplate)
				templates.DELETE("/:id", auth.RequireMinRole(model.RoleAdmin), templateHandler.DeleteTemplate)
			}

			// Voucher resellers — deliberately a TOP-LEVEL group, not
			// /vouchers/resellers: gin's router panics when a static segment
			// collides with the existing /vouchers/:id wildcard.
			resellers := protected.Group("/voucher-resellers")
			{
				resellers.GET("", resellerHandler.ListResellers)
				resellers.POST("", auth.RequireMinRole(model.RoleAdmin), resellerHandler.CreateReseller)
				resellers.PUT("/:id", auth.RequireMinRole(model.RoleAdmin), resellerHandler.UpdateReseller)
				resellers.DELETE("/:id", auth.RequireMinRole(model.RoleAdmin), resellerHandler.DeleteReseller)
			}

			// Printed-voucher layout — one row per business (business_id IS the PK).
			layout := protected.Group("/voucher-print-layout")
			{
				layout.GET("", resellerHandler.GetPrintLayout)
				layout.PUT("", auth.RequireMinRole(model.RoleAdmin), resellerHandler.SavePrintLayout)
			}

			// Fleet scans — generated by the scan engine, so READ-ONLY for tenants.
			scans := protected.Group("/fleet-scan-runs")
			{
				scans.GET("", scanHandler.ListScanRuns)
				scans.GET("/:id", scanHandler.GetScanRun)
			}

			// AI usage — written by the AI features themselves, so READ-ONLY too.
			protected.GET("/ai-usage-events", usageHandler.ListUsageEvents)

			// Magic Dude is diagnostic only. The handler has no router-control
			// dependency and enforces included-role / active-key access itself.
			protected.POST("/magic-dude/chat", magicDudeHandler.Chat)

			// Terminal history — an audit log of commands, so READ-ONLY as well.
			protected.GET("/terminal-history", historyHandler.ListTerminalHistory)

			// Unlock keys — created by the coin purchase flow, so READ-ONLY.
			protected.GET("/unlock-keys", unlockHandler.ListUnlockKeys)

			// Translation overrides. `i18n_overrides` has NO business_id and RLS is
			// DISABLED on it, so every business shares these rows. Reading them is
			// harmless, but a tenant admin writing here would rewrite the UI for all
			// other tenants — so writes carry the exact DEVELOPER gate, matching the
			// /platform group, not RequireMinRole(ADMIN).
			protected.GET("/i18n-overrides", i18nH.ListOverrides)
			protected.POST("/i18n-overrides",
				auth.RequireRole(string(model.RoleDeveloper)), i18nH.CreateOverride)
			protected.PUT("/i18n-overrides/:id",
				auth.RequireRole(string(model.RoleDeveloper)), i18nH.UpdateOverride)
			protected.DELETE("/i18n-overrides/:id",
				auth.RequireRole(string(model.RoleDeveloper)), i18nH.DeleteOverride)

			portalStaff := protected.Group("/portal")
			{
				portalStaff.GET("/config", portalHandler.StaffGetConfig)
				portalStaff.PUT("/config", auth.RequireMinRole(model.RoleAdmin), portalHandler.StaffUpdateConfig)
			}

			billing := protected.Group("/billing", auth.RequireMinRole(model.RoleOperator))
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
				gw.PUT("/telegram-bot", auth.RequireMinRole(model.RoleAdmin), guestWifiHandler.PutBotBinding)
				gw.POST("/telegram-bot/test", auth.RequireMinRole(model.RoleAdmin), guestWifiHandler.TestBotBinding)
				gw.GET("/requests", guestWifiHandler.ListGuestRequests)
				gw.GET("/revenue", guestWifiHandler.GuestRevenue)
				gw.POST("/approve/:requestId", auth.RequireMinRole(model.RoleOperator), guestWifiHandler.ApproveGuest)
				gw.POST("/reject/:requestId", auth.RequireMinRole(model.RoleOperator), guestWifiHandler.RejectGuest)
			}

			webhooks := protected.Group("/webhooks", auth.RequireMinRole(model.RoleAdmin))
			{
				webhooks.GET("", webhookHandler.List)
				webhooks.POST("", webhookHandler.Create)
				webhooks.PUT("/:id", webhookHandler.Update)
				webhooks.DELETE("/:id", webhookHandler.Delete)
				webhooks.GET("/deliveries", webhookHandler.ListDeliveries)
			}

			// Security posture (C1): tenant-scoped RouterOS version audit.
			security := protected.Group("/security")
			{
				security.GET("/posture", securityHandler.FleetPosture)
			}
			protected.GET("/devices/:id/security", securityHandler.DeviceSecurity)

			// Fleet pulse (Stage 1B): one tenant-scoped aggregate for the day view.
			protected.GET("/fleet/pulse", pulseHandler.Pulse)

			// Config backups + change-history timeline + one-click restore (C2).
			protected.GET("/devices/:id/backups", backupHandler.ListBackups)
			protected.POST("/devices/:id/backups", auth.RequireMinRole(model.RoleAdmin), backupHandler.CreateBackup)
			protected.DELETE("/devices/:id/backups/:backupId", auth.RequireMinRole(model.RoleAdmin), backupHandler.DeleteBackup)
			protected.GET("/devices/:id/changes", backupHandler.Changes)
			protected.POST("/devices/:id/restore/:backupId", auth.RequireMinRole(model.RoleAdmin), backupHandler.Restore)

			// Team management (C3): OWNER-only role grants, tenant-scoped.
			team := protected.Group("/team", auth.RequireMinRole(model.RoleOwner))
			{
				team.GET("", teamHandler.List)
				team.POST("", teamHandler.Create)
				team.PUT("/:userId", teamHandler.Update)
				team.DELETE("/:userId", teamHandler.Delete)
			}
		}

		// Platform (DEVELOPER) cross-tenant window. SYSTEM-LEVEL reads that see
		// EVERY business — this is the platform operator's lens, not a tenant
		// view. auth.Middleware FIRST (validates JWT, sets role context) then
		// RequireRole(DEVELOPER) so no customer role (OWNER, VIEWER, etc.) can
		// reach it. Deliberately does NOT use the tenant-scoped protected group
		// or tenantMW.
		platform := tv1.Group("/platform", auth.CookieOrigin(), auth.Middleware(authService), auth.RequireRole(string(model.RoleDeveloper)))
		{
			platform.GET("/businesses", platformHandler.ListBusinesses)
			platform.GET("/incidents", platformHandler.ListOpenIncidents)
			platform.GET("/incidents/counts", platformHandler.CountIncidents)
			// Admin console (tenant management). Same strongest gate as the group:
			// RequireRole(DEVELOPER) — NO customer role reaches these.
			platform.POST("/businesses", platformHandler.CreateBusiness)
			platform.POST("/businesses/:businessId/suspend", platformHandler.SuspendBusiness)
			platform.POST("/businesses/:businessId/unsuspend", platformHandler.UnsuspendBusiness)
			platform.POST("/businesses/:businessId/reset-owner-password", platformHandler.ResetOwnerPassword)
			platform.GET("/businesses/:businessId/state", platformHandler.TenantState)
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
	worker, err := s.buildGuestRepairWorker()
	if err != nil {
		return fmt.Errorf("guest recovery worker: %w", err)
	}
	if worker != nil {
		ctx, cancel := context.WithCancel(context.Background())
		s.guestRepairMu.Lock()
		if s.guestRepairCancel != nil {
			s.guestRepairMu.Unlock()
			cancel()
			return fmt.Errorf("guest recovery worker already started")
		}
		s.guestRepairCancel = cancel
		done := make(chan struct{})
		s.guestRepairDone = done
		s.guestRepairMu.Unlock()
		go func() { defer close(done); worker(ctx) }()
		defer cancel()
	}
	s.sugar.Infow("Starting HTTP server", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.sugar.Info("Shutting down HTTP server")
	s.guestRepairMu.Lock()
	cancel, done := s.guestRepairCancel, s.guestRepairDone
	s.guestRepairMu.Unlock()
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

// registerGeoTopologyRoute keeps the legacy CoreBit/The Dude URL as an alias
// of the real topology handler. The caller must pass the protected tenant route
// group; keeping registration in this helper gives the auth contract a focused
// regression test without constructing the full server dependency graph.
func registerGeoTopologyRoute(r gin.IRoutes, topologyMap gin.HandlerFunc) {
	r.GET("/geo/topology-canvas", topologyMap)
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
