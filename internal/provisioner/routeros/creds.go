package routeros

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ErrNoTarget means the peer has no usable connection facts (no address, or no
// credentials). It is a DISTINCT, expected outcome — not a transport failure.
//
// The distinction matters: "this device was never provisioned for SSH" must be
// an honest skip, whereas "this device is unreachable" is an incident. Folding
// them together is how a fleet screen ends up lying.
var ErrNoTarget = errors.New("routeros: device has no ssh target configured")

// ErrIncompleteTarget means the row is partially configured. The DB CHECK
// constraint should make this unreachable; treating it as an error here means a
// constraint bypass (a manual UPDATE with constraints deferred, a restore) still
// cannot yield a half-built transport.
var ErrIncompleteTarget = errors.New("routeros: device ssh target is incomplete")

// DeviceTarget is one device's OWN connection facts, resolved from the database
// and with the secret already decrypted. Secret never carries a value that was
// not successfully opened, and must never be logged or serialised.
type DeviceTarget struct {
	DeviceID    string
	Addr        string // host:port
	User        string
	Secret      string
	Kind        string // "password" | "key"
	Fingerprint string // recorded for future host-key pinning; not enforced yet
}

// ── Leak guards ───────────────────────────────────────────────────────────
//
// DeviceTarget holds a DECRYPTED router credential, so the ordinary ways a
// struct escapes into a log line — %v, %+v, %s, json.Marshal — are all leak
// paths. The same discipline already applies to
// router_connections.legacy_password_ciphertext, which the DB reduces to a
// boolean so it can never be serialised; this applies it to the new field.
//
// GoString covers %#v, which bypasses Stringer.

// String renders the target with the secret redacted.
func (t DeviceTarget) String() string {
	return fmt.Sprintf("DeviceTarget{DeviceID:%s Addr:%s User:%s Kind:%s Secret:[REDACTED] Fingerprint:%s}",
		t.DeviceID, t.Addr, t.User, t.Kind, t.Fingerprint)
}

// GoString covers %#v.
func (t DeviceTarget) GoString() string { return t.String() }

// MarshalJSON omits the secret entirely rather than redacting it, so no API
// response or structured log can ever carry it. Note this makes DeviceTarget
// non-round-trippable BY DESIGN: it is a resolved handle, not a transferable
// value. Use SetDeviceCredentials to write credentials.
func (t DeviceTarget) MarshalJSON() ([]byte, error) {
	type safeTarget struct {
		DeviceID       string `json:"device_id"`
		Addr           string `json:"addr"`
		User           string `json:"user"`
		Kind           string `json:"kind"`
		Fingerprint    string `json:"fingerprint,omitempty"`
		HasSecret      bool   `json:"has_secret"`
		SecretRedacted string `json:"secret"`
	}
	return json.Marshal(safeTarget{
		DeviceID:       t.DeviceID,
		Addr:           t.Addr,
		User:           t.User,
		Kind:           t.Kind,
		Fingerprint:    t.Fingerprint,
		HasSecret:      t.Secret != "",
		SecretRedacted: "[REDACTED]",
	})
}

// TargetSource resolves a peer's own connection facts. Implemented by the
// database layer; injected as an interface so the prober is testable without a
// database and so the "resolve per peer" rule is enforced at the seam.
type TargetSource interface {
	TargetForDevice(ctx context.Context, businessID, deviceID string) (*DeviceTarget, error)
}

// SSHConfigFromTarget maps a resolved target onto the transport's config.
//
// NOTE: InsecureIgnoreHostKey stays true. Host-key verification is NOT quietly
// claimed here — the fingerprint is carried for a later slice, and the existing
// guard.go safety layer is unchanged. Storing a fingerprint is the first step
// toward pinning, not pinning itself.
func SSHConfigFromTarget(t *DeviceTarget, timeout time.Duration) (SSHConfig, error) {
	if t == nil {
		return SSHConfig{}, ErrNoTarget
	}
	if strings.TrimSpace(t.Addr) == "" || strings.TrimSpace(t.User) == "" || t.Secret == "" {
		return SSHConfig{}, ErrIncompleteTarget
	}
	cfg := SSHConfig{
		Addr:                  t.Addr,
		User:                  t.User,
		InsecureIgnoreHostKey: true,
		Timeout:               timeout,
	}
	switch t.Kind {
	case "password":
		cfg.Password = t.Secret
	case "key":
		cfg.PrivateKeyPEM = []byte(t.Secret)
	default:
		return SSHConfig{}, fmt.Errorf("%w: unknown secret kind %q", ErrIncompleteTarget, t.Kind)
	}
	return cfg, nil
}

// RunnerFactory builds a command runner for one device's config. It exists so
// tests can prove per-peer routing without opening an SSH connection.
type RunnerFactory func(SSHConfig) CommandRunner

// DeviceProber reads RouterOS facts by resolving a transport PER PEER.
//
// This replaces the single-host Prober on the fleet path. The old design bound
// one transport to cfg.Host and reused it for every peerID, so a fleet sweep
// stamped one router's version onto every device in every tenant — fabricated
// per-device security data.
//
// THE INVARIANT THIS TYPE ENFORCES: there is NO fallback transport. If a peer
// cannot be resolved to its own target, the call fails. It never quietly reads
// from a shared host.
type DeviceProber struct {
	src     TargetSource
	factory RunnerFactory
	timeout time.Duration
}

// NewDeviceProber builds the per-peer prober. A nil factory defaults to the
// real SSH transport.
func NewDeviceProber(src TargetSource, timeout time.Duration, factory RunnerFactory) *DeviceProber {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if factory == nil {
		factory = func(c SSHConfig) CommandRunner { return NewSSHTransport(c) }
	}
	return &DeviceProber{src: src, factory: factory, timeout: timeout}
}

// runnerForDevice resolves the peer's own transport. Returning an error here is
// the whole point: a missing target must stop the read, not redirect it.
func (p *DeviceProber) runnerForDevice(ctx context.Context, businessID, peerID string) (CommandRunner, error) {
	if p.src == nil {
		return nil, fmt.Errorf("%w: no target source wired", ErrNoTarget)
	}
	if strings.TrimSpace(peerID) == "" {
		return nil, fmt.Errorf("%w: empty peer id", ErrNoTarget)
	}
	t, err := p.src.TargetForDevice(ctx, businessID, peerID)
	if err != nil {
		return nil, err
	}
	cfg, err := SSHConfigFromTarget(t, p.timeout)
	if err != nil {
		return nil, err
	}
	runner := p.factory(cfg)
	if runner == nil {
		return nil, fmt.Errorf("routeros: nil runner for device %s", peerID)
	}
	return runner, nil
}

// ObserveVersion reads the RouterOS version + channel from THIS peer's device.
// Read-only.
//
// businessID scopes the lookup so a peer id from one tenant cannot be used to
// read another tenant's device row.
func (p *DeviceProber) ObserveVersion(ctx context.Context, businessID, peerID string) (version, channel string, err error) {
	runner, err := p.runnerForDevice(ctx, businessID, peerID)
	if err != nil {
		// Deliberately NOT wrapped into a generic failure: callers need to tell
		// "unconfigured" (ErrNoTarget) from "unreachable".
		return "", "", err
	}
	out, err := runner.Run(ctx, SystemResourceCommand)
	if err != nil {
		return "", "", fmt.Errorf("routeros: probe version for %s: %w", peerID, err)
	}
	// The channel is not a separate field on real RouterOS — it is the
	// parenthesised suffix of `version`. See parseSystemResource.
	version, channel = parseSystemResource(out)
	return version, channel, nil
}

// JoinHostPort builds the SSH address from a stored management address and port.
//
// A stored address may already carry a port (an operator typed "10.0.0.1:2222");
// joining blindly would produce "10.0.0.1:2222:22". An empty address stays
// empty so the caller hits ErrNoTarget rather than dialling ":22".
func JoinHostPort(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	// Defensive: an inet value can arrive as "10.0.0.1/32" (Postgres renders
	// inet with its netmask when cast to text). Joining that would produce the
	// undialable "10.0.0.1/32:22", so drop any mask. The SQL uses host() as
	// well; this guards values that reach here by another route.
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = strings.TrimSpace(host[:i])
		if host == "" {
			return ""
		}
	}
	if port <= 0 {
		port = 22
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host // already host:port
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
