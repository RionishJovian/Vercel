package server

import (
	"strings"
	"testing"

	"go.uber.org/zap"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/monitoring"
	"mega-magic/internal/secretbox"
)

// Posture capture is READ-ONLY with respect to routers, so it must be possible to
// enable it WITHOUT enabling the live applier — which performs router WRITES and
// whose misconfiguration fails server startup. Before this was decoupled, capture
// sat inside `if !applier.Enabled { return nil }`, so a read-only monitoring
// feature could not be turned on without also turning on a write-capable one.
func TestWirePostureCaptureGating(t *testing.T) {
	sugar := zap.NewNop().Sugar()

	// base64 of 32 zero bytes — a structurally valid key for this test only.
	const goodKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if _, err := secretbox.New(goodKey); err != nil {
		t.Fatalf("fixture key must be valid: %v", err)
	}

	// Nothing wired: a disabled capture must be a clean no-op, never an error.
	t.Run("disabled is a no-op even with nothing wired", func(t *testing.T) {
		if err := wirePostureCapture(&config.Config{}, sugar, nil, nil); err != nil {
			t.Fatalf("disabled capture must not error, got %v", err)
		}
	})

	// An EXPLICIT enable that cannot work must fail loudly rather than silently
	// capturing nothing while the app reports itself enabled.
	t.Run("enabled without monitor/db fails", func(t *testing.T) {
		cfg := &config.Config{Provisioner: config.ProvisionerConfig{
			PostureCapture: config.PostureCaptureConfig{Enabled: true},
		}}
		if err := wirePostureCapture(cfg, sugar, nil, nil); err == nil {
			t.Fatal("an explicit enable that cannot work must fail, not be silently ignored")
		}
	})

	t.Run("enabled with no key fails and names the setting", func(t *testing.T) {
		cfg := &config.Config{Provisioner: config.ProvisionerConfig{
			PostureCapture: config.PostureCaptureConfig{Enabled: true},
		}}
		err := wirePostureCapture(cfg, sugar, &monitoring.MonitoringService{}, &database.DB{})
		if err == nil {
			t.Fatal("enabled capture with no key must fail")
		}
		if !strings.Contains(err.Error(), "credential_key") {
			t.Errorf("error should name the setting to fix, got: %v", err)
		}
	})

	// THE POINT OF THIS CHANGE: capture wires on its own flag with the applier
	// explicitly OFF.
	t.Run("wires with the applier disabled", func(t *testing.T) {
		cfg := &config.Config{Provisioner: config.ProvisionerConfig{
			PostureCapture: config.PostureCaptureConfig{Enabled: true, CredentialKey: goodKey},
			Applier:        config.ApplierConfig{Enabled: false},
		}}
		if err := wirePostureCapture(cfg, sugar, &monitoring.MonitoringService{}, &database.DB{}); err != nil {
			t.Fatalf("capture should wire on its own flag; got %v", err)
		}
	})

	// The key used to live under the applier; an existing deployment must keep
	// working without moving it.
	t.Run("key still read from the original applier location", func(t *testing.T) {
		cfg := &config.Config{Provisioner: config.ProvisionerConfig{
			PostureCapture: config.PostureCaptureConfig{Enabled: true}, // no key here
			Applier:        config.ApplierConfig{CredentialKey: goodKey},
		}}
		if err := wirePostureCapture(cfg, sugar, &monitoring.MonitoringService{}, &database.DB{}); err != nil {
			t.Fatalf("back-compat key location should work; got %v", err)
		}
	})
}
