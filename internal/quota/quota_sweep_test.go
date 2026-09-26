package quota

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/database"
)

type fakeLister struct {
	rows []database.PassExpiry
}

func (f fakeLister) ListAllQuotas() ([]database.PassExpiry, error) { return f.rows, nil }

type recPublisher struct {
	mu     sync.Mutex
	events []published
}

type published struct {
	biz     string
	subject string
	data    []byte
}

func (p *recPublisher) Publish(biz, subject string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, published{biz, subject, data})
	return nil
}

func (p *recPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *recPublisher) first() (published, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return published{}, false
	}
	return p.events[0], true
}

func TestSweep_emitsOnlyExpired(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	pub := &recPublisher{}
	s := NewSweep(fakeLister{rows: []database.PassExpiry{
		{BusinessID: "biz-past", ExpiresAt: &past},
		{BusinessID: "biz-future", ExpiresAt: &future},
		{BusinessID: "biz-none", ExpiresAt: nil},
	}}, pub, zap.NewNop().Sugar(), time.Minute)
	s.now = func() time.Time { return now }

	s.pass()

	if pub.count() != 1 {
		t.Fatalf("emissions = %d, want 1 (only the expired pass)", pub.count())
	}
	ev, _ := pub.first()
	if ev.biz != "biz-past" {
		t.Fatalf("emitted for %q, want biz-past", ev.biz)
	}
	if ev.subject != "quota.pass.expired" {
		t.Fatalf("subject = %q, want quota.pass.expired", ev.subject)
	}
	var payload map[string]string
	if err := json.Unmarshal(ev.data, &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if payload["business_id"] != "biz-past" {
		t.Fatalf("payload business_id = %q", payload["business_id"])
	}
	if payload["expired_at"] == "" {
		t.Fatal("payload expired_at missing")
	}
}

func TestSweep_nilAndFutureNeverEmit(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)

	pub := &recPublisher{}
	s := NewSweep(fakeLister{rows: []database.PassExpiry{
		{BusinessID: "biz-future", ExpiresAt: &future},
		{BusinessID: "biz-none", ExpiresAt: nil},
	}}, pub, zap.NewNop().Sugar(), time.Minute)
	s.now = func() time.Time { return now }

	s.pass()
	s.pass()
	if pub.count() != 0 {
		t.Fatalf("emissions = %d, want 0 for nil/future expiries", pub.count())
	}
}

// TestSweep_StartAutonomousPush is the PUSH proof: the worker fires on its own
// with ZERO inbound requests. No HTTP server, no curl — just Start() and a
// recording publisher.
func TestSweep_StartAutonomousPush(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Minute)

	pub := &recPublisher{}
	s := NewSweep(fakeLister{rows: []database.PassExpiry{
		{BusinessID: "biz-push", ExpiresAt: &past},
	}}, pub, zap.NewNop().Sugar(), 20*time.Millisecond)
	s.now = func() time.Time { return now }

	s.Start()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if pub.count() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pub.count() < 1 {
		t.Fatal("worker did not emit autonomously (no inbound request present)")
	}
}
