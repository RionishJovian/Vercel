package server

import (
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/messaging"
	"mega-magic/internal/monitoring"
	"mega-magic/internal/provisioner"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/recovery"
	"mega-magic/internal/secretbox"
)

// shouldEnableLiveApplier is the pure wiring decision: live router writes are
// wired only when the flag is on AND a usable (applier, prober) pair was built.
// Anything else keeps the recovery ladder plan-only.
func shouldEnableLiveApplier(flagEnabled bool, applier provisioner.Applier, prober provisioner.Prober) bool {
	return flagEnabled && applier != nil && prober != nil
}

// startLiveApplier wires the existing recovery ApplyConsumer to the real
// RouterOS Applier, gated behind provisioner.applier.enabled (OFF by default).
//
// Disabled (the default): logs one line and returns nil — no live writes, no
// live transport, recovery stays plan-only.
//
// Enabled: builds a tenant-scoped per-device applier/prober from encrypted
// device credentials and starts the consumer. Any error fails server startup
// loudly — a mis-config must never silently look "enabled".
func startLiveApplier(cfg *config.Config, client *messaging.Client, sugar *zap.SugaredLogger, monitor *monitoring.MonitoringService, db *database.DB, backup *handler.BackupHandler, securityHandler *handler.SecurityHandler, incidentAction *handler.IncidentActionHandler, guestHandlers ...*handler.GuestWifiHandler) error {
	// Security-posture capture is wired FIRST, on its OWN flag. It never writes to
	// a router, so it must not sit behind applier.enabled — that flag turns on
	// live router WRITES and the single-host live applier they require (which
	// fails server startup when misconfigured). Gating a read-only monitoring
	// feature behind a write-capable one made it impossible to enable safely.
	if err := wirePostureCapture(cfg, sugar, monitor, db); err != nil {
		return err
	}

	appCfg := cfg.Provisioner.Applier
	if !appCfg.Enabled {
		sugar.Info("live applier disabled — recovery ladder stays plan-only")
		return nil
	}

	if db == nil {
		return errors.New("live applier: database is required for per-device target resolution")
	}
	box, err := secretbox.New(appCfg.CredentialKey)
	if err != nil {
		return fmt.Errorf("live applier credential key: %w", err)
	}
	source := database.NewDeviceCredentialSource(db, box)
	policySource, err := routeros.NewCIDRTargetSource(source, cfg.Provisioner.AllowedTargetCIDRs)
	if err != nil {
		return fmt.Errorf("live applier target policy: %w", err)
	}
	applier := routeros.NewDeviceApplier(
		policySource,
		appCfg.Timeout,
		nil,
		appCfg.Interface,
		appCfg.ListenPortRotateTo,
		appCfg.ResetHold,
	).WithControlConfig(appCfg.PingTarget, appCfg.TDown)
	prober := routeros.NewDeviceProber(policySource, appCfg.Timeout, nil).
		WithControlConfig(appCfg.Interface, appCfg.PingTarget, appCfg.TDown)
	if !shouldEnableLiveApplier(appCfg.Enabled, applier, prober) {
		return fmt.Errorf("live applier: enabled but no applier/prober was constructed")
	}

	svc := provisioner.NewApplyService(applier, prober)
	for _, guest := range guestHandlers {
		if guest != nil {
			guest.EnableGuestGrace(applier)
		}
	}
	if securityHandler != nil {
		securityHandler.EnableLive(svc, applier, prober, appCfg.ManagementCIDRs)
		sugar.Info("security verification and mitigation controls wired to the live applier")
	}

	// C2: attach the live applier + OTP verifier to the backup/restore handler so
	// capture/restore stop failing closed. Same flag gates both.
	if backup != nil {
		backupOTP := provisioner.NewTOTPVerifier(provisioner.TOTPConfig{
			Secret: appCfg.OtpSecret,
			Digits: appCfg.OtpDigits,
			Step:   appCfg.OtpStep,
			Skew:   appCfg.OtpSkew,
		})
		backup.EnableLive(applier, backupOTP)
		sugar.Info("config backup/restore wired to the live applier")
	}

	// Stage 1A: attach the live applier to the incident-action handler too.
	if incidentAction != nil {
		incidentAction.EnableLive(applier)
	}

	// Real RFC 6238 TOTP gate for HIGH-RISK actions (Slice 2). Until a base32
	// secret is configured (provisioner.applier.otp_secret), the verifier DENIES
	// every high-risk approval, preserving the fail-closed posture. Slice 3 (the
	// Telegram bridge) will hand this verifier the presented code.
	otp := provisioner.NewTOTPVerifier(provisioner.TOTPConfig{
		Secret: appCfg.OtpSecret,
		Digits: appCfg.OtpDigits,
		Step:   appCfg.OtpStep,
		Skew:   appCfg.OtpSkew,
	})

	approver := recovery.NewNatsApprover(client, 5*time.Minute)
	consumer := recovery.NewApplyConsumer(svc, prober, approver, otp, client, sugar)
	if err := consumer.Start(client); err != nil {
		return fmt.Errorf("live applier consumer start: %w", err)
	}

	// Topology explicit device actions (reboot) on the same apply-core + gate:
	// fire-and-validate, no config rollback. Same flag — live router writes for
	// owner-initiated commands are only ever wired when the applier is enabled.
	topoConsumer := recovery.NewTopologyActionConsumer(svc, prober, approver, otp, client, sugar)
	if err := topoConsumer.Start(client); err != nil {
		return fmt.Errorf("topology action consumer start: %w", err)
	}

	sugar.Infow("Live per-device apply consumer started (flag-gated)",
		"interface", appCfg.Interface)

	// Telegram approve bridge (Slice 3): answers provision.approval on behalf of
	// the human Lead. The real bot transport is an injected seam; without a
	// messenger the bridge is inert and approvals time out to DENY (fail closed).
	if messenger, ok := provisioner.ConfigTelegramMessenger(appCfg.TelegramBotToken, appCfg.TelegramChatID, 5*time.Minute); ok {
		approver := provisioner.NewTelegramApprover(messenger, appCfg.TelegramChatID, 5*time.Minute)
		bridge := recovery.NewApprovalBridge(approver, client, sugar)
		if err := bridge.Start(); err != nil {
			return fmt.Errorf("approval bridge start: %w", err)
		}
		sugar.Info("Telegram approval bridge started (flag-gated)")
	} else {
		sugar.Warn("Telegram approval bridge not configured (no bot messenger) — approval requests will time out and DENY")
	}

	return nil
}

