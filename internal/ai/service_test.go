package ai

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/model"
)

// --- fakes ------------------------------------------------------------------

type fakeBus struct {
	published []pubRec
	pattern   string
}

type pubRec struct {
	biz     string
	subject string
	data    []byte
}

func (b *fakeBus) Publish(biz, subject string, data []byte) error {
	b.published = append(b.published, pubRec{biz, subject, data})
	return nil
}

func (b *fakeBus) SubscribePattern(pattern string, _ func(*nats.Msg)) error {
	b.pattern = pattern
	return nil
}

type fakeMeter struct {
	spent  bool
	err    error
	spends []int
}

func (m *fakeMeter) Spend(_ context.Context, _ string, cost int) (bool, error) {
	m.spends = append(m.spends, cost)
	return m.spent, m.err
}

type stubClient struct {
	finding *Finding
	err     error
	calls   int
}

func (c *stubClient) Analyze(_ context.Context, _ model.TelemetrySample) (*Finding, error) {
	c.calls++
	return c.finding, c.err
}

func testService(bus *fakeBus, meter *fakeMeter, client *stubClient) *Service {
	return NewWithMeter(bus, meter, client, Config{Enabled: true, CostPerAnalysis: 1}, zap.NewNop().Sugar())
}

func sample(id string) model.TelemetrySample {
	dev := "dev-" + id
	return model.TelemetrySample{BusinessID: "biz-" + id, DeviceID: &dev, SubjectKind: "router", SubjectID: id}
}

// --- tests ------------------------------------------------------------------

func TestService_healthyPublishesNothing(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: true}
	svc := testService(bus, meter, &stubClient{finding: nil})

	svc.process(context.Background(), sample("1"))

	if len(bus.published) != 0 {
		t.Fatalf("healthy sample must publish nothing, got %d", len(bus.published))
	}
	if len(meter.spends) != 0 {
		t.Fatalf("healthy sample must not spend, got %d spends", len(meter.spends))
	}
}

func TestService_criticalPublishesAndSpendsExactlyCost(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: true}
	svc := testService(bus, meter, &stubClient{finding: &Finding{Rule: "latency_spike", Severity: "critical"}})

	svc.process(context.Background(), sample("2"))

	if len(bus.published) != 1 {
		t.Fatalf("critical sample must publish once, got %d", len(bus.published))
	}
	if bus.published[0].subject != "ai.finding" {
		t.Fatalf("subject = %q, want ai.finding", bus.published[0].subject)
	}
	if len(meter.spends) != 1 || meter.spends[0] != 1 {
		t.Fatalf("must spend exactly cost=1, got spends=%v", meter.spends)
	}
}

func TestService_exhaustedQuotaNoSpendNoPublish(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: false} // balance exhausted
	svc := testService(bus, meter, &stubClient{finding: &Finding{Rule: "cpu_high", Severity: "warn"}})

	svc.process(context.Background(), sample("3"))

	if len(bus.published) != 0 {
		t.Fatalf("exhausted quota must not publish, got %d", len(bus.published))
	}
	if len(meter.spends) != 1 {
		t.Fatalf("meter is consulted once (spent=false), got %d", len(meter.spends))
	}
}

func TestService_analysisErrorNoSpendNoPublish(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: true}
	svc := testService(bus, meter, &stubClient{err: context.DeadlineExceeded})

	svc.process(context.Background(), sample("4"))

	if len(bus.published) != 0 {
		t.Fatalf("analysis error must not publish, got %d", len(bus.published))
	}
	if len(meter.spends) != 0 {
		t.Fatalf("analysis error must not spend, got %d", len(meter.spends))
	}
}

func TestService_dedupeSameFindingTriple(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: true}
	svc := testService(bus, meter, &stubClient{finding: &Finding{Rule: "latency_spike", Severity: "critical"}})

	s := sample("5")
	svc.process(context.Background(), s)
	svc.process(context.Background(), s) // repeat, unresolved

	if len(bus.published) != 1 {
		t.Fatalf("dedupe: expected exactly 1 publish, got %d", len(bus.published))
	}
	if len(meter.spends) != 1 {
		t.Fatalf("dedupe: expected exactly 1 spend, got %d", len(meter.spends))
	}
}

func TestService_healthyClearsDedupe(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: true}
	client := &stubClient{finding: &Finding{Rule: "latency_spike", Severity: "critical"}}
	svc := testService(bus, meter, client)

	s := sample("6")
	svc.process(context.Background(), s) // fires
	client.finding = nil
	svc.process(context.Background(), s) // healthy -> clears
	client.finding = &Finding{Rule: "latency_spike", Severity: "critical"}
	svc.process(context.Background(), s) // fires again

	if len(bus.published) != 2 {
		t.Fatalf("expected 2 publishes across a clear, got %d", len(bus.published))
	}
}

func TestNextBalance_neverBelowZero(t *testing.T) {
	cases := []struct{ balance, cost, want int }{
		{5, 1, 4},
		{1, 1, 0},
		{0, 1, 0},
		{1, 5, 0},
		{3, 0, 3},
		{3, -2, 3},
	}
	for _, c := range cases {
		if got := nextBalance(c.balance, c.cost); got != c.want {
			t.Fatalf("nextBalance(%d,%d) = %d, want %d", c.balance, c.cost, got, c.want)
		}
	}
}

func TestService_startGatedOffDoesNotSubscribe(t *testing.T) {
	bus := &fakeBus{}
	svc := NewWithMeter(bus, &fakeMeter{spent: true}, &stubClient{}, Config{Enabled: false}, zap.NewNop().Sugar())
	if err := svc.Start(); err != nil {
		t.Fatalf("gated Start must return nil, got %v", err)
	}
	if bus.pattern != "" {
		t.Fatalf("gated-off service must not subscribe, got pattern %q", bus.pattern)
	}
}

func TestService_startEnabledSubscribes(t *testing.T) {
	bus := &fakeBus{}
	svc := NewWithMeter(bus, &fakeMeter{spent: true}, &stubClient{}, Config{Enabled: true}, zap.NewNop().Sugar())
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if bus.pattern != "telemetry.ingest" {
		t.Fatalf("pattern = %q, want telemetry.ingest", bus.pattern)
	}
}
