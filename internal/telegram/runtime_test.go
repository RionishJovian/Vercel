package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memoryBindings struct {
	mu       sync.Mutex
	bindings []Binding
}

func (s *memoryBindings) List(context.Context) ([]Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Binding(nil), s.bindings...), nil
}
func (s *memoryBindings) Get(_ context.Context, id string) (Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.bindings {
		if b.BusinessID == id {
			return b, nil
		}
	}
	return Binding{}, errors.New("not found")
}
func (s *memoryBindings) set(bindings ...Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings = bindings
}

type recordingDecider struct {
	mu        sync.Mutex
	business  string
	decisions int
}

func (d *recordingDecider) ResolveRequestBusiness(string) (string, error) { return d.business, nil }
func (d *recordingDecider) DecidePurchaseRequest(string, string, bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.decisions++
	return nil
}
func (d *recordingDecider) count() int { d.mu.Lock(); defer d.mu.Unlock(); return d.decisions }
func waitRuntime(t *testing.T, check func() bool) {
	t.Helper()
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("runtime condition not reached")
}
func runtimeFixture(t *testing.T, source *memoryBindings, updates []byte) (*Runtime, *recordingDecider, *recordingDecider, *[]string, *sync.Mutex) {
	t.Helper()
	platform := &recordingDecider{business: "customer"}
	business := &recordingDecider{business: "customer"}
	var mu sync.Mutex
	var targets []string
	batch := updates
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = req.ParseForm()
		switch req.URL.Path {
		case "/botfake/getMe":
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/botfake/getWebhookInfo":
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"url": ""}})
		case "/botfake/getUpdates":
			mu.Lock()
			out := batch
			batch = nil
			mu.Unlock()
			if out != nil {
				w.Write([]byte(`{"ok":true,"result":`))
				w.Write(out)
				w.Write([]byte(`}`))
				return
			}
			select {
			case <-req.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
			w.Write([]byte(`{"ok":true,"result":[]}`))
		default:
			mu.Lock()
			targets = append(targets, req.Form.Get("chat_id"))
			mu.Unlock()
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	r := NewRuntime(source, func(Binding) PurchaseDecider { return platform }, business)
	r.factory = func(token string, opts ...BotOption) *Bot { return NewBot(token, append(opts, WithBase(api.URL))...) }
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); r.stop(); api.Close() })
	r.Reconcile(ctx)
	return r, platform, business, &targets, &mu
}
func TestRuntimePlatformRoutingAndAuthorization(t *testing.T) {
	binding := Binding{BusinessID: "admin", Token: "fake", ActorID: "admin-user", ChatID: -100123, ApproverID: 42, Enabled: true, Platform: true}
	s := &memoryBindings{bindings: []Binding{binding}}
	updates := batchOf(callbackUpdate(1, -100123, "approve first"), callbackUpdate(2, -100123, "approve second"))
	// Use the configured sender for the second update; the first is an outsider.
	var raw []map[string]any
	json.Unmarshal(updates, &raw)
	raw[1]["callback_query"].(map[string]any)["from"] = map[string]any{"id": 42}
	updates, _ = json.Marshal(raw)
	r, platform, business, targets, mu := runtimeFixture(t, s, updates)
	waitRuntime(t, func() bool { return platform.count() == 1 && r.Status("admin").Receiver == "running" })
	if business.count() != 0 {
		t.Fatal("platform callback reached customer decider")
	}
	if err := r.SendButtons("foreign-chat", "review", "third"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	for _, target := range *targets {
		if target != "" && target != "-100123" {
			t.Fatalf("foreign target %s", target)
		}
	}
	mu.Unlock()
	if r.Status("admin").LastDelivery != "delivered" {
		t.Fatal("delivery status not observed")
	}
	binding.Enabled = false
	s.set(binding)
	if err := r.SendMessage("ignored", "should not send"); err == nil {
		t.Fatal("disabled binding could send before reconciliation")
	}
}
func TestRuntimeCustomerCannotApproveOtherTenant(t *testing.T) {
	b := Binding{BusinessID: "tenant-A", Token: "fake", ChatID: 123, ApproverID: 123, Enabled: true}
	// callbackUpdate uses from=chat*7, so this is the configured sender and chat.
	raw := callbackUpdate(1, 123, "approve foreign")
	var u map[string]any
	json.Unmarshal(raw, &u)
	u["callback_query"].(map[string]any)["from"] = map[string]any{"id": 123}
	raw, _ = json.Marshal(u)
	r, platform, business, _, _ := runtimeFixture(t, &memoryBindings{bindings: []Binding{b}}, batchOf(raw))
	waitRuntime(t, func() bool { return r.Status(b.BusinessID).Receiver == "running" })
	if platform.count() != 0 || business.count() != 0 {
		t.Fatal("foreign tenant approval accepted")
	}
}
func TestRuntimeDuplicateTokensFailClosed(t *testing.T) {
	b := Binding{BusinessID: "tenant-A", Token: "fake", ChatID: 123, ApproverID: 123, Enabled: true}
	s := &memoryBindings{bindings: []Binding{b}}
	r, _, _, _, _ := runtimeFixture(t, s, nil)
	waitRuntime(t, func() bool { return r.Status(b.BusinessID).Receiver == "running" })
	other := b
	other.BusinessID = "tenant-B"
	s.set(b, other)
	r.Reconcile(context.Background())
	if r.Status(b.BusinessID).Receiver != "error" || r.Status(other.BusinessID).Receiver != "error" {
		t.Fatal("duplicate tokens must stop both consumers")
	}
}
func TestRuntimePlatformRequiresActor(t *testing.T) {
	b := Binding{BusinessID: "admin", Token: "fake", ChatID: 123, ApproverID: 123, Enabled: true, Platform: true}
	r, _, _, _, _ := runtimeFixture(t, &memoryBindings{bindings: []Binding{b}}, nil)
	if r.Status(b.BusinessID).Receiver != "error" {
		t.Fatal("platform actor required")
	}
}

func TestRuntimeRejectsPrivateChatApproverMismatch(t *testing.T) {
	b := Binding{BusinessID: "customer", Token: "fake", ChatID: 999, ApproverID: 123, Enabled: true}
	r, _, _, _, _ := runtimeFixture(t, &memoryBindings{bindings: []Binding{b}}, nil)
	status := r.Status(b.BusinessID)
	if status.Receiver != "error" || status.ApproverConfigured {
		t.Fatal("private-chat sender mismatch was accepted")
	}
}
func TestBotProbeRefusesExistingWebhook(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bottoken/getMe" {
			w.Write([]byte(`{"ok":true}`))
			return
		}
		w.Write([]byte(`{"ok":true,"result":{"url":"https://other-integration.invalid/webhook"}}`))
	}))
	defer api.Close()
	b := NewBot("token", WithBase(api.URL))
	if err := b.Probe(context.Background()); err == nil {
		t.Fatal("poller would compete with an existing webhook")
	}
}

