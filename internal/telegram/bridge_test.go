package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- ParsePurchaseReply ---------------------------------------------------

func TestParsePurchaseReply(t *testing.T) {
	cases := []struct {
		in          string
		wantID      string
		wantApprove bool
		wantOK      bool
	}{
		{"approve a1b2c3", "a1b2c3", true, true},
		{"reject a1b2c3", "a1b2c3", false, true},
		{"/approve xyz", "xyz", true, true},
		{"yes 123", "123", true, true},
		{"no 456", "456", false, true},
		{"", "", false, false},
		{"approve", "", false, false},              // missing id
		{"approve a b", "", false, false},          // too many tokens
		{"banana a1b2c3", "", false, false},        // unknown verdict
		{"approve approve", "approve", true, true}, // id is the second token, no validation here
	}
	for _, tc := range cases {
		id, app, ok := ParsePurchaseReply(tc.in)
		if id != tc.wantID || app != tc.wantApprove || ok != tc.wantOK {
			t.Fatalf("ParsePurchaseReply(%q) = (%q,%v,%v), want (%q,%v,%v)", tc.in, id, app, ok, tc.wantID, tc.wantApprove, tc.wantOK)
		}
	}
}

// --- Bot transport against a fake API server ------------------------------

// fakeAPI is a scripted Telegram Bot-API stand-in.
type fakeAPI struct {
	srv       *httptest.Server
	updates   [][]byte // each batch served per getUpdates call
	sendLog   []string // captured chat_id|text
	errStatus int      // if set, return this status for all calls
}

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.errStatus != 0 {
			http.Error(w, "boom", f.errStatus)
			return
		}
		_ = r.ParseForm()
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			f.sendLog = append(f.sendLog, r.PostFormValue("chat_id")+"|"+r.PostFormValue("text"))
			fmt.Fprint(w, `{"ok":true,"result":{}}`)
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			resp := map[string]any{"ok": true, "result": []any{}}
			if len(f.updates) > 0 {
				resp["result"] = json.RawMessage(f.updates[0])
				f.updates = f.updates[1:]
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	return f
}

func update(id int64, chatID int64, text string) []byte {
	b, _ := json.Marshal(map[string]any{
		"update_id": id,
		"message": map[string]any{
			"message_id": id * 10,
			"chat":       map[string]any{"id": chatID},
			"from":       map[string]any{"id": chatID * 7},
			"text":       text,
		},
	})
	return b
}

// batchOf marshals multiple update objects into one getUpdates `result` array,
// exactly as the real API delivers them in a single poll.
func batchOf(updates ...[]byte) []byte {
	// json.RawMessage keeps the object bytes verbatim inside the array.
	raw := make([]json.RawMessage, len(updates))
	for i, u := range updates {
		raw[i] = json.RawMessage(u)
	}
	b, _ := json.Marshal(raw)
	return b
}

func TestBot_SendMessage(t *testing.T) {
	fake := newFakeAPI()
	defer fake.srv.Close()
	b := NewBot("TOK", WithBase(fake.srv.URL))
	if err := b.SendMessage("12345", "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(fake.sendLog) != 1 || fake.sendLog[0] != "12345|hello" {
		t.Fatalf("send log = %v, want 12345|hello", fake.sendLog)
	}
}

