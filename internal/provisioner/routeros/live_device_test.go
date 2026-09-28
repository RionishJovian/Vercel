// Package routeros_test holds the REAL-HARDWARE integration proof.
//
// WHY THIS FILE EXISTS: every other test in this package substitutes a fake
// CommandRunner, and a fake will happily "answer" a command that no real RouterOS
// accepts. That is exactly how `/system/resource print value-list detail` shipped
// broken — it failed on every physical device while the suite stayed green. This
// test removes the double: it drives the REAL per-device transport (encrypted
// credentials -> secretbox -> SSH -> a genuine RouterOS) and requires a genuine
// version back.
//
// It is opt-in (it needs a live database AND a reachable router), so the default
// `go test ./...` stays green without either:
//
//	MM_LIVE_DSN='host=... port=5433 user=... password=... dbname=... sslmode=disable' \
//	MM_LIVE_CRED_KEY='<base64 or hex 32-byte key>' \
//	MM_LIVE_BUSINESS='<uuid>' MM_LIVE_DEVICE='<uuid>' \
//	  go test ./internal/provisioner/routeros/ -run LiveDevice -v
//
// It WRITES NOTHING: ObserveVersion only reads the router; the credential source
// only reads the device row.
package routeros_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"mega-magic/internal/database"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// liveProber builds the REAL production path: the encrypted credential store
// backed by the live database, and a DeviceProber with a nil factory — which
// NewDeviceProber turns into the real SSH transport.
func liveProber(t *testing.T) (p *routeros.DeviceProber, business, device string) {
	t.Helper()

	dsn := os.Getenv("MM_LIVE_DSN")
	key := os.Getenv("MM_LIVE_CRED_KEY")
	business = os.Getenv("MM_LIVE_BUSINESS")
	device = os.Getenv("MM_LIVE_DEVICE")
	if dsn == "" || key == "" || business == "" || device == "" {
		t.Skip("MM_LIVE_DSN / MM_LIVE_CRED_KEY / MM_LIVE_BUSINESS / MM_LIVE_DEVICE not all set; skipping real-router test")
	}

	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := raw.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("credential key: %v", err)
	}

	src := database.NewDeviceCredentialSource(&database.DB{DB: raw}, box)
	return routeros.NewDeviceProber(src, 15*time.Second, nil), business, device
}

// rosVersionPrefix matches the leading "major.minor" of a RouterOS version.
var rosVersionPrefix = regexp.MustCompile(`^\d+\.\d+`)

// TestLiveDeviceObserveVersion is the missing proof: a version read from a REAL
// router through the REAL transport.
func TestLiveDeviceObserveVersion(t *testing.T) {
	p, business, device := liveProber(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	version, channel, err := p.ObserveVersion(ctx, business, device)
	if err != nil {
		t.Fatalf("ObserveVersion against the real router failed: %v", err)
	}
	t.Logf("real router reported version=%q channel=%q", version, channel)

	if !rosVersionPrefix.MatchString(version) {
		t.Errorf("version %q does not look like a RouterOS version (want e.g. \"7.15.3\")", version)
	}
	// RouterOS renders the channel as the parenthesised suffix of `version`
	// ("7.15.3 (stable)"); an empty channel means that split is broken.
	if channel == "" {
		t.Errorf("channel is empty — the parenthesised suffix of the version was not extracted")
	}
}

// TestLiveDeviceUnknownDeviceIsRefused proves the prober never fabricates: a
// device with no stored target must FAIL, not fall back to some other device's
// transport (which is how one router's version would land on another's row).
func TestLiveDeviceUnknownDeviceIsRefused(t *testing.T) {
	p, business, _ := liveProber(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := p.ObserveVersion(ctx, business, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("an unknown device must NOT resolve to a transport")
	}
	if !errors.Is(err, routeros.ErrNoTarget) {
		t.Fatalf("want ErrNoTarget for an unconfigured device, got: %v", err)
	}
}
