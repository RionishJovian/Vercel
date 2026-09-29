package routeros_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"mega-magic/internal/database"
	"mega-magic/internal/provisioner"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// TestHardwareCertification_ProductionPathOnDisposableRouter certifies the
// exact fleet control chain used by the server:
//
// PostgreSQL row -> AES-GCM credential open -> tenant/device resolution ->
// pinned SSH handshake -> RouterOS snapshot -> mutation -> read-back -> restore.
//
// It is opt-in and mutation-capable. Safety is fail-closed: all variables must
// be present, the resolved target must be a private IP inside the operator's
// explicit lab CIDR, and the production denylist is checked before snapshot or
// mutation. A public hostname/IP can never pass this certification test.
func TestHardwareCertification_ProductionPathOnDisposableRouter(t *testing.T) {
	required := []string{
		"MM_CERT_DSN",
		"MM_CERT_CRED_KEY",
		"MM_CERT_BUSINESS",
		"MM_CERT_DEVICE",
		"MM_CERT_LAB_CIDR",
		"MM_CERT_ROTATE_PORT",
	}
	values := make(map[string]string, len(required))
	set := 0
	for _, key := range required {
		values[key] = strings.TrimSpace(os.Getenv(key))
		if values[key] != "" {
			set++
		}
	}
	if set == 0 {
		t.Skip("hardware certification skipped: use scripts/certify-routeros-hardware.sh with a disposable CHR and throwaway/test database")
	}
	if set != len(required) {
		var missing []string
		for _, key := range required {
			if values[key] == "" {
				missing = append(missing, key)
			}
		}
		t.Fatalf("partial hardware certification configuration; missing %s", strings.Join(missing, ", "))
	}

	rotatePort, err := strconv.Atoi(values["MM_CERT_ROTATE_PORT"])
	if err != nil || rotatePort < 1 || rotatePort > 65535 {
		t.Fatalf("MM_CERT_ROTATE_PORT must be an integer from 1 to 65535")
	}
	iface := strings.TrimSpace(os.Getenv("MM_CERT_WG_INTERFACE"))
	if iface == "" {
		iface = routeros.DefaultInterface
	}
	if err := routeros.ValidateDisposableCIDR(values["MM_CERT_LAB_CIDR"]); err != nil {
		t.Fatalf("MM_CERT_LAB_CIDR safety: %v", err)
	}

	raw, err := sql.Open("postgres", values["MM_CERT_DSN"])
	if err != nil {
		t.Fatalf("open certification database: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		t.Fatalf("ping certification database: %v", err)
	}

	box, err := secretbox.New(values["MM_CERT_CRED_KEY"])
	if err != nil {
		t.Fatalf("credential key: %v", err)
	}
	src := database.NewDeviceCredentialSource(&database.DB{DB: raw}, box)
	policySource, err := routeros.NewCIDRTargetSource(src, []string{values["MM_CERT_LAB_CIDR"]})
	if err != nil {
		t.Fatalf("certification target policy: %v", err)
	}
	target, err := policySource.TargetForDevice(ctx, values["MM_CERT_BUSINESS"], values["MM_CERT_DEVICE"])
	if err != nil {
		t.Fatalf("resolve tenant/device target: %v", err)
	}
	if err := requireDisposableTarget(target, values["MM_CERT_LAB_CIDR"]); err != nil {
		t.Fatalf("certification safety gate: %v", err)
	}

	prober := routeros.NewDeviceProber(policySource, 15*time.Second, nil).
		WithControlConfig(iface, "", 3*time.Minute)
	applier := routeros.NewDeviceApplier(policySource, 15*time.Second, nil, iface, rotatePort, 0).
		WithControlConfig("", 3*time.Minute)

	if _, _, err := prober.ObserveVersion(ctx, "00000000-0000-0000-0000-000000000000", values["MM_CERT_DEVICE"]); !errors.Is(err, routeros.ErrNoTarget) {
		t.Fatalf("cross-tenant device resolution was not refused: %v", err)
	}
	version, channel, err := prober.ObserveVersion(ctx, values["MM_CERT_BUSINESS"], values["MM_CERT_DEVICE"])
	if err != nil {
		t.Fatalf("pinned SSH identity/version read: %v", err)
	}
	if !rosVersionPrefix.MatchString(version) || channel == "" {
		t.Fatalf("invalid RouterOS identity response: version=%q channel=%q", version, channel)
	}

	operation, err := applier.BindOperation(ctx, values["MM_CERT_BUSINESS"], values["MM_CERT_DEVICE"])
	if err != nil {
		t.Fatalf("bind immutable certification operation: %v", err)
	}
	before, _, err := operation.Observe(ctx)
	if err != nil {
		t.Fatalf("read WireGuard baseline: %v", err)
	}
	if before.ListenPort < 1 {
		t.Fatalf("WireGuard interface %q has no usable listen port", iface)
	}
	if before.ListenPort == rotatePort {
		t.Fatalf("MM_CERT_ROTATE_PORT %d equals the current port; choose a different lab port", rotatePort)
	}

	change := []provisioner.Change{{Action: provisioner.ActionRotateListenPort}}
	snapshot, err := operation.Snapshot(ctx, change)
	if err != nil {
		t.Fatalf("capture RouterOS rollback snapshot: %v", err)
	}
	restored := false
	t.Cleanup(func() {
		if restored {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		if err := operation.Restore(cleanupCtx, snapshot); err != nil {
			t.Errorf("EMERGENCY RESTORE FAILED: %v", err)
		}
	})

	if err := operation.Apply(ctx, change); err != nil {
		t.Fatalf("apply lab port rotation: %v", err)
	}
	afterApply, _, err := operation.Observe(ctx)
	if err != nil {
		t.Fatalf("read back mutation: %v", err)
	}
	if afterApply.ListenPort != rotatePort {
		t.Fatalf("mutation did not land: listen port=%d, want %d", afterApply.ListenPort, rotatePort)
	}

	if err := operation.Restore(ctx, snapshot); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	final, _, err := operation.Observe(ctx)
	if err != nil {
		t.Fatalf("verify restored state: %v", err)
	}
	if final.ListenPort != before.ListenPort {
		t.Fatalf("restore mismatch: listen port=%d, want original %d", final.ListenPort, before.ListenPort)
	}
	restored = true

	t.Logf("CERTIFIED disposable RouterOS %s (%s): tenant isolation, pinned SSH, snapshot, apply, read-back, and restore passed", version, channel)
}

func requireDisposableTarget(target *routeros.DeviceTarget, labCIDR string) error {
	if target == nil {
		return routeros.ErrNoTarget
	}
	host, portText, err := net.SplitHostPort(target.Addr)
	if err != nil {
		return fmt.Errorf("resolved address %q is not host:port: %w", target.Addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("resolved SSH port %q is invalid", portText)
	}
	ip := net.ParseIP(host)
	if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
		return fmt.Errorf("resolved target %q must be a private or loopback IP, not a hostname or public address", host)
	}
	_, allowed, err := net.ParseCIDR(labCIDR)
	if err != nil {
		return fmt.Errorf("MM_CERT_LAB_CIDR %q is invalid: %w", labCIDR, err)
	}
	if !allowed.Contains(ip) {
		return fmt.Errorf("resolved target %s is outside explicit lab CIDR %s", ip, allowed)
	}
	if err := routeros.GuardTarget(host, port); err != nil {
		return err
	}
	if err := routeros.ValidateHostKeyFingerprint(target.Fingerprint); err != nil {
		return fmt.Errorf("target does not have a valid pinned host key: %w", err)
	}
	return nil
}

func TestHardwareCertificationSafetyGate(t *testing.T) {
	validFingerprint := "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tests := []struct {
		name   string
		target *routeros.DeviceTarget
		cidr   string
		ok     bool
	}{
		{
			name:   "private target inside explicit lab",
			target: &routeros.DeviceTarget{Addr: "172.20.0.250:22", Fingerprint: validFingerprint},
			cidr:   "172.20.0.0/16",
			ok:     true,
		},
		{
			name:   "loopback tunnel inside explicit lab",
			target: &routeros.DeviceTarget{Addr: "127.0.0.1:32222", Fingerprint: validFingerprint},
			cidr:   "127.0.0.0/8",
			ok:     true,
		},
		{
			name:   "public target refused",
			target: &routeros.DeviceTarget{Addr: "203.0.113.10:22", Fingerprint: validFingerprint},
			cidr:   "203.0.113.0/24",
		},
		{
			name:   "hostname refused",
			target: &routeros.DeviceTarget{Addr: "router.test:22", Fingerprint: validFingerprint},
			cidr:   "172.20.0.0/16",
		},
		{
			name:   "outside lab cidr refused",
			target: &routeros.DeviceTarget{Addr: "192.168.1.10:22", Fingerprint: validFingerprint},
			cidr:   "172.20.0.0/16",
		},
		{
			name:   "known production range refused",
			target: &routeros.DeviceTarget{Addr: "10.77.0.4:22", Fingerprint: validFingerprint},
			cidr:   "10.0.0.0/8",
		},
		{
			name:   "unpinned target refused",
			target: &routeros.DeviceTarget{Addr: "172.20.0.250:22", Fingerprint: "SHA256:bad"},
			cidr:   "172.20.0.0/16",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireDisposableTarget(tc.target, tc.cidr)
			if tc.ok && err != nil {
				t.Fatalf("safe lab target rejected: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("unsafe certification target was accepted")
			}
		})
	}
}
