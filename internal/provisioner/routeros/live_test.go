package routeros

import (
	"errors"
	"testing"
	"time"

	"mega-magic/internal/config"
)

func TestNewLive_disabledByDefault(t *testing.T) {
	_, _, err := NewLive(LiveConfig{Enabled: false, Host: "172.20.0.250"})
	if !errors.Is(err, ErrLiveDisabled) {
		t.Fatalf("err = %v, want ErrLiveDisabled", err)
	}
}

func TestNewLive_prodGuardTrips(t *testing.T) {
	cases := []LiveConfig{
		{Enabled: true, Host: "10.77.0.4", Port: 22, User: "admin", AllowEmptyPassword: true},
		{Enabled: true, Host: "hub.mikromagic.com", Port: 22, User: "admin", Password: "x"},
		{Enabled: true, Host: "172.20.0.250", Port: ProdHubPort, User: "admin", Password: "x"},
	}
	for _, cfg := range cases {
		if _, _, err := NewLive(cfg); err == nil {
			t.Fatalf("NewLive(%+v) = nil error, want prod refusal", cfg)
		}
	}
}

func TestNewLive_buildsForLabWithoutIO(t *testing.T) {
	ap, pr, err := NewLive(LiveConfig{
		Enabled:               true,
		Host:                  "172.20.0.250",
		Port:                  22,
		User:                  "admin",
		AllowEmptyPassword:    true,
		InsecureIgnoreHostKey: true,
		Interface:             "wg-magichub",
		ListenPortRotateTo:    13241,
		Timeout:               time.Second,
	})
	if err != nil {
		t.Fatalf("NewLive(lab) = %v, want no error", err)
	}
	if ap == nil || pr == nil {
		t.Fatal("lab config must build both applier and prober")
	}
}

func TestNewLive_requiresCredentials(t *testing.T) {
	_, _, err := NewLive(LiveConfig{Enabled: true, Host: "172.20.0.250", Port: 22})
	if err == nil {
		t.Fatal("no user must be refused")
	}
	_, _, err = NewLive(LiveConfig{Enabled: true, Host: "172.20.0.250", Port: 22, User: "admin"})
	if err == nil {
		t.Fatal("empty password without allow_empty_password must be refused")
	}
}

func TestLiveConfigFromConfig(t *testing.T) {
	got := LiveConfigFromConfig(config.ApplierConfig{Enabled: true, Host: "172.20.0.250", Port: 22, User: "admin", Interface: "wg-magichub", PingTarget: "10.99.0.1"})
	if !got.Enabled || got.Host != "172.20.0.250" || got.Port != 22 || got.PingTarget != "10.99.0.1" {
		t.Fatalf("mapping wrong: %+v", got)
	}
}
