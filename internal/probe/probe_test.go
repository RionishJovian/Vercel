package probe

import "testing"

// classifyFreshness is the pure peer-state decision table. Test every branch:
// the four reachable combinations (up/stale/offline) plus the unknown default.
func TestClassifyFreshness(t *testing.T) {
	// up: handshake fresh AND data plane up
	if got := classifyFreshness(true, true); got != "up" {
		t.Fatalf("fresh+up -> %q, want %q", got, "up")
	}
	// stale: handshake expired BUT data plane still up
	if got := classifyFreshness(false, true); got != "stale" {
		t.Fatalf("expired+up -> %q, want %q", got, "stale")
	}
	// offline: handshake expired AND data plane down
	if got := classifyFreshness(false, false); got != "offline" {
		t.Fatalf("expired+down -> %q, want %q", got, "offline")
	}
	// unknown: fresh handshake but data plane down cannot be "up" (no telemetry
	// yet) — the fallback.
	if got := classifyFreshness(true, false); got != "unknown" {
		t.Fatalf("fresh+down -> %q, want %q", got, "unknown")
	}
}