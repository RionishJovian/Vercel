package server

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"mega-magic/internal/config"
	"mega-magic/internal/provisioner"
)

// stubApplier satisfies provisioner.Applier for wiring-decision tests; the
// decision only cares whether it is non-nil.
type stubApplier struct{}

func (stubApplier) Snapshot(_ context.Context, _ string, _ []provisioner.Change) (provisioner.RollbackSnapshot, error) {
	return provisioner.RollbackSnapshot{}, nil
}
func (stubApplier) Apply(_ context.Context, _ string, _ []provisioner.Change) error { return nil }
func (stubApplier) Restore(_ context.Context, _ provisioner.RollbackSnapshot) error { return nil }

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
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil); err != nil {
		t.Fatalf("disabled live applier must be a no-op, got %v", err)
	}
}

func TestStartLiveApplier_enabledProdTargetFailsLoudly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled:            true,
		Host:               "10.77.0.4", // prod-shaped: guard must refuse
		Port:               22,
		User:               "admin",
		AllowEmptyPassword: true,
	}
	err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil)
	if err == nil {
		t.Fatal("enabled + prod-shaped target must fail startup loudly")
	}
}

func TestStartLiveApplier_enabledMissingCredsFailsLoudly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled: true,
		Host:    "172.20.0.250", // lab-shaped, but no credentials
		Port:    22,
	}
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil); err == nil {
		t.Fatal("enabled + missing credentials must fail loudly")
	}
}

func TestStartLiveApplier_enabledButNoClientFailsLoudly(t *testing.T) {
	// A valid lab config builds the live applier, but with no messaging client
	// the consumer cannot start: that must surface as an error, not be swallowed.
	cfg := &config.Config{}
	cfg.Provisioner.Applier = config.ApplierConfig{
		Enabled:               true,
		Host:                  "172.20.0.250",
		Port:                  22,
		User:                  "admin",
		AllowEmptyPassword:    true,
		InsecureIgnoreHostKey: true,
		Interface:             "wg-magichub",
	}
	if err := startLiveApplier(cfg, nil, zap.NewNop().Sugar(), nil, nil, nil); err == nil {
		t.Fatal("enabled live path without a messaging client must fail loudly")
	}
}
