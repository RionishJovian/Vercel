package ai

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/model"
)

// fakeMsg is a fake telegram.Messenger (no network).
type fakeMsg struct {
	targets []string
	sent    []string
	err     error
}

func (f *fakeMsg) SendMessage(target, text string) error {
	if f.err != nil {
		return f.err
	}
	f.targets = append(f.targets, target)
	f.sent = append(f.sent, text)
	return nil
}
func (f *fakeMsg) VerifyToken() (bool, string)                             { return true, "" }
func (f *fakeMsg) AwaitReply(string, time.Duration) (string, string, bool) { return "", "", false }

func sampleIncident() *model.Incident {
	detail := "latency 540ms"
	return &model.Incident{
		ID:           "inc-123",
		Kind:         "ai_latency_spike",
		Severity:     "critical",
		SubjectID:    "dev-9",
		SubjectLabel: "Router dev-9",
		Detail:       &detail,
		OpenedAt:     time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
	}
}

func TestNotifier_IncidentOpened_sendsMessage(t *testing.T) {
	f := &fakeMsg{}
	n := NewTelegramNotifier(f, "42", zap.NewNop().Sugar())
	n.IncidentOpened("biz-1", sampleIncident())

	if len(f.sent) != 1 {
		t.Fatalf("sends = %d, want 1", len(f.sent))
	}
	if f.targets[0] != "42" {
		t.Fatalf("target = %q, want 42", f.targets[0])
	}
	text := f.sent[0]
	for _, want := range []string{"critical", "Router dev-9", "inc-123"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text %q missing %q", text, want)
		}
	}
	if strings.ContainsAny(text, "\n") {
		t.Fatalf("expected a single line, got %q", text)
	}
}

func TestNotifier_missingTargetIsNoop(t *testing.T) {
	// nil messenger
	n := NewTelegramNotifier(nil, "42", zap.NewNop().Sugar())
	n.IncidentOpened("biz-1", sampleIncident()) // must not panic

	// empty target
	f := &fakeMsg{}
	n2 := NewTelegramNotifier(f, "", zap.NewNop().Sugar())
	n2.IncidentOpened("biz-1", sampleIncident()) // must not panic / send

	if len(f.sent) != 0 {
		t.Fatalf("empty target must not send, got %d", len(f.sent))
	}
	// nil receiver + nil incident are safe too.
	var nilN *TelegramNotifier
	nilN.IncidentOpened("biz-1", sampleIncident())
	n.IncidentOpened("biz-1", nil)
}

func TestNotifier_sendFailureLoggedNotFatal(t *testing.T) {
	f := &fakeMsg{err: errors.New("telegram 429")}
	n := NewTelegramNotifier(f, "42", zap.NewNop().Sugar())

	// Must not panic and must not re-raise.
	n.IncidentOpened("biz-1", sampleIncident())
}

func TestFanout_callsBothSinks(t *testing.T) {
	f := &fakeMsg{}
	tg := NewTelegramNotifier(f, "42", zap.NewNop().Sugar())
	webhook := &fakeNotifier{}

	fan := NewFanout(webhook, tg)
	fan.IncidentOpened("biz-1", sampleIncident())

	if webhook.calls != 1 {
		t.Fatalf("webhook calls = %d, want 1", webhook.calls)
	}
	if len(f.sent) != 1 {
		t.Fatalf("telegram sends = %d, want 1", len(f.sent))
	}
}

func TestFanout_dropsNilSinks(t *testing.T) {
	webhook := &fakeNotifier{}
	fan := NewFanout(nil, webhook, nil)
	fan.IncidentOpened("biz-1", sampleIncident())
	if webhook.calls != 1 {
		t.Fatalf("webhook calls = %d, want 1 (nils dropped)", webhook.calls)
	}
	// nil fanout receiver is safe.
	var nilFan *Fanout
	nilFan.IncidentOpened("biz-1", sampleIncident())
}

func TestFormatIncidentLine_defaults(t *testing.T) {
	inc := &model.Incident{ID: "x", Kind: "ai_latency_spike", Severity: "", SubjectID: "dev-1", OpenedAt: time.Now().UTC()}
	line := formatIncidentLine(inc)
	if !strings.HasPrefix(line, "AI alert [info] ") {
		t.Fatalf("empty severity must default to info: %q", line)
	}
	if !strings.Contains(line, "dev-1") {
		t.Fatalf("subject id fallback missing: %q", line)
	}
}
