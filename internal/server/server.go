package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"mega-magic/internal/auth"
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
	"mega-magic/internal/recovery"
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
	notifierr := notifier.New(database.NewWebhookRepository(db), sugar)

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
	webhookHandler := handler.NewWebhookHandler(database.NewWebhookRepository(db))
	// Voucher management for vistors (revenue features)
	voucherHandler := handler.NewVoucherHandler(db, database.NewVoucherRepository(db), database.NewBusinessMeteredQuotaRepository(db))

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

	setupRoutes(r, authService, authHandler, routerHandler, telemetryHandler, incidentHandler, webhookHandler, voucherHandler, monitorHandler, fleetHandler, messagingClient, cacheClient, db, tenantMW)

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
		recovery:       recoveryService,
		restricted:     restrictedPool,
	}, nil
}

func setupRoutes(r *gin.Engine, authService *auth.Service, authHandler *handler.AuthHandler, routerHandler *handler.RouterHandler, telemetryHandler *handler.TelemetryHandler, incidentHandler *handler.IncidentHandler, webhookHandler *handler.WebhookHandler, voucherHandler *handler.VoucherHandler, monitorHandler *handler.MonitorHandler, fleetHandler *handler.FleetHandler, msg *messaging.Client, cache *cache.Client, db *database.DB, tenantMW gin.HandlerFunc) {
	r.GET("/health", healthCheck)
	r.GET("/ready", readinessCheck(&Server{cache: cache, db: db, messaging: msg}))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/version", version)

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
