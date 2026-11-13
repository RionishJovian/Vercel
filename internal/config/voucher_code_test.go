package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadConfigYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("environment: development\n"+body), 0600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

// Protection is opt-in: an untouched config changes nothing.
func TestVoucherCodeProtectionDefaultsOff(t *testing.T) {
	cfg, err := loadConfigYAML(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.ProtectVoucherCodes || cfg.Security.VoucherCodeKey != "" {
		t.Fatal("voucher code protection must default to off with no key")
	}
}

func TestVoucherCodeProtectionNeedsAValidKey(t *testing.T) {
	if _, err := loadConfigYAML(t, "security:\n  protect_voucher_codes: true\n"); err == nil || !strings.Contains(err.Error(), "voucher_code_key") {
		t.Fatalf("protecting new codes without a key must be refused, got %v", err)
	}
	if _, err := loadConfigYAML(t, "security:\n  voucher_code_key: not-a-key\n"); err == nil || !strings.Contains(err.Error(), "voucher_code_key") {
		t.Fatalf("a malformed key must be refused even with the flag off, got %v", err)
	}
	cfg, err := loadConfigYAML(t, "security:\n  protect_voucher_codes: true\n  voucher_code_key: \""+validCredentialKey+"\"\n")
	if err != nil {
		t.Fatalf("a valid key with the flag on must load: %v", err)
	}
	if !cfg.Security.ProtectVoucherCodes {
		t.Fatal("flag not read")
	}
	// A key alone (flag off) is valid: it lets the app read rows that were already protected.
	if _, err := loadConfigYAML(t, "security:\n  voucher_code_key: \""+validCredentialKey+"\"\n"); err != nil {
		t.Fatalf("key with the flag off must load: %v", err)
	}
}
