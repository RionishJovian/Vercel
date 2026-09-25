package routeros

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"mega-magic/internal/config"
)

// ErrLiveDisabled is returned when the live applier is requested while the
// config flag is off. Live writes are OFF by default.
var ErrLiveDisabled = errors.New("routeros: live applier disabled (set provisioner.applier.enabled=true to allow real writes)")

// LiveConfig is the fully-resolved config for the real transport. Credentials
// are injected here, never hardcoded.
type LiveConfig struct {
	Enabled               bool
	Host                  string
	Port                  int
	User                  string
	Password              string
	PrivateKeyPath        string
	InsecureIgnoreHostKey bool
	// AllowEmptyPassword permits lab-style admin/empty logins; it must be
	// explicitly enabled because an empty password is otherwise a config error.
	AllowEmptyPassword bool
	Timeout            time.Duration

	Interface          string
	ListenPortRotateTo int
	ResetHold          time.Duration
	PingTarget         string
	TDown              time.Duration
}

// LiveConfigFromConfig maps the config file section into a LiveConfig.
func LiveConfigFromConfig(c config.ApplierConfig) LiveConfig {
	return LiveConfig{
		Enabled:               c.Enabled,
		Host:                  c.Host,
		Port:                  c.Port,
		User:                  c.User,
		Password:              c.Password,
		PrivateKeyPath:        c.PrivateKeyPath,
		InsecureIgnoreHostKey: c.InsecureIgnoreHostKey,
		AllowEmptyPassword:    c.AllowEmptyPassword,
		Timeout:               c.Timeout,
		Interface:             c.Interface,
		ListenPortRotateTo:    c.ListenPortRotateTo,
		ResetHold:             c.ResetHold,
		PingTarget:            c.PingTarget,
		TDown:                 c.TDown,
	}
}

// NewLive constructs the real Applier + Prober, gated by:
//
//  1. Enabled must be true (OFF by default), and
//  2. GuardTarget must accept the host/port (fail closed on prod-shaped targets),
//  3. credentials must be configured (no anonymous prod-adjacent defaults).
//
// It performs no network I/O at construction time.
func NewLive(cfg LiveConfig) (*Applier, *Prober, error) {
	if !cfg.Enabled {
		return nil, nil, ErrLiveDisabled
	}

	port := cfg.Port
	if port == 0 {
		port = 22
	}
	if err := GuardTarget(cfg.Host, port); err != nil {
		return nil, nil, err
	}
	if cfg.User == "" {
		return nil, nil, fmt.Errorf("routeros: no SSH user configured; refusing")
	}

	var keyPEM []byte
	if cfg.PrivateKeyPath != "" {
		b, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("routeros: read ssh private key %s: %w", cfg.PrivateKeyPath, err)
		}
		keyPEM = b
	}
	if cfg.Password == "" && len(keyPEM) == 0 && !cfg.AllowEmptyPassword {
		return nil, nil, fmt.Errorf("routeros: no SSH credential configured (password, private key, or allow_empty_password); refusing")
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	tr := NewSSHTransport(SSHConfig{
		Addr:                  addr,
		User:                  cfg.User,
		Password:              cfg.Password,
		PrivateKeyPEM:         keyPEM,
		InsecureIgnoreHostKey: cfg.InsecureIgnoreHostKey,
		Timeout:               cfg.Timeout,
	})
	return NewApplier(tr, cfg.Interface, cfg.ListenPortRotateTo, cfg.ResetHold),
		NewProber(tr, cfg.Interface, cfg.PingTarget, cfg.TDown),
		nil
}
