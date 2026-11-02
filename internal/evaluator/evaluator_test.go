package evaluator

import (
	"testing"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/model"
)

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }
func s(v string) *string   { return &v }

func TestBreachedByPolicyKind(t *testing.T) {
	e := &Evaluator{}
	cases := []struct {
		name   string
		policy model.AlertPolicy
		sample sampleEnvelope
		want   bool
	}{
		{"cpu above threshold", model.AlertPolicy{Kind: "cpu_high", Threshold: 80}, sampleEnvelope{CPUPercent: f(80.5)}, true},
		{"cpu equal to threshold is not a breach", model.AlertPolicy{Kind: "cpu_high", Threshold: 80}, sampleEnvelope{CPUPercent: f(80)}, false},
		{"cpu missing from the sample", model.AlertPolicy{Kind: "cpu_high", Threshold: 80}, sampleEnvelope{MemoryPercent: f(99)}, false},
		{"memory above threshold", model.AlertPolicy{Kind: "memory_high", Threshold: 90}, sampleEnvelope{MemoryPercent: f(91)}, true},
		{"memory below threshold", model.AlertPolicy{Kind: "memory_high", Threshold: 90}, sampleEnvelope{MemoryPercent: f(10)}, false},
		{"latency above threshold", model.AlertPolicy{Kind: "latency_high", Threshold: 150}, sampleEnvelope{LatencyMs: i(151)}, true},
		{"latency at threshold", model.AlertPolicy{Kind: "latency_high", Threshold: 150}, sampleEnvelope{LatencyMs: i(150)}, false},
		{"wan down", model.AlertPolicy{Kind: "wan_down"}, sampleEnvelope{WANState: s("down")}, true},
		{"wan up", model.AlertPolicy{Kind: "wan_down"}, sampleEnvelope{WANState: s("up")}, false},
		{"wan unknown", model.AlertPolicy{Kind: "wan_down"}, sampleEnvelope{}, false},
		{"tunnel down", model.AlertPolicy{Kind: "tunnel_down"}, sampleEnvelope{TunnelState: s("down")}, true},
		{"tunnel up", model.AlertPolicy{Kind: "tunnel_down"}, sampleEnvelope{TunnelState: s("up")}, false},
		{"unknown kind never fires", model.AlertPolicy{Kind: "something_else", Threshold: 0}, sampleEnvelope{CPUPercent: f(100), WANState: s("down")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := e.breached(&c.policy, &c.sample); got != c.want {
				t.Fatalf("breached = %v, want %v", got, c.want)
			}
		})
	}
}

// Malformed or irrelevant messages must be dropped before any repository is touched; the repositories are nil here,
// so reaching them would panic.
func TestHandlersDropMalformedAndIrrelevantMessages(t *testing.T) {
	e := &Evaluator{logger: zap.NewNop().Sugar()}
	for name, m := range map[string]*nats.Msg{
		"sample, short subject": {Subject: "mm", Data: []byte(`{}`)},
		"sample, bad json":      {Subject: "mm.biz.telemetry.ingest", Data: []byte(`{not json`)},
		"sample, no subject id": {Subject: "mm.biz.telemetry.ingest", Data: []byte(`{"cpu_pct":99}`)},
	} {
		t.Run(name, func(t *testing.T) { e.handleSample(m) })
	}
	for name, m := range map[string]*nats.Msg{
		"peer, short subject": {Subject: "mm", Data: []byte(`{}`)},
		"peer, bad json":      {Subject: "mm.biz.monitor.peer.state", Data: []byte(`nope`)},
		"peer, no peer id":    {Subject: "mm.biz.monitor.peer.state", Data: []byte(`{"state":"offline"}`)},
		"peer, healthy state": {Subject: "mm.biz.monitor.peer.state", Data: []byte(`{"peer_id":"p1","state":"online"}`)},
		"peer, unknown state": {Subject: "mm.biz.monitor.peer.state", Data: []byte(`{"peer_id":"p1","state":"recovering"}`)},
	} {
		t.Run(name, func(t *testing.T) { e.handlePeerState(m) })
	}
}

func TestNewIDIsRandomHex(t *testing.T) {
	a, b := newID(), newID()
	if len(a) != 32 || a == b {
		t.Fatalf("ids %q %q", a, b)
	}
}
