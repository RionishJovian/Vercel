package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMediaConnectorDefaultOffAndOptIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(path, []byte("environment: development\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("MEDIA_CONNECTOR_ENABLED", "false")
	cfg, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Media.ConnectorEnabled {
		t.Fatal("media default must be off")
	}
	t.Setenv("MEDIA_CONNECTOR_ENABLED", "true")
	cfg, e = Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if !cfg.Media.ConnectorEnabled {
		t.Fatal("explicit media opt-in ignored")
	}
}