// wirePostureCapture wires the fleet's per-device RouterOS version observer.
//
// READ-ONLY with respect to routers — it only reads a version over SSH — which is
// exactly why it is gated by its own flag (provisioner.posture_capture.enabled)
// and not by applier.enabled.
//
// An EXPLICIT enable that cannot work is a startup ERROR. The previous code
// warned and continued, which was only tolerable while capture was an accidental
// side effect of the applier flag. Once an operator asks for capture by name,
// capturing nothing while reporting success is precisely the "looks enabled but
// isn't" failure this codebase refuses everywhere else.
func wirePostureCapture(cfg *config.Config, sugar *zap.SugaredLogger, monitor *monitoring.MonitoringService, db *database.DB) error {
	c := cfg.Provisioner.PostureCapture
	if !c.Enabled {
		sugar.Info("security-posture capture disabled — device versions stay unknown")
		return nil
	}
	if monitor == nil || db == nil {
		return errors.New("security-posture capture: enabled but the monitoring service or database is unavailable")
	}

	// The key originally lived under the applier; that location is still read so
	// an existing deployment does not have to be migrated.
	key := c.CredentialKey
	if key == "" {
		key = cfg.Provisioner.Applier.CredentialKey
	}
	box, err := secretbox.New(key)
	if err != nil {
		return fmt.Errorf("security-posture capture: %w (set provisioner.posture_capture.credential_key)", err)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// A PER-DEVICE prober, deliberately NOT the single-host `prober` built above
	// for the applier: the fleet sweep evaluates every peer in every tenant, so a
	// single-host observer would stamp one router's version onto every device in
	// the fleet. The type system enforces this — Prober's ObserveVersion does not
	// satisfy monitoring.VersionObserver, so that mistake will not compile.
	src := database.NewDeviceCredentialSource(db, box)
	policySource, err := routeros.NewCIDRTargetSource(src, cfg.Provisioner.AllowedTargetCIDRs)
	if err != nil {
		return fmt.Errorf("security-posture capture target policy: %w", err)
	}
	deviceProber := routeros.NewDeviceProber(policySource, timeout, nil)
	monitor.WithVersionCapture(db, database.NewDeviceRepository(db), deviceProber)
	sugar.Info("security-posture capture wired (per-device transport)")
	return nil
}
