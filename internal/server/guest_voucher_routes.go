package server

import (
	"context"
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
		// Ground-operation voucher login: the RouterBOARD validates issued
		// codes locally; the app installs them at issue time and
		// synchronizes their live accounting back through the hub.
		Provisioner: voucherLoginTransport{applier: applier},
		Syncer:      voucherLoginTransport{applier: applier},
	}
	registerGuestVoucherEndpoints(r, service, db, h, handler.SubscriptionAccess(db, cfg.Billing.SubscriptionOnly, cfg.Billing.ExistingAccountPolicy), handler.IndividualPassAccess(db, cfg.Billing.IndividualPassesEnabled))
	return nil
}

// voucherLoginTransport adapts the live RouterOS applier to the handler's
// voucher-login interfaces (router -> hub -> app transport stays the hub).
type voucherLoginTransport struct {
	applier *routeros.DeviceApplier
}

func (t voucherLoginTransport) ProvisionVoucherLogins(ctx context.Context, businessID, deviceID string, logins []handler.VoucherLoginSpec) error {
	out := make([]routeros.VoucherLogin, 0, len(logins))
	for _, l := range logins {
		out = append(out, routeros.VoucherLogin{VoucherID: l.VoucherID, Code: l.Code, Minutes: l.Minutes, DataMB: l.DataMB})
	}
	return t.applier.ProvisionVoucherLogins(ctx, businessID, deviceID, out)
}

func (t voucherLoginTransport) ReadVoucherLogins(ctx context.Context, businessID, deviceID string, refs []handler.VoucherLoginSpec) ([]handler.VoucherLoginState, error) {
	out := make([]routeros.VoucherLogin, 0, len(refs))
	for _, l := range refs {
		out = append(out, routeros.VoucherLogin{VoucherID: l.VoucherID, Code: l.Code})
	}
	states, err := t.applier.ReadVoucherLogins(ctx, businessID, deviceID, out)
	if err != nil {
		return nil, err
	}
	adapted := make([]handler.VoucherLoginState, 0, len(states))
	for _, s := range states {
		adapted = append(adapted, handler.VoucherLoginState{VoucherID: s.VoucherID, Exists: s.Exists, Disabled: s.Disabled,
			UptimeSeconds: s.UptimeSeconds, BytesUsed: s.BytesUsed, ActiveMAC: s.ActiveMAC,
			FirstUseEpoch: s.FirstUseEpoch, FirstMAC: s.FirstMAC})
	}
	return adapted, nil
}

func registerGuestVoucherEndpoints(r *gin.Engine, service *auth.Service, db *database.DB, h *handler.GuestVoucherHandler, access ...gin.HandlerFunc) {
	owner := r.Group("/api/v1/guest-vouchers", auth.CookieOrigin(), auth.Middleware(service), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleAdmin))
	owner.Use(access...)
	owner.POST("", h.Issue)
	owner.GET("/batches", h.ListIssueBatches)
	owner.GET("/batches/:batchId", h.GetIssueBatch)
	owner.POST("/batches/:batchId/retry", h.RetryIssueBatch)
	// Router -> hub -> app accounting synchronization for locally validated
	// voucher codes (ground-operation design).
	owner.POST("/sync", h.SyncVoucherLogins)
	owner.GET("/config", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(200, gin.H{"enabled": true, "validity_starts": "first_use", "batch_recovery_enabled": true, "batch_id_required": true, "max_batch_size": handler.GuestVoucherIssueBatchLimit})
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
		{"guest_voucher_issue_batches", "guest_voucher_issue_batch_guard", "guard_guest_voucher_issue_batch", []string{"business_id", "id", "device_id", "request_hash", "plan_label", "price_mmk", "minutes", "data_mb", "total_count", "lease_token", "lease_until", "created_at", "updated_at"}},
		{"guest_voucher_issue_items", "guest_voucher_issue_item_guard", "guard_guest_voucher_issue_item", []string{"business_id", "batch_id", "voucher_id", "position", "install_state", "verified_at", "updated_at"}},
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
