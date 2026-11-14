package hub

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const goodToken = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq" // 43 base64url characters

func TestRouterOSVersionGate(t *testing.T) {
	for version, want := range map[string]bool{
		"7.16.2 (stable)":                    true,
		"7.17rc1 (testing)":                  true,
		"7.0":                                true,
		"7.19.4":                             true,
		"6.49.13 (long-term)":                false,
		"6.48":                               false,
		"":                                   false,
		"seven":                              false,
		"7":                                  false, // no minor: unreadable, fail closed
		"7.1\"; /system reset-configuration": false, // script syntax is never accepted
		"7.1" + strings.Repeat("a", 60):      false,
	} {
		err := RequireWireGuard(version)
		if (err == nil) != want {
			t.Fatalf("RequireWireGuard(%q) = %v, want ok=%v", version, err, want)
		}
		if err != nil && !errors.Is(err, ErrWireGuardUnsupported) {
			t.Fatalf("unexpected error type for %q: %v", version, err)
		}
	}
	if major, minor, ok := ParseRouterOSVersion("7.16.2 (stable)"); !ok || major != 7 || minor != 16 {
		t.Fatal("version not parsed")
	}
}

func TestCallbackBootstrapKeepsKeyOnRouterAndVerifiesTLS(t *testing.T) {
	script, err := BuildCallbackBootstrapScript("gateway", "https://megamagic.tech", goodToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-key", "check-certificate=no", "check-certificate=yes-without-crl", "/user", "/ip/service", "/ip/firewall", "0.0.0.0/0", "remove"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("callback script contains %q", forbidden)
		}
	}
	for _, required := range []string{
		"check-certificate=yes",
		"https://megamagic.tech" + CallbackEnrollPath,
		"public-key]",
		`mega-magic:mmh-`,
		"output=user as-value",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("callback script lacks %q", required)
		}
	}
	// The version refusal must come before the first write to the router.
	gate := strings.Index(script, "$mmMajor < 7")
	firstWrite := strings.Index(script, "/interface/wireguard add")
	if gate < 0 || firstWrite < 0 || gate > firstWrite {
		t.Fatal("RouterOS 6 must be refused before anything is created")
	}
	if strings.Count(script, goodToken) != 1 {
		t.Fatal("token must appear exactly once, in the request body")
	}
	again, _ := BuildCallbackBootstrapScript("gateway", "https://megamagic.tech", goodToken)
	if again != script {
		t.Fatal("script is not deterministic")
	}
	other, _ := BuildCallbackBootstrapScript("other-gateway", "https://megamagic.tech", goodToken)
	if other == script {
		t.Fatal("gateway interfaces collide")
	}
}

func TestCallbackBootstrapRejectsUnsafeInputs(t *testing.T) {
	for name, args := range map[string][3]string{
		"http base":        {"gateway", "http://megamagic.tech", goodToken},
		"userinfo":         {"gateway", "https://user:pw@megamagic.tech", goodToken},
		"path":             {"gateway", "https://megamagic.tech/x", goodToken},
		"query":            {"gateway", "https://megamagic.tech?x=1", goodToken},
		"quote in host":    {"gateway", `https://mega"magic.tech`, goodToken},
		"empty base":       {"gateway", "", goodToken},
		"short token":      {"gateway", "https://megamagic.tech", "short"},
		"token injection":  {"gateway", "https://megamagic.tech", strings.Repeat("A", 40) + `";:`},
		"bad gateway name": {"bad name;", "https://megamagic.tech", goodToken},
	} {
		if s, err := BuildCallbackBootstrapScript(args[0], args[1], args[2]); err == nil || s != "" {
			t.Fatalf("%s: unsafe input produced a script", name)
		}
	}
}

func retryClient(t *testing.T, retries int, transport roundTrip, waits *[]time.Duration) *Client {
	t.Helper()
	o := opts()
	o.Retries = retries
	o.Transport = transport
	o.Sleep = func(_ context.Context, d time.Duration) error {
		if waits != nil {
			*waits = append(*waits, d)
		}
		return nil
	}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestIdempotentCallsRetryWithFreshSignatures(t *testing.T) {
	var mu sync.Mutex
	var ids, signatures []string
	var waits []time.Duration
	c := retryClient(t, 0, func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		ids = append(ids, r.Header.Get("x-mm-request-id"))
		signatures = append(signatures, r.Header.Get("x-mm-signature"))
		if len(ids) < 3 {
			return response(503, "busy"), nil
		}
		return response(200, peerJSON), nil
	}, &waits)
	if _, err := c.InspectRouter(context.Background(), identity); err != nil {
		t.Fatalf("lookup should recover: %v", err)
	}
	if len(ids) != 3 || ids[0] == ids[1] || ids[1] == ids[2] || signatures[0] == signatures[1] {
		t.Fatalf("each attempt needs a fresh request id and signature: %v", ids)
	}
	if len(waits) != 2 || waits[1] <= waits[0]/2 || waits[0] <= 0 {
		t.Fatalf("backoff should grow: %v", waits)
	}
}

func TestRetriesAreBounded(t *testing.T) {
	calls := 0
	c := retryClient(t, 0, func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("connection refused")
	}, nil)
	if _, err := c.InspectRouter(context.Background(), identity); err == nil {
		t.Fatal("offline Hub must fail")
	}
	if calls != 1+DefaultRetries {
		t.Fatalf("expected %d attempts, got %d", 1+DefaultRetries, calls)
	}
	calls = 0
	c = retryClient(t, -1, func(*http.Request) (*http.Response, error) {
		calls++
		return response(503, ""), nil
	}, nil)
	if _, err := c.InspectRouter(context.Background(), identity); err == nil || calls != 1 {
		t.Fatalf("negative retries disables retrying, got %d calls", calls)
	}
}

func TestRemovePeerRetriesButCreateNeverDoes(t *testing.T) {
	removeCalls := 0
	c := retryClient(t, 0, func(r *http.Request) (*http.Response, error) {
		removeCalls++
		if removeCalls == 1 {
			return response(502, ""), nil
		}
		return response(200, ""), nil
	}, nil)
	if err := c.RemovePeer(context.Background(), identity, "peer-a"); err != nil || removeCalls != 2 {
		t.Fatalf("DELETE is idempotent and should retry once: err=%v calls=%d", err, removeCalls)
	}

	postCalls := 0
	c = retryClient(t, 0, func(r *http.Request) (*http.Response, error) {
		postCalls++
		if r.Method != http.MethodPost {
			t.Fatal("create must stay POST")
		}
		return response(503, ""), nil
	}, nil)
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	if _, err := c.EnrollPeer(context.Background(), identity, key, 8728); err == nil || postCalls != 1 {
		t.Fatalf("a create that may have reserved a key must not be repeated: err=%v calls=%d", err, postCalls)
	}
}

func TestCancelledContextStopsRetrying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	c := retryClient(t, 0, func(*http.Request) (*http.Response, error) {
		calls++
		cancel()
		return nil, errors.New("timeout")
	}, nil)
	if _, err := c.InspectRouter(ctx, identity); err == nil || calls != 1 {
		t.Fatalf("a cancelled request must not keep retrying: err=%v calls=%d", err, calls)
	}
}
