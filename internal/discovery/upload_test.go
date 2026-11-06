package discovery

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sample() Observation {
	return Observation{
		Brand: "MikroTik", Method: "MNDP", Status: StatusNearbyUnverified, MAC: "48:A9:8A:00:00:06", MACs: []string{"48:A9:8A:00:00:06"}, DeviceKey: "sw:0123456789abcdef",
		Identity: "hAP", Board: "C52iG-5HaxD2HaxD", Platform: "MikroTik", Version: "7.25", Interface: "bridge/wifi2", ClaimedIP: "192.168.88.1", SourceIP: "192.168.88.1", Count: 3,
		FirstSeen: time.Now(), LastSeen: time.Now(),
	}
}

func TestPayloadSendsOnlyWhatTheServerNeeds(t *testing.T) {
	b, err := Payload([]Observation{sample()})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Observations []map[string]any `json:"observations"`
	}
	if err := json.Unmarshal(b, &got); err != nil || len(got.Observations) != 1 {
		t.Fatalf("payload: %s %v", b, err)
	}
	o := got.Observations[0]
	want := map[string]bool{"device_key": true, "macs": true, "identity": true, "board": true, "platform": true, "version": true, "interface": true, "claimed_ip": true, "source_ip": true, "ip_mismatch": true, "ip_changed": true, "heard_count": true, "status": true}
	for k := range o {
		if !want[k] {
			t.Errorf("unexpected field sent: %s", k)
		}
	}
	if o["status"] != "nearby_unverified" || o["heard_count"].(float64) != 3 || o["device_key"] != "sw:0123456789abcdef" {
		t.Fatalf("unexpected values: %v", o)
	}
	// The raw software id and anything uptime-like must never appear.
	for _, banned := range []string{"CMAN", "software", "uptime", "ipv6", "fe80"} {
		if strings.Contains(strings.ToLower(string(b)), banned) {
			t.Errorf("payload mentions %q", banned)
		}
	}
}

func TestPayloadKeepsOnlyLocalDevicesAndBoundsTheBatch(t *testing.T) {
	pub := sample()
	pub.SourceIP = "8.8.8.8"
	noSource := sample()
	noSource.SourceIP = ""
	pubClaim := sample()
	pubClaim.ClaimedIP = "8.8.8.8"
	fallback := sample()
	fallback.DeviceKey = ""
	b, _ := Payload([]Observation{pub, noSource, pubClaim, fallback})
	var got struct {
		Observations []map[string]any `json:"observations"`
	}
	_ = json.Unmarshal(b, &got)
	if len(got.Observations) != 2 {
		t.Fatalf("only observations from a private IPv4 source may be sent: %s", b)
	}
	if got.Observations[0]["claimed_ip"] != "" || got.Observations[1]["device_key"] != "mac:48:A9:8A:00:00:06" {
		t.Fatalf("a public claimed address is dropped and a missing key falls back to the MAC: %v", got.Observations)
	}
	many := make([]Observation, 100)
	for i := range many {
		many[i] = sample()
	}
	b, _ = Payload(many)
	_ = json.Unmarshal(b, &got)
	if len(got.Observations) != 64 {
		t.Fatalf("a batch must not exceed 64: %d", len(got.Observations))
	}
}

func TestUploaderRefusesPlainHTTPAndOddURLs(t *testing.T) {
	for _, bad := range []string{"http://megamagic.tech", "megamagic.tech", "https://", "https://user:pw@megamagic.tech", "https://megamagic.tech?x=1", "ftp://x", ""} {
		if _, err := NewUploader(bad, nil); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if _, err := NewUploader("https://megamagic.tech/", nil); err != nil {
		t.Fatal(err)
	}
}

func TestUploaderVerifiesCertificatesByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	u, err := NewUploader(srv.URL, nil) // default client: does not trust the test certificate
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Pair(context.Background(), "ABCDEF-GHJKMNPQ"); err == nil {
		t.Fatal("an untrusted certificate must fail the request")
	}
	if hits.Load() != 0 {
		t.Fatal("no request may reach a server whose certificate is not trusted")
	}
}

