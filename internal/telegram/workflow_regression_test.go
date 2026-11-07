package telegram

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkflowBatchMustDeliverEveryUpdate(t *testing.T) {
	f := newFakeAPI()
	defer f.srv.Close()
	f.updates = [][]byte{batchOf(update(1, 123, "approve first"), update(2, 123, "approve second"))}
	b := NewBot("AUDIT_FAKE_TOKEN", WithBase(f.srv.URL), WithAllowChat(123))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := b.Next(ctx); err != nil {
		t.Fatal(err)
	}
	m, err := b.Next(ctx)
	if err != nil || m.Text != "approve second" {
		t.Fatal("second update was discarded after the first Next call")
	}
}
func TestWorkflowNegativeChatMustStillFilter(t *testing.T) {
	f := newFakeAPI()
	defer f.srv.Close()
	f.updates = [][]byte{batchOf(update(1, 999, "approve outsider"), update(2, -100123, "approve operator"))}
	b := NewBot("AUDIT_FAKE_TOKEN", WithBase(f.srv.URL), WithAllowChat(-100123))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	m, err := b.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.ChatID != -100123 {
		t.Fatal("configured negative group chat did not reject a foreign chat")
	}
}
func TestWorkflowTokenMustNotAppearInProbeErrors(t *testing.T) {
	s := httptest.NewServer(nil)
	base := s.URL
	s.Close()
	b := NewBot("AUDIT_FAKE_TOKEN", WithBase(base))
	ok, detail := b.VerifyToken()
	if ok {
		t.Fatal("unexpected success")
	}
	if strings.Contains(detail, "AUDIT_FAKE_TOKEN") {
		t.Fatal("token is exposed in the verification error detail")
	}
}
func TestWorkflowPlatformBridgeMustRejectUnconfiguredSender(t *testing.T) {
	d := &fakeDecider{resolved: map[string]string{"order": "business"}}
	b := NewPurchaseBridge(nil, nil, "", d)
	b.handle(InboundMessage{ChatID: 999, FromID: 999, Text: "approve order"})
	if len(d.decisions) != 0 {
		t.Fatal("platform bridge accepted an approval without a configured operator restriction")
	}
}
