package routeros

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

// labAddrForTest returns the lab SSH address from the environment, or skips.
//
// The lab integration test is opt-in (env-gated) so `go test ./...` stays
// deterministic and never depends on a live box. When the env var is absent the
// test SKIPS LOUDLY with the exact command to run it. It never silently passes.
func labAddrForTest(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MM_LAB_SSH_ADDR")
	if addr == "" {
		t.Skip("lab integration skipped: set MM_LAB_SSH_ADDR=<lab-host>:22 (and optionally MM_LAB_SSH_USER, MM_LAB_SSH_PASSWORD) to run against the disposable lab router; requires the sandbox lab only, never production")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("MM_LAB_SSH_ADDR must be host:port: %v", err)
	}
	// The lab test must never be pointed at production.
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	if err := GuardTarget(host, port); err != nil {
		t.Fatalf("refusing to run the lab test against a prod-shaped target: %v", err)
	}
	return addr
}

func labTransport(t *testing.T) *SSHTransport {
	t.Helper()
	addr := labAddrForTest(t)
	tr := SSHTransport{cfg: SSHConfig{
		Addr:                  addr,
		User:                  envOr("MM_LAB_SSH_USER", "admin"),
		Password:              os.Getenv("MM_LAB_SSH_PASSWORD"),
		InsecureIgnoreHostKey: true,
		Timeout:               10 * time.Second,
	}}
	// Fail fast with a clear reason if the lab is down.
	if _, err := tr.Run(context.Background(), "/system identity print"); err != nil {
		t.Skipf("lab integration skipped: lab SSH %s unreachable: %v", addr, err)
	}
	return &tr
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

const (
	labIfaceName      = "wg-magichub"
	labRotatePort     = 13241
	labOriginalPort   = 13231
	labPeerPingTarget = "10.99.0.1"
)

// TestLab_SnapshotApplyVerifyRollback proves the full live path on real
// RouterOS: snapshot -> apply (translated change) -> verify (real prober) ->
// auto-rollback within the 60s window, leaving the lab reverted and the peer
// enabled. Sandbox 10.99.0.0/30 only.
func TestLab_SnapshotApplyVerifyRollback(t *testing.T) {
	tr := labTransport(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	ap := NewApplier(tr, labIfaceName, labRotatePort, 0)
	pr := NewProber(tr, labIfaceName, labPeerPingTarget, 3*time.Minute)

	// Pre-state must be the documented lab baseline.
	before, _, err := pr.Observe(ctx, "peer2")
	if err != nil {
		t.Fatalf("observe before: %v", err)
	}
	if before.ListenPort != labOriginalPort {
		t.Fatalf("lab baseline listen-port = %d, want %d", before.ListenPort, labOriginalPort)
	}

	// 1) Snapshot (real /export + /system backup save + WG capture).
	snap, err := ap.Snapshot(ctx, "peer2", []provisioner.Change{{Action: provisioner.ActionRotateListenPort}})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.ExportFile == "" || snap.WGListenPort != labOriginalPort {
		t.Fatalf("snapshot missing data: %+v", snap)
	}

	// 2) Direct apply, then read back — proves the translation really writes.
	if err := ap.Apply(ctx, "peer2", []provisioner.Change{{Action: provisioner.ActionRotateListenPort}}); err != nil {
		t.Fatalf("apply rotate: %v", err)
	}
	mid, _, err := pr.Observe(ctx, "peer2")
	if err != nil {
		t.Fatalf("observe after apply: %v", err)
	}
	if mid.ListenPort != labRotatePort {
		t.Fatalf("after apply listen-port = %d, want %d", mid.ListenPort, labRotatePort)
	}

	// 3) Direct restore — proves the revert path.
	if err := ap.Restore(ctx, snap); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, _, err := pr.Observe(ctx, "peer2")
	if err != nil {
		t.Fatalf("observe after restore: %v", err)
	}
	if restored.ListenPort != labOriginalPort {
		t.Fatalf("after restore listen-port = %d, want %d", restored.ListenPort, labOriginalPort)
	}

	// 4) Full gated path: ApplyService takes the snapshot, applies, verifies via
	//    the real prober (offline on the lab, no hub counterpart), and must
	//    auto-rollback within the 60s window.
	svc := provisioner.NewApplyService(ap, pr)
	start := time.Now()
	res, err := svc.ApplyFixes(ctx, provisioner.ApplyRequest{
		BusinessID: "lab",
		PeerID:     "peer2",
		Changes:    []provisioner.Change{{Action: provisioner.ActionRotateListenPort}},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if !res.SnapshotCaptured {
		t.Fatal("apply must snapshot before writing")
	}
	if res.Verified {
		t.Fatalf("lab has no WG counterpart; verification must NOT pass: %+v", res)
	}
	if !res.RolledBack {
		t.Fatalf("verify failure must roll back: %+v", res)
	}
	if res.DeadlineExceeded {
		t.Fatalf("rollback must finish inside the 60s window: %+v", res)
	}
	if elapsed > 60*time.Second {
		t.Fatalf("rollback exceeded the 60s window: %s", elapsed)
	}

	// Final device state: reverted to baseline and peer enabled.
	final, _, err := pr.Observe(ctx, "peer2")
	if err != nil {
		t.Fatalf("observe final: %v", err)
	}
	if final.ListenPort != labOriginalPort {
		t.Fatalf("final listen-port = %d, want %d (rollback must revert)", final.ListenPort, labOriginalPort)
	}
}

// TestLab_ProdGuardRefusesProd documents, on the live box, that a prod-shaped
// target is refused before any connection is attempted.
func TestLab_ProdGuardRefusesProd(t *testing.T) {
	if os.Getenv("MM_LAB_SSH_ADDR") == "" {
		t.Skip("lab integration skipped: set MM_LAB_SSH_ADDR to run the guard-on-live check")
	}
	if _, _, err := NewLive(LiveConfig{Enabled: true, Host: "10.77.0.4", Port: 22, User: "admin", AllowEmptyPassword: true}); err == nil {
		t.Fatal("prod target must be refused even with the live flag on")
	}
}

// TestLab_EnforceGuestQuotaLive proves Phase B2 enforcement on real hardware:
// it creates a hotspot user with a data+time cap, verifies the caps are applied
// (via the real Prober), then revokes the user and verifies it's gone. Restores
// the lab to its pre-test state regardless of failure. Sandbox 10.99.0.0/30.
func TestLab_EnforceGuestQuotaLive(t *testing.T) {
	tr := labTransport(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	const identity = "B2LAB:00:11:22:33:44:55"
	name := guestUserName(identity)
	pr := NewProber(tr, "wg-magichub", "", 0)
	ap := NewApplier(tr, "wg-magichub", 0, 0)

	// Clean guarantee: start from a known-empty state.
	if err := ap.Apply(ctx, name, []provisioner.Change{{Action: provisioner.ActionRevokeGuestQuota, Target: identity}}); err != nil {
		t.Fatalf("pre-clean revoke: %v", err)
	}
	if st, err := pr.ObserveGuestQuota(ctx, identity); err != nil || st.Exists {
		t.Fatalf("pre-test user must not exist: exists=%v err=%v", st.Exists, err)
	}

	// 1) Enforce: a bought "1GB / 60min" plan.
	err := ap.Apply(ctx, name, []provisioner.Change{{
		Action:   provisioner.ActionEnforceGuestQuota,
		Target:   identity,
		Proposed: `{"data_bytes":1073741824,"uptime_seconds":3600}`,
	}})
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}

	// 2) Verify the caps are ACTUALLY on the router.
	mid, err := pr.ObserveGuestQuota(ctx, identity)
	if err != nil {
		t.Fatalf("observe after enforce: %v", err)
	}
	if !mid.Exists {
		t.Fatalf("enforcement user missing after apply: %+v", mid)
	}
	if mid.LimitBytes != 1073741824 || mid.LimitUptime != 3600 {
		t.Fatalf("caps not enforced: got %+v, want data=1GiB uptime=3600s", mid)
	}

	// 3) Revoke (plan lapsed / rejected) and verify it's gone.
	if err := ap.Apply(ctx, name, []provisioner.Change{{Action: provisioner.ActionRevokeGuestQuota, Target: identity}}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	final, err := pr.ObserveGuestQuota(ctx, identity)
	if err != nil {
		t.Fatalf("observe after revoke: %v", err)
	}
	if final.Exists {
		t.Fatalf("user must be removed after revoke: %+v", final)
	}
}
