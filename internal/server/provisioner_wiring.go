package server

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/config"
	"mega-magic/internal/messaging"
	"mega-magic/internal/provisioner"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/recovery"
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
// Enabled: builds the live applier/prober via routeros.NewLive (which itself
// fails closed on prod-shaped targets or missing credentials) and starts the
// consumer. Any error fails server startup loudly — a mis-config must never
// silently look "enabled".
func startLiveApplier(cfg *config.Config, client *messaging.Client, sugar *zap.SugaredLogger) error {
	appCfg := cfg.Provisioner.Applier
	if !appCfg.Enabled {
		sugar.Info("live applier disabled — recovery ladder stays plan-only")
		return nil
	}

	applier, prober, err := routeros.NewLive(routeros.LiveConfigFromConfig(appCfg))
	if err != nil {
		return fmt.Errorf("live applier: %w", err)
	}
	if !shouldEnableLiveApplier(appCfg.Enabled, applier, prober) {
		return fmt.Errorf("live applier: enabled but no applier/prober was constructed")
	}

	svc := provisioner.NewApplyService(applier, prober)

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

	sugar.Infow("Live apply consumer started (flag-gated)",
		"host", appCfg.Host, "interface", appCfg.Interface)

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
