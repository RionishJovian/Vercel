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
