package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

const labPeersIdle = "   persistent-keepalive: 25s\n   allowed-address: 10.99.0.1/32\n        rx: 0\n        tx: 0\n"
const labPeersTraffic = "   persistent-keepalive: 25s\n   allowed-address: 10.99.0.1/32\n        rx: 4096\n        tx: 2048\n"
const labIface = "         name: wg-magichub\n  listen-port: 13231\n"

const pingFail = "    sent=2 received=0 packet-loss=100% \n"
const pingOKOut = "    sent=2 received=2 packet-loss=0% min-rtt=105us avg-rtt=117us \n"
const pingRowsOnly = "Columns: SEQ, HOST, SIZE, TTL, TIME\n0 10.99.0.1 56 64 2ms627us\n1 10.99.0.1 56 64 2ms179us\n"

func TestObserve_classify(t *testing.T) {
	cases := []struct {
		name      string
		peers     string
		ping      string
		want      string
		wantReach bool
	}{
		{"idle + no reply => offline", labPeersIdle, pingFail, provisioner.StateOffline, false},
		{"traffic + reply => up", labPeersTraffic, pingOKOut, provisioner.StateUp, true},
		{"idle + reply => stale", labPeersIdle, pingOKOut, provisioner.StateStale, true},
		{"unit traffic + row replies => up", "persistent-keepalive: 25s\nrx: 197.6KiB\ntx: 12.5KiB\n", pingRowsOnly, provisioner.StateUp, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRunner{out: map[string]string{
				"/interface wireguard peers print": labPeersIdle,
				"/interface wireguard print":       labIface,
				"/ping":                            tc.ping,
			}}
			fr.out["/interface wireguard peers print"] = tc.peers
			p := NewProber(fr, "wg-magichub", "10.99.0.1", 3*time.Minute)
			obs, conn, err := p.Observe(context.Background(), "biz-1", "peer2")
			if err != nil {
				t.Fatalf("Observe: %v", err)
			}
			if obs.State != tc.want {
				t.Fatalf("state = %q, want %q", obs.State, tc.want)
			}
			if conn.Reachable != tc.wantReach {
				t.Fatalf("reachable = %v, want %v", conn.Reachable, tc.wantReach)
			}
		})
	}
}

func TestObserve_parsesFields(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{
		"/interface wireguard peers print": labPeersIdle,
		"/interface wireguard print":       labIface,
		"/ping":                            pingFail,
	}}
	p := NewProber(fr, "wg-magichub", "10.99.0.1", 3*time.Minute)
	obs, conn, err := p.Observe(context.Background(), "biz-1", "peer2")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.KeepaliveKnown || obs.Keepalive != 25*time.Second {
		t.Fatalf("keepalive = %v known=%v, want 25s known", obs.Keepalive, obs.KeepaliveKnown)
	}
	if obs.ListenPort != 13231 {
		t.Fatalf("listen port = %d, want 13231", obs.ListenPort)
	}
	if !conn.Checked {
		t.Fatal("connectivity must be marked checked")
	}
}

func TestObserve_emptyPingTargetSkipsDataPlane(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{
		"/interface wireguard peers print": labPeersIdle,
		"/interface wireguard print":       labIface,
	}}
	p := NewProber(fr, "wg-magichub", "", 3*time.Minute)
	obs, _, err := p.Observe(context.Background(), "biz-1", "peer2")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.DataPlaneUp {
		t.Fatal("no ping target => data plane must not be reported up")
	}
}

func TestObserve_runnerError(t *testing.T) {
	fr := &fakeRunner{err: map[string]error{"/interface wireguard peers print": errors.New("ssh down")}}
	if _, _, err := NewProber(fr, "wg-magichub", "10.99.0.1", time.Minute).Observe(context.Background(), "biz-1", "peer2"); err == nil {
		t.Fatal("runner error must propagate")
	}
}