func TestBot_SendMessage_errorSurface(t *testing.T) {
	fake := newFakeAPI()
	defer fake.srv.Close()
	fake.errStatus = 500
	b := NewBot("TOK", WithBase(fake.srv.URL))
	if err := b.SendMessage("1", "x"); err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestBot_Next_filtersChat(t *testing.T) {
	fake := newFakeAPI()
	defer fake.srv.Close()
	// two updates: one from wrong chat, one from operator chat
	fake.updates = [][]byte{
		batchOf(update(1, 999, "approve a1b2c3"), update(2, 123, "approve xyz9")),
	}
	b := NewBot("TOK", WithBase(fake.srv.URL), WithAllowChat(123), WithPollTimeout(time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, err := b.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if m.ChatID != 123 || m.Text != "approve xyz9" {
		t.Fatalf("Next = %+v, want operator-chat approve xyz9", m)
	}
}

func TestBot_AwaitReply_correlates(t *testing.T) {
	fake := newFakeAPI()
	defer fake.srv.Close()
	fake.updates = [][]byte{batchOf(update(1, 123, "approve req-42"))}
	b := NewBot("TOK", WithBase(fake.srv.URL), WithAllowChat(123), WithPollTimeout(time.Millisecond))
	text, rid, ok := b.AwaitReply("req-42", 2*time.Second)
	if !ok || text != "approve req-42" || rid != "req-42" {
		t.Fatalf("AwaitReply = (%q,%q,%v), want correlated ok", text, rid, ok)
	}
}

// --- Bridge worker ---------------------------------------------------------

type fakeSource struct {
	msgs []InboundMessage
	i    int
}

func (f *fakeSource) Next(ctx context.Context) (InboundMessage, error) {
	if f.i >= len(f.msgs) {
		<-ctx.Done()
		return InboundMessage{}, ctx.Err()
	}
	m := f.msgs[f.i]
	f.i++
	return m, nil
}

type fakeDecider struct {
	resolved    map[string]string
	decisions   []string
	errResolve  error
	errDecide   error
	notFoundIDs map[string]bool // ids that return not-found
}

func (f *fakeDecider) ResolveRequestBusiness(requestID string) (string, error) {
	if f.errResolve != nil {
		return "", f.errResolve
	}
	if f.notFoundIDs[requestID] {
		return "", fmt.Errorf("request not found: %s", requestID)
	}
	return f.resolved[requestID], nil
}
func (f *fakeDecider) DecidePurchaseRequest(businessID, requestID string, approve bool) error {
	if f.errDecide != nil {
		return f.errDecide
	}
	f.decisions = append(f.decisions, fmt.Sprintf("%s|%s|%v", businessID, requestID, approve))
	return nil
}

type fakeMsg struct{ sent []string }

func (f *fakeMsg) SendMessage(target, text string) error                   { f.sent = append(f.sent, text); return nil }
func (f *fakeMsg) AwaitReply(string, time.Duration) (string, string, bool) { return "", "", false }

func TestPurchaseBridge_decidesApprove(t *testing.T) {
	src := &fakeSource{msgs: []InboundMessage{{ChatID: 1, Text: "approve pur-1"}}}
	dec := &fakeDecider{resolved: map[string]string{"pur-1": "bizA"}}
	msgr := &fakeMsg{}
	b := NewPurchaseBridge(src, msgr, "999", dec)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	_ = b.Run(ctx)
	if len(dec.decisions) != 1 || dec.decisions[0] != "bizA|pur-1|true" {
		t.Fatalf("decisions = %v, want one approve", dec.decisions)
	}
	// ack sent
	if len(msgr.sent) != 1 || !strings.Contains(msgr.sent[0], "pur-1 approved") {
		t.Fatalf("acks = %v, want success ack", msgr.sent)
	}
}

func TestPurchaseBridge_unparseableIgnored(t *testing.T) {
	src := &fakeSource{msgs: []InboundMessage{{Text: "what do you want"}, {Text: "approve pur-2"}}}
	dec := &fakeDecider{resolved: map[string]string{"pur-2": "bizA"}}
	msgr := &fakeMsg{}
	b := NewPurchaseBridge(src, msgr, "999", dec)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	_ = b.Run(ctx)
	if len(dec.decisions) != 1 {
		t.Fatalf("should only act on parseable reply; got %v", dec.decisions)
	}
}

func TestPurchaseBridge_unknownRequestDenied(t *testing.T) {
	src := &fakeSource{msgs: []InboundMessage{{Text: "reject ghost-1"}}}
	dec := &fakeDecider{notFoundIDs: map[string]bool{"ghost-1": true}}
	msgr := &fakeMsg{}
	b := NewPurchaseBridge(src, msgr, "999", dec)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	_ = b.Run(ctx)
	if len(dec.decisions) != 0 {
		t.Fatalf("must not decide unknown request, got %v", dec.decisions)
	}
	if len(msgr.sent) != 1 {
		t.Fatalf("expected a not-found ack, got %v", msgr.sent)
	}
}
