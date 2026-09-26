package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_provisionerApplierDefaultsOff proves the live applier is DISABLED by
// default (deliverable 4) even when the config file says nothing about it.
func TestLoad_provisionerApplierDefaultsOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("# no provisioner section on purpose\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ap := cfg.Provisioner.Applier
	if ap.Enabled {
		t.Fatal("provisioner.applier.enabled must default to false")
	}
	if ap.Port != 22 {
		t.Fatalf("applier port default = %d, want 22", ap.Port)
	}
	if !ap.InsecureIgnoreHostKey {
		t.Fatal("lab default expects insecure_ignore_host_key=true")
	}
	if ap.AllowEmptyPassword {
		t.Fatal("allow_empty_password must default to false")
	}
	if ap.Interface != "wg-magichub" {
		t.Fatalf("interface default = %q, want wg-magichub", ap.Interface)
	}
	if ap.Host != "" {
		t.Fatalf("host must default empty, got %q", ap.Host)
	}
}

// TestLoad_vpsConfigParses proves the real config.yaml's vps section loads.
func TestLoad_vpsConfigParses(t *testing.T) {
	cfg, err := Load("../../config.yaml")
	if err != nil {
		t.Fatalf("Load config.yaml: %v", err)
	}
	if cfg.VPS.EndpointHost != "" {
		t.Fatalf("vps.endpoint_host should default empty, got %q", cfg.VPS.EndpointHost)
	}
	if cfg.WireGuard.Interface != "wg-magichub" || cfg.WireGuard.ListenPort != 13231 {
		t.Fatalf("wireguard config not loaded: %+v", cfg.WireGuard)
	}
}

// TestLoad_billingTelegramDefaultsOff proves the operator-purchase bot is
// DISABLED by default (fail closed): no token, no chat, so the bridge stays
// inert until explicitly enabled.
func TestLoad_billingTelegramDefaultsOff(t *testing.T) {
	cfg, err := Load("../../config.yaml")
	if err != nil {
		t.Fatalf("Load config.yaml: %v", err)
	}
	tg := cfg.Billing.Telegram
	if tg.Enabled {
		t.Fatal("billing.telegram.enabled must default to false")
	}
	if tg.BotToken != "" {
		t.Fatal("billing.telegram.bot_token must default empty")
	}
	if tg.ChatID != 0 {
		t.Fatalf("billing.telegram.chat_id must default 0, got %d", tg.ChatID)
	}
}