func TestParseHelpers(t *testing.T) {
	kv := parseValueList("  key:  value \nflags: R\nplain line\n: emptykey\n")
	if kv["key"] != "value" || kv["flags"] != "R" {
		t.Fatalf("parseValueList = %v", kv)
	}
	if _, ok := kv[""]; ok {
		t.Fatal("empty keys must be dropped")
	}
	if !pingOK(pingOKOut) || pingOK(pingFail) {
		t.Fatal("pingOK misclassified")
	}
	if !pingOK(pingRowsOnly) {
		t.Fatal("pingOK must accept structurally valid RouterOS reply rows without a summary")
	}
	if !trafficPositive("197.6KiB") || !trafficPositive("12.5KiB") || trafficPositive("0B") || trafficPositive("") {
		t.Fatal("trafficPositive misclassified RouterOS unit-formatted counters")
	}
	if got := pingSummary(pingFail); got == "" {
		t.Fatal("pingSummary should return the sent= line")
	}
}

// goldenSystemResourcePrint is the VERBATIM output of `/system/resource print`
// captured from a real RouterOS 7.15.3 (CHR, x86_64). It guards two bugs that
// only a real device exposed:
//
//  1. The command must have NO arguments. `/system/resource` is a SINGLETON
//     menu and rejects `value-list`; the old command
//     (`/system/resource print value-list detail`) returned "expected end of
//     command" against every real router, so version capture could never have
//     worked outside a test double that answered the invalid command.
//  2. There is no `channel` field — the channel is the parenthesised suffix of
//     `version`.
//
// board-name's value contains BOTH a colon and parentheses, and build-time's
// value contains colons, so this also proves the parser splits on the FIRST
// colon and is not confused by parens elsewhere in the block.
const goldenSystemResourcePrint = `                   uptime: 7m54s
                  version: 7.15.3 (stable)
               build-time: 2024-07-24 10:39:01
         factory-software: 7.1
              free-memory: 307.9MiB
             total-memory: 512.0MiB
                      cpu: Intel
                cpu-count: 4
                 cpu-load: 1%
        architecture-name: x86_64
               board-name: CHR QEMU Standard PC (i440FX + PIIX, 1996)
                 platform: MikroTik
`

func TestParseSystemResourceGolden(t *testing.T) {
	// CRLF, exactly as it arrives over SSH.
	out := strings.ReplaceAll(goldenSystemResourcePrint, "\n", "\r\n")
	v, ch := parseSystemResource(out)
	if v != "7.15.3" {
		t.Errorf("version = %q, want 7.15.3", v)
	}
	if ch != "stable" {
		t.Errorf("channel = %q, want stable", ch)
	}
}

// The command text is load-bearing: adding an argument breaks it on every real
// router. Pin it so a future "improvement" cannot silently re-break it.
func TestSystemResourceCommandHasNoArguments(t *testing.T) {
	if SystemResourceCommand != "/system/resource print" {
		t.Fatalf("SystemResourceCommand = %q — /system resource is a singleton menu and rejects print arguments", SystemResourceCommand)
	}
}

func TestParseSystemResourceEdgeCases(t *testing.T) {
	// A version with no parenthesised suffix must still yield the version and
	// an empty channel rather than a mangled value.
	if v, ch := parseSystemResource("version: 7.15.3\n"); v != "7.15.3" || ch != "" {
		t.Fatalf("no-channel: got version=%q channel=%q", v, ch)
	}
	// No version at all → both empty, which callers treat as "no observation".
	if v, ch := parseSystemResource("uptime: 1s\n"); v != "" || ch != "" {
		t.Fatalf("absent version should be empty, got %q/%q", v, ch)
	}
	// Trailing spaces inside the suffix.
	if v, ch := parseSystemResource("version: 7.16 (long-term) \n"); v != "7.16" || ch != "long-term" {
		t.Fatalf("suffixed: got version=%q channel=%q", v, ch)
	}
}