func TestBindingAuthorizationRechecksEveryBoundary(t *testing.T) {
	b := Binding{BusinessID: "tenant-A", Token: "token", ChatID: -100123, ApproverID: 42, Enabled: true}
	m := InboundMessage{ChatID: b.ChatID, FromID: b.ApproverID}
	if !bindingAuthorized(b, b, m, "tenant-A") {
		t.Fatal("configured approver refused")
	}
	for _, tc := range []struct {
		name   string
		change func(*Binding, *InboundMessage)
		tenant string
	}{
		{"other user", func(_ *Binding, m *InboundMessage) { m.FromID++ }, "tenant-A"},
		{"missing sender", func(_ *Binding, m *InboundMessage) { m.FromID = 0 }, "tenant-A"},
		{"other chat", func(_ *Binding, m *InboundMessage) { m.ChatID++ }, "tenant-A"},
		{"other tenant", func(*Binding, *InboundMessage) {}, "tenant-B"},
		{"disabled", func(b *Binding, _ *InboundMessage) { b.Enabled = false }, "tenant-A"},
		{"token replaced", func(b *Binding, _ *InboundMessage) { b.Token = "new" }, "tenant-A"},
		{"chat changed", func(b *Binding, _ *InboundMessage) { b.ChatID++ }, "tenant-A"},
		{"approver changed", func(b *Binding, _ *InboundMessage) { b.ApproverID++ }, "tenant-A"},
		{"scope changed", func(b *Binding, _ *InboundMessage) { b.Platform = true }, "tenant-A"},
		{"actor changed", func(b *Binding, _ *InboundMessage) { b.ActorID = "new" }, "tenant-A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, msg := b, m
			tc.change(&current, &msg)
			if bindingAuthorized(b, current, msg, tc.tenant) {
				t.Fatal("unauthorized approval allowed")
			}
		})
	}
	b.Platform = true
	b.ActorID = "platform-user"
	if !bindingAuthorized(b, b, m, "tenant-B") {
		t.Fatal("platform operator could not review customer order")
	}
}

func TestRuntimeChangedBindingDoesNotReportOldReceiver(t *testing.T) {
	b := Binding{BusinessID: "customer", Token: "fake", ChatID: 123, ApproverID: 123, Enabled: true}
	source := &memoryBindings{bindings: []Binding{b}}
	r, _, _, _, _ := runtimeFixture(t, source, nil)
	waitRuntime(t, func() bool { return r.Status(b.BusinessID).Receiver == "running" })
	b.Token = "replacement"
	source.set(b)
	status := r.Status(b.BusinessID)
	if status.Receiver != "starting" || status.Token != "unknown" {
		t.Fatal("new token inherited old receiver evidence")
	}
	if err := r.SendTest(context.Background(), b.BusinessID); err == nil {
		t.Fatal("new binding sent through old bot")
	}
}
func TestRuntimeReservedRouterTokenDoesNotStartAccountReceiver(t *testing.T) {
	b := Binding{BusinessID: "customer", Token: "fake", ChatID: 123, ApproverID: 123, Enabled: true, Reserved: true}
	r, _, _, _, _ := runtimeFixture(t, &memoryBindings{bindings: []Binding{b}}, nil)
	if r.Status(b.BusinessID).Receiver != "error" {
		t.Fatal("reserved router token acquired a second consumer")
	}
}
func TestRuntimeMultiplePlatformBindingsFailClosed(t *testing.T) {
	a := Binding{BusinessID: "admin-A", Token: "fake", ChatID: 123, ApproverID: 123, Enabled: true, Platform: true, ActorID: "actor-A"}
	b := a
	b.BusinessID = "admin-B"
	b.Token = "other"
	b.ActorID = "actor-B"
	r, _, _, _, _ := runtimeFixture(t, &memoryBindings{bindings: []Binding{a, b}}, nil)
	if r.Status(a.BusinessID).Receiver != "error" || r.Status(b.BusinessID).Receiver != "error" {
		t.Fatal("ambiguous platform binding accepted")
	}
}

func TestDeliveryObserverReceivesFailure(t *testing.T) {
	api := newFakeAPI()
	defer api.srv.Close()
	api.errStatus = 500
	var observed error
	bot := NewBot("fake-token", WithBase(api.srv.URL), WithDeliveryObserver(func(err error) { observed = err }))
	if err := bot.SendMessage("123", "fixture"); err == nil || observed == nil {
		t.Fatal("delivery failure was not reported")
	}
}
