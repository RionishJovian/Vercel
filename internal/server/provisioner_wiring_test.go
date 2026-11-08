package server

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
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
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("disabled live applier must be a no-op, got %v", err)
	}
}

func TestStartLiveApplier_enabledWithoutDatabaseFailsLoudly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled: true,
	}
	err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil, nil, nil, nil)
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
	err = startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, &database.DB{}, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("enabled router access without an explicit target CIDR allowlist must fail")
	}
}

func TestStartLiveApplier_enabledMissingCredentialKeyFailsLoudly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled: true,
	}
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, &database.DB{}, nil, nil, nil, nil); err == nil {
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
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, &database.DB{}, nil, nil, nil, nil); err == nil {
		t.Fatal("enabled live path without a messaging client must fail loudly")
	}
}

func TestWireBackupCaptureIndependentOfLiveApply(t *testing.T) {
	sugar := zap.NewNop().Sugar()
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	t.Run("off remains disabled", func(t *testing.T) {
		backup := &handler.BackupHandler{}
		if err := wireBackupCapture(&config.Config{}, sugar, nil, backup); err != nil {
			t.Fatalf("disabled backup capture must be no-op: %v", err)
		}
		if backup.CaptureEnabledForTest() { t.Fatal("backup was enabled without an opt-in") }
	})
	t.Run("missing database fails", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Provisioner.BackupCapture.Enabled = true
		if err := wireBackupCapture(cfg, sugar, nil, &handler.BackupHandler{}); err == nil {
			t.Fatal("enabled backup without DB must fail")
		}
	})
	t.Run("bad key fails", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Provisioner.BackupCapture.Enabled = true
		cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
		if err := wireBackupCapture(cfg, sugar, &database.DB{}, &handler.BackupHandler{}); err == nil {
			t.Fatal("enabled backup without decrypt key must fail")
		}
	})
	t.Run("missing CIDR allowlist fails", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Provisioner.BackupCapture = config.BackupCaptureConfig{Enabled: true, CredentialKey: key}
		if err := wireBackupCapture(cfg, sugar, &database.DB{}, &handler.BackupHandler{}); err == nil {
			t.Fatal("enabled backup without management CIDR must fail")
		}
	})
	t.Run("capture works while applier disabled", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Provisioner.Applier.Enabled = false
		cfg.Provisioner.BackupCapture = config.BackupCaptureConfig{Enabled: true, CredentialKey: key}
		cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
		backup := &handler.BackupHandler{}
		if err := wireBackupCapture(cfg, sugar, &database.DB{}, backup); err != nil {
			t.Fatalf("capture-only wiring failed: %v", err)
		}
		if !backup.CaptureEnabledForTest() || backup.RestoreEnabledForTest() {
			t.Fatal("capture must not grant restore when applier is disabled")
		}
	})
}
