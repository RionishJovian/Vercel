package notifier

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mega-magic/internal/model"
)

// The production client refuses loopback addresses, so tests use a plain client against a local server.
func testNotifier() *Notifier {
	sleep = func(time.Duration) {}
	return &Notifier{client: &http.Client{Timeout: 2 * time.Second}}
}

func str(s string) *string { return &s }

func TestDeliverSignsThePayloadAndSetsHeaders(t *testing.T) {
	payload := []byte(`{"event":"incident.opened","id":"i1"}`)
	var gotSig, gotEvent, gotUA, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig, gotEvent, gotUA, gotType = r.Header.Get("X-MegaMagic-Signature"), r.Header.Get("X-MegaMagic-Event"), r.Header.Get("User-Agent"), r.Header.Get("Content-Type")
		w.WriteHeader(204)
	}))
	defer srv.Close()

	ok, code, errText := testNotifier().deliver(&model.WebhookEndpoint{URL: srv.URL, Secret: str("whsec_test")}, "incident.opened", payload)
	if !ok || code != 204 || errText != "" {
		t.Fatalf("deliver = %v %d %q", ok, code, errText)
	}
	mac := hmac.New(sha256.New, []byte("whsec_test"))
	mac.Write(payload)
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); gotSig != want {
		t.Fatalf("signature %q, want %q", gotSig, want)
	}
	if gotEvent != "incident.opened" || gotUA != "MegaMagic-Notifier/1.0" || gotType != "application/json" {
		t.Fatalf("headers: event=%q ua=%q type=%q", gotEvent, gotUA, gotType)
	}
}

func TestDeliverSendsNoSignatureWithoutASecret(t *testing.T) {
	for name, secret := range map[string]*string{"nil": nil, "empty": str("")} {
		t.Run(name, func(t *testing.T) {
			var sawSig bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, sawSig = r.Header["X-Megamagic-Signature"]
			}))
			defer srv.Close()
			if ok, _, _ := testNotifier().deliver(&model.WebhookEndpoint{URL: srv.URL, Secret: secret}, "x", []byte("{}")); !ok || sawSig {
				t.Fatalf("ok=%v sawSignature=%v", ok, sawSig)
			}
		})
	}
}

func TestDeliverRetriesThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	ok, code, _ := testNotifier().deliver(&model.WebhookEndpoint{URL: srv.URL}, "x", []byte("{}"))
	if !ok || code != 200 || atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("ok=%v code=%d calls=%d", ok, code, calls)
	}
}

func TestDeliverGivesUpAfterMaxAttemptsAndReportsTheLastFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	ok, code, errText := testNotifier().deliver(&model.WebhookEndpoint{URL: srv.URL}, "x", []byte("{}"))
	if ok || code != 503 || errText != "http 503" || atomic.LoadInt32(&calls) != MaxAttempts {
		t.Fatalf("ok=%v code=%d err=%q calls=%d", ok, code, errText, calls)
	}
}

func TestDeliverTreatsRedirectsAndUnreachableTargetsAsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(302) }))
	n := testNotifier()
	n.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if ok, code, _ := n.deliver(&model.WebhookEndpoint{URL: srv.URL}, "x", []byte("{}")); ok || code != 302 {
		t.Fatalf("redirect counted as delivered: ok=%v code=%d", ok, code)
	}
	srv.Close()
	if ok, code, errText := testNotifier().deliver(&model.WebhookEndpoint{URL: srv.URL}, "x", []byte("{}")); ok || code != 0 || errText == "" {
		t.Fatalf("unreachable: ok=%v code=%d err=%q", ok, code, errText)
	}
}

func TestDeliverRefusesAnInvalidURL(t *testing.T) {
	if ok, _, errText := testNotifier().deliver(&model.WebhookEndpoint{URL: "http://[::1"}, "x", []byte("{}")); ok || errText == "" {
		t.Fatalf("invalid URL delivered: ok=%v err=%q", ok, errText)
	}
}

type fixtureIncidentSink struct {
	count    int
	business string
}

func (s *fixtureIncidentSink) IncidentOpened(business string, _ *model.Incident) {
	s.count++
	s.business = business
}
func TestIncidentSinkReceivesOpenedEvent(t *testing.T) {
	n := testNotifier()
	sink := &fixtureIncidentSink{}
	n.SetIncidentSink(sink)
	n.IncidentOpened("business-fixture", &model.Incident{ID: "incident-fixture"})
	if sink.count != 1 || sink.business != "business-fixture" {
		t.Fatal("platform incident sink was not notified")
	}
	n.IncidentOpened("business-fixture", nil)
	if sink.count != 1 {
		t.Fatal("nil incident notified")
	}
}
