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

	// Slice 1 deliberately ships NO real TOTP provider. Passing a nil
	// OtpVerifier preserves the committed fail-closed rule: an approved
	// high-risk plan (rotate_listen_port/escalate) is DENIED by
	// EvaluateApproval until the real provider lands in Slice 2.
	approver := recovery.NewNatsApprover(client, 5*time.Minute)
	consumer := recovery.NewApplyConsumer(svc, prober, approver, nil, client, sugar)
	if err := consumer.Start(client); err != nil {
		return fmt.Errorf("live applier consumer start: %w", err)
	}

	sugar.Infow("Live apply consumer started (flag-gated)",
		"host", appCfg.Host, "interface", appCfg.Interface)
	return nil
}