func TestPairAndUploadFlow(t *testing.T) {
	var seenToken, seenPath string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/discovery/pair":
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["pairing_code"] != "ABCDEF-GHJKMNPQ" {
				w.WriteHeader(401)
				return
			}
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"session_id":"0a1b2c3d-0000-4000-8000-000000000001","helper_token":"` + strings.Repeat("a", 64) + `","expires_at":"2026-11-06T12:00:00Z"}`))
		default:
			seenPath, seenToken = r.URL.Path, r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"accepted":1,"dropped":0}`))
		}
	}))
	defer srv.Close()
	u, _ := NewUploader(srv.URL, srv.Client())
	if _, err := u.Pair(context.Background(), "WRONGC-ODEWRONG"); err == nil {
		t.Fatal("a refused code must be an error")
	}
	s, err := u.Pair(context.Background(), " ABCDEF-GHJKMNPQ ")
	if err != nil || s.SessionID == "" || s.Token == "" {
		t.Fatalf("pair: %+v %v", s, err)
	}
	a, d, err := u.Upload(context.Background(), s, []Observation{sample()})
	if err != nil || a != 1 || d != 0 {
		t.Fatalf("upload: %d %d %v", a, d, err)
	}
	if seenPath != "/api/v1/discovery/sessions/"+s.SessionID+"/observations" || seenToken != "Bearer "+strings.Repeat("a", 64) {
		t.Fatalf("path or token wrong: %s %s", seenPath, seenToken)
	}
}

func TestUploadMapsServerAnswers(t *testing.T) {
	for status, want := range map[int]error{401: ErrSessionEnded, 404: ErrSessionEnded, 403: ErrSessionEnded, 429: ErrSlowDown} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		u, _ := NewUploader(srv.URL, srv.Client())
		_, _, err := u.Upload(context.Background(), PairedSession{SessionID: "s", Token: "t"}, nil)
		srv.Close()
		if err != want {
			t.Errorf("status %d: got %v, want %v", status, err, want)
		}
	}
}

func TestUploaderNeverFollowsRedirectsWithTheToken(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Add(1)
		}
	}))
	defer other.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	u, _ := NewUploader(srv.URL, srv.Client())
	_, _, err := u.Upload(context.Background(), PairedSession{SessionID: "s", Token: strings.Repeat("a", 64)}, nil)
	if err == nil || leaked.Load() != 0 {
		t.Fatalf("a redirect must not be followed (err=%v, leaked=%d)", err, leaked.Load())
	}
}

func TestListenProgressReportsAndStopsWhenTheSessionEnds(t *testing.T) {
	now := t0
	conn := &fakeConn{now: &now, packets: [][]byte{full(), full(), full(), full(), full(), full()}}
	calls := 0
	res, err := ListenProgress(context.Background(), conn, parser, 120*time.Second, func() time.Time { return now }, 10*time.Second, func(r Result) error {
		calls++
		if calls == 2 {
			return ErrSessionEnded
		}
		return nil
	})
	if err != ErrSessionEnded || calls != 2 || res.Packets == 0 {
		t.Fatalf("a failing report must end the run: calls=%d err=%v res=%+v", calls, err, res)
	}
}

func TestExtraCAIsTrustedOnlyWhenGiven(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"session_id":"s","helper_token":"` + strings.Repeat("a", 64) + `","expires_at":"2026-11-06T12:00:00Z"}`))
	}))
	defer srv.Close()
	pemPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(pemPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	// Without the extra authority the certificate is refused.
	plain, _ := NewUploader(srv.URL, nil)
	if _, err := plain.Pair(context.Background(), "ABCDEF-GHJKMNPQ"); err == nil {
		t.Fatal("an unknown certificate authority must be refused by default")
	}
	// With it, that one authority is trusted and normal verification still runs.
	hc, err := ClientWithExtraCA(pemPath)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := NewUploader(srv.URL, hc)
	if _, err := u.Pair(context.Background(), "ABCDEF-GHJKMNPQ"); err != nil {
		t.Fatalf("the extra authority must be trusted: %v", err)
	}
	// A different certificate is still refused: the option adds one authority, it does not switch checking off.
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer other.Close()
	u2, _ := NewUploader(other.URL, hc)
	if _, err := u2.Pair(context.Background(), "ABCDEF-GHJKMNPQ"); err == nil {
		t.Fatal("a certificate from a different authority must still fail")
	}
	if _, err := ClientWithExtraCA(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Fatal("a missing file must be an error")
	}
	bad := filepath.Join(t.TempDir(), "bad.pem")
	_ = os.WriteFile(bad, []byte("not a certificate"), 0o600)
	if _, err := ClientWithExtraCA(bad); err == nil {
		t.Fatal("a file without a certificate must be an error")
	}
}
