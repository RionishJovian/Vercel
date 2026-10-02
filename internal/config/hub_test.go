package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHubLookupConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("environment: development\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("VPS_HUB_LOOKUP_ENABLED", "false")
	t.Setenv("VPS_HUB_ENROLLMENT_ENABLED", "false")
	cfg, err := Load(path)
	if err != nil || cfg.VPS.HubLookupEnabled {
		t.Fatalf("default disabled: %v", err)
	}
	t.Setenv("VPS_HUB_ENROLLMENT_ENABLED", "true")
	if _, err := Load(path); err == nil {
		t.Fatal("enrollment enabled without lookup")
	}
	t.Setenv("VPS_HUB_ENROLLMENT_ENABLED", "false")
	t.Setenv("VPS_HUB_LOOKUP_ENABLED", "true")
	if _, err := Load(path); err == nil {
		t.Fatal("enabled without credentials")
	}
	t.Setenv("VPS_HUB_URL", "https://hub.example.test/internal/provisioner")
	t.Setenv("VPS_HUB_KEY_ID", "test-v1")
	key := strings.Repeat("ab", 32)
	t.Setenv("VPS_HUB_SIGNING_KEY", key)
	cfg, err = Load(path)
	if err != nil || !cfg.VPS.HubLookupEnabled || cfg.VPS.HubSigningKey != key {
		t.Fatalf("opt-in env configuration: %v", err)
	}
	t.Setenv("VPS_HUB_ENROLLMENT_ENABLED", "true")
	if enrolled, err := Load(path); err != nil || !enrolled.VPS.HubEnrollmentEnabled {
		t.Fatal("enrollment opt-in unavailable", err)
	}
	raw, err := json.Marshal(cfg.VPS)
	if err != nil || strings.Contains(string(raw), key) {
		t.Fatal("key exposed in JSON")
	}
	t.Setenv("VPS_HUB_URL", "http://hub.example.test")
	if _, err := Load(path); err == nil {
		t.Fatal("plaintext endpoint accepted")
	}
}
