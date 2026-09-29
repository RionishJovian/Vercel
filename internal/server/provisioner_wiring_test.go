package server

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/provisioner"
	"mega-magic/internal/secretbox"
)

// stubApplier satisfies provisioner.Applier for wiring-decision tests; the
// decision only cares whether it is non-nil.
type stubApplier struct{}

func (stubApplier) Snapshot(_ context.Context, _, _ string, _ []provisioner.Change) (provisioner.RollbackSnapshot, error) {
	return provisioner.RollbackSnapshot{}, nil
}
func (stubApplier) Apply(_ context.Context, _, _ string, _ []provisioner.Change) error { return nil }
func (stubApplier) Restore(_ context.Context, _ string, _ provisioner.RollbackSnapshot) error {
	return nil
}

func TestShouldEnableLiveApplier(t *testing.T) {
	prober := provisioner.StaticProber{}
	cases := []struct {
		name    string
		enabled bool
		applier provisioner.Applier
		prober  provisioner.Prober
		want    bool
	}{
		{"flag off, everything built", false, stubApplier{}, prober, false},
		{"flag on, everything built", true, stubApplier{}, prober, true},
		{"flag on, no applier", true, nil, prober, false},
		{"flag on, no prober", true, stubApplier{}, nil, false},
		{"flag off, nothing built", false, nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldEnableLiveApplier(tc.enabled, tc.applier, tc.prober); got != tc.want {
				t.Fatalf("shouldEnableLiveApplier = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStartLiveApplier_disabledIsNoOp(t *testing.T) {
	// Default posture: flag OFF -> no error, no live path, no transport.
	cfg := &config.Config{}
	cfg.Provisioner.Applier.Enabled = false
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil, nil); err != nil {
		t.Fatalf("disabled live applier must be a no-op, got %v", err)
	}
}

func TestStartLiveApplier_enabledWithoutDatabaseFailsLoudly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled: true,
	}
	err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil, nil)
	if err == nil {
		t.Fatal("enabled per-device control without a database must fail startup loudly")
	}
}

func TestStartLiveApplier_enabledWithoutTargetAllowlistFailsLoudly(t *testing.T) {
	credentialKey, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Provisioner.Applier = config.ApplierConfig{Enabled: true, CredentialKey: credentialKey}
	err = startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, &database.DB{}, nil, nil)
	if err == nil {
		t.Fatal("enabled router access without an explicit target CIDR allowlist must fail")
	}
}

func TestStartLiveApplier_enabledMissingCredentialKeyFailsLoudly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled: true,
	}
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, &database.DB{}, nil, nil); err == nil {
		t.Fatal("enabled + missing credential encryption key must fail loudly")
	}
}

func TestStartLiveApplier_enabledButNoClientFailsLoudly(t *testing.T) {
	// A valid per-device configuration builds the control path, but with no
	// messaging client the consumer cannot start. That must surface loudly.
	credentialKey, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled:       true,
		CredentialKey: credentialKey,
		Interface:     "wg-magichub",
	}
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, &database.DB{}, nil, nil); err == nil {
		t.Fatal("enabled live path without a messaging client must fail loudly")
	}
}
