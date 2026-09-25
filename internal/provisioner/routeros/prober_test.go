package routeros

import (
	"context"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

const labPeersIdle = "   persistent-keepalive: 25s\n   allowed-address: 10.99.0.1/32\n        rx: 0\n        tx: 0\n"
const labPeersTraffic = "   persistent-keepalive: 25s\n   allowed-address: 10.99.0.1/32\n        rx: 4096\n        tx: 2048\n"
const labIface = "         name: wg-magichub\n  listen-port: 13231\n"

const pingFail = "    sent=2 received=0 packet-loss=100% \n"
const pingOKOut = "    sent=2 received=2 packet-loss=0% min-rtt=105us avg-rtt=117us \n"

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
			obs, conn, err := p.Observe(context.Background(), "peer2")
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
	obs, conn, err := p.Observe(context.Background(), "peer2")
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
	obs, _, err := p.Observe(context.Background(), "peer2")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.DataPlaneUp {
		t.Fatal("no ping target => data plane must not be reported up")
	}
}

func TestObserve_runnerError(t *testing.T) {
	fr := &fakeRunner{err: map[string]error{"/interface wireguard peers print": errors.New("ssh down")}}
	if _, _, err := NewProber(fr, "wg-magichub", "10.99.0.1", time.Minute).Observe(context.Background(), "peer2"); err == nil {
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
	if got := pingSummary(pingFail); got == "" {
		t.Fatal("pingSummary should return the sent= line")
	}
}
