package server

import (
	"fmt"
	"net/url"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// Kept off until explicit schema rollout and real-router acceptance. Startup
// refuses an opted-in deployment with missing forced RLS or disabled router I/O.
func registerGuestVoucherRoutes(r *gin.Engine, cfg *config.Config, db *database.DB, service *auth.Service, vouchers *handler.VoucherHandler) error {
	if !cfg.GuestWifi.VoucherAccessEnabled {
		return nil
	}
	if !cfg.Provisioner.Applier.Enabled {
		return fmt.Errorf("guest voucher access requires the live applier")
	}
	if db == nil || service == nil || vouchers == nil {
		return fmt.Errorf("guest voucher access dependencies unavailable")
	}
	if e := guestVoucherSchemaReady(db); e != nil {
		return e
	}
	box, e := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if e != nil {
		return e
	}
	source, e := routeros.NewCIDRTargetSource(database.NewDeviceCredentialSource(db, box), cfg.Provisioner.AllowedTargetCIDRs)
	if e != nil {
		return e
	}
	a := cfg.Provisioner.Applier
	applier := routeros.NewDeviceApplier(source, a.Timeout, nil, a.Interface, a.ListenPortRotateTo, a.ResetHold).WithControlConfig(a.PingTarget, a.TDown)
	recovery := database.GuestRecoveryRepository{DB: db}
	h := &handler.GuestVoucherHandler{DB: db, Vouchers: vouchers, ResolveDevice: database.NewDeviceRepository(db).GetDeviceByID,
		Redeemer:     guestaccess.ProvisionService{Store: database.GuestProvisionRepository{DB: db}, Router: routeros.GuestProvisionAdapter{Applier: applier}, Credentials: recovery},
		Resumer:      guestaccess.ResumeService{Store: recovery, Router: applier},
		PortalConfig: handler.GuestPortalConfigResolver(db),
	}
	registerGuestVoucherEndpoints(r, service, db, h, handler.SubscriptionAccess(db, cfg.Billing.SubscriptionOnly, cfg.Billing.ExistingAccountPolicy))
	return nil
}

func registerGuestVoucherEndpoints(r *gin.Engine, service *auth.Service, db *database.DB, h *handler.GuestVoucherHandler, access ...gin.HandlerFunc) {
	owner := r.Group("/api/v1/guest-vouchers", auth.CookieOrigin(), auth.Middleware(service), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleAdmin))
	owner.Use(access...)
	owner.POST("", h.Issue)
	owner.GET("/config", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(200, gin.H{"enabled": true, "validity_starts": "first_use"})
	})
	public := r.Group("/api/v1/portal", auth.CookieOrigin(), auth.BoundedPortalRateLimit(), boundedPortalBody())
	public.POST("/:deviceId/guest-voucher", h.Redeem)
	// Mutating MAC transfer remains absent until uncertain completion and restart
	// reconciliation are implemented. Read-only resume is a separate capability.
	if h.Recoverer != nil {
		public.POST("/:deviceId/guest-recovery", guestRecoveryOrigin(), h.Recover)
	}
	if h.Resumer != nil {
		public.POST("/:deviceId/guest-resume", guestRecoveryOrigin(), h.Resume)
	}
}

// Readiness is checked before exposing any route or minting recovery cookies.
// This is a compatibility/preflight check, not permission to auto-migrate.
func guestVoucherSchemaReady(db *database.DB) error {
	if db == nil {
		return fmt.Errorf("guest voucher database unavailable")
	}
	for _, spec := range []struct {
		table, trigger, function string
		columns                  []string
	}{
		{"guest_voucher_grants", "guest_voucher_snapshot_guard", "guard_guest_voucher_snapshot", []string{"voucher_id", "business_id", "device_id", "minutes", "data_mb", "state", "client_mac", "first_used_at", "expires_at", "lease_token", "lease_until", "updated_at"}},
		{"guest_voucher_grants", "guest_voucher_activation_guard", "guard_guest_voucher_activation", []string{"activated_at"}},
		{"guest_access_recovery", "guest_access_recovery_guard", "guard_guest_access_recovery", []string{"voucher_id", "business_id", "device_id", "secret_hash", "current_mac", "pending_mac", "state", "transfer_id", "completed_transfer_id", "used_bytes", "transfer_count", "window_started_at", "updated_at"}},
	} {
		var ready bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname=$1 AND c.relkind='r' AND c.relrowsecurity AND c.relforcerowsecurity
 AND EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc f ON f.oid=t.tgfoid JOIN pg_namespace fn ON fn.oid=f.pronamespace
 WHERE t.tgrelid=c.oid AND t.tgname=$2 AND NOT t.tgisinternal AND t.tgenabled IN ('O','A') AND t.tgtype=19 AND f.proname=$3 AND fn.nspname='public')
 AND EXISTS(SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid AND p.polname=$1||'_tenant' AND p.polcmd='*'
 AND p.polqual IS NOT NULL AND p.polwithcheck IS NOT NULL AND p.polroles=ARRAY[(SELECT oid FROM pg_roles WHERE rolname='authenticated')])
 AND (SELECT count(*) FROM pg_policy p WHERE p.polrelid=c.oid)=1
 AND has_table_privilege('authenticated',c.oid,'SELECT')
 AND has_table_privilege('authenticated',c.oid,'INSERT')
 AND has_table_privilege('authenticated',c.oid,'UPDATE'))`, spec.table, spec.trigger, spec.function).Scan(&ready)
		if err != nil || !ready {
			return fmt.Errorf("guest voucher schema protections unavailable for %s", spec.table)
		}
		for _, column := range spec.columns {
			if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, spec.table, column).Scan(&ready); err != nil || !ready {
				return fmt.Errorf("guest voucher schema incompatible for %s", spec.table)
			}
		}
	}
	return nil
}

// Paid recovery is cookie-authenticated even when the guest has no business
// session. Require explicit same-origin intent; Authorization cannot bypass it.
func guestRecoveryOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := url.Parse(c.GetHeader("Origin"))
		valid := err == nil && u.Host == c.Request.Host && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" &&
			(u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")))
		if !valid || c.GetHeader("X-MM-CSRF") != "1" {
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatusJSON(403, gin.H{"error": "same-origin recovery request required"})
			return
		}
		c.Next()
	}
}
