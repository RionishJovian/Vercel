package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/model"
)

// --- fakes ------------------------------------------------------------------

type fakeIncidentStore struct {
	created bool
	err     error
	got     []model.Incident
	calls   int
}

func (f *fakeIncidentStore) CreateIncident(_ string, inc model.Incident) (bool, error) {
	f.calls++
	f.got = append(f.got, inc)
	return f.created, f.err
}

type fakeNotifier struct {
	calls  int
	opened []model.Incident
}

func (f *fakeNotifier) IncidentOpened(_ string, inc *model.Incident) {
	f.calls++
	f.opened = append(f.opened, *inc)
}

func consumerService(store IncidentStore, n IncidentNotifier) *Service {
	return NewWithMeter(&fakeBus{}, &fakeMeter{spent: true}, &stubClient{}, Config{Enabled: true, CostPerAnalysis: 1}, zap.NewNop().Sugar()).
		WithIncidentStore(store).
		WithNotifier(n)
}

// --- tests ------------------------------------------------------------------

func TestConsumeFinding_opensIncident(t *testing.T) {
	store := &fakeIncidentStore{created: true}
	nf := &fakeNotifier{}
	svc := consumerService(store, nf)

	s := sample("1") // biz-1 / dev-1
	f := &Finding{Rule: "latency_spike", Severity: "critical", Summary: "latency 500ms"}
	svc.consumeFinding(s, f)

	if store.calls != 1 {
		t.Fatalf("store calls = %d, want 1", store.calls)
	}
	if nf.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", nf.calls)
	}
	inc := nf.opened[0]
	if inc.Kind != "ai_latency_spike" {
		t.Fatalf("kind = %q, want ai_latency_spike", inc.Kind)
	}
	if inc.Severity != "critical" {
		t.Fatalf("severity = %q, want critical (passthrough)", inc.Severity)
	}
	if inc.Detail == nil || *inc.Detail != "latency 500ms" {
		t.Fatalf("detail = %v, want summary", inc.Detail)
	}
	if inc.BusinessID != s.BusinessID {
		t.Fatalf("business = %q", inc.BusinessID)
	}
	if inc.SubjectID != "dev-1" {
		t.Fatalf("subject = %q, want dev-1 (device anchor)", inc.SubjectID)
	}
	if inc.PolicyID != nil {
		t.Fatal("AI finding incident must have nil PolicyID")
	}
	if inc.OpenedAt.IsZero() || inc.OpenedAt.Location() != time.UTC {
		t.Fatalf("opened_at must be UTC now, got %v", inc.OpenedAt)
	}
	if inc.ID == "" {
		t.Fatal("incident must carry an id")
	}
}

func TestConsumeFinding_dedupedByStore(t *testing.T) {
	store := &fakeIncidentStore{created: false} // already open
	nf := &fakeNotifier{}
	svc := consumerService(store, nf)

	svc.consumeFinding(sample("2"), &Finding{Rule: "cpu_high", Severity: "warn"})

	if store.calls != 1 {
		t.Fatalf("store calls = %d, want 1", store.calls)
	}
	if nf.calls != 0 {
		t.Fatalf("already-open incident must NOT notify, got %d", nf.calls)
	}
}

func TestConsumeFinding_storeFailureIsFailClosed(t *testing.T) {
	store := &fakeIncidentStore{err: context.DeadlineExceeded}
	nf := &fakeNotifier{}
	svc := consumerService(store, nf)

	// Must not panic and must not notify.
	svc.consumeFinding(sample("3"), &Finding{Rule: "latency_spike", Severity: "critical"})

	if nf.calls != 0 {
		t.Fatalf("store failure must not notify, got %d", nf.calls)
	}
}

func TestFindingToIncident_severityNormalized(t *testing.T) {
	inc := findingToIncident(sample("4"), &Finding{Rule: "x", Severity: ""}, time.Now().UTC())
	if inc.Severity != "info" {
		t.Fatalf("empty severity => info, got %q", inc.Severity)
	}
	// unknown severity also normalizes to info.
	inc = findingToIncident(sample("4"), &Finding{Rule: "x", Severity: "banana"}, time.Now().UTC())
	if inc.Severity != "info" {
		t.Fatalf("unknown severity => info, got %q", inc.Severity)
	}
}

func TestSanitizeRule_toAlphanumericUnderscore(t *testing.T) {
	got := sanitizeRule("latency spike!")
	if got != "latency_spike" {
		t.Fatalf("sanitizeRule = %q, want latency_spike", got)
	}
	if s := sanitizeRule("CPU>85%"); s != "cpu_85" {
		t.Fatalf("sanitizeRule = %q, want cpu_85", s)
	}
	if s := sanitizeRule(""); s != "unknown" {
		t.Fatalf("empty rule => unknown, got %q", s)
	}
}

// TestService_processOpensIncident couples the whole path: analyse -> spend ->
// publish -> open incident.
func TestService_processOpensIncident(t *testing.T) {
	bus := &fakeBus{}
	meter := &fakeMeter{spent: true}
	store := &fakeIncidentStore{created: true}
	nf := &fakeNotifier{}
	svc := NewWithMeter(bus, meter, &stubClient{finding: &Finding{Rule: "latency_spike", Severity: "critical"}},
		Config{Enabled: true, CostPerAnalysis: 1}, zap.NewNop().Sugar()).
		WithIncidentStore(store).
		WithNotifier(nf)

	svc.process(context.Background(), sample("9"))

	if len(bus.published) != 1 || bus.published[0].subject != "ai.finding" {
		t.Fatalf("expected one ai.finding publish, got %+v", bus.published)
	}
	if store.calls != 1 || nf.calls != 1 {
		t.Fatalf("expected incident opened once: store=%d notifier=%d", store.calls, nf.calls)
	}
	// Envelope carries the device context.
	payload := string(bus.published[0].data)
	if !strings.Contains(payload, "\"device_id\":\"dev-9\"") {
		t.Fatalf("envelope missing device_id: %s", payload)
	}
}

func TestService_processHealthyDoesNotOpenIncident(t *testing.T) {
	bus := &fakeBus{}
	store := &fakeIncidentStore{created: true}
	nf := &fakeNotifier{}
	svc := NewWithMeter(bus, &fakeMeter{spent: true}, &stubClient{finding: nil},
		Config{Enabled: true, CostPerAnalysis: 1}, zap.NewNop().Sugar()).
		WithIncidentStore(store).
		WithNotifier(nf)

	svc.process(context.Background(), sample("10"))

	if store.calls != 0 || nf.calls != 0 {
		t.Fatalf("healthy sample must open nothing: store=%d notifier=%d", store.calls, nf.calls)
	}
}
