package hub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func opts() Options {
	return Options{URL: "https://hub.example.test/internal/provisioner", KeyID: "v1", SigningKey: testKey}
}

var identity = Identity{TenantID: "tenant-a", RouterID: "router-a", RequestedBy: "owner-a"}

const peerJSON = `{"peerId":"peer-a","tenantId":"tenant-a","routerId":"router-a","state":"active","lastHandshakeAt":null}`

func TestNodeSignatureVector(t *testing.T) {
	key, _ := hex.DecodeString(testKey)
	canonical := strings.Join([]string{"GET", "/v1/peers/peer-a", "2026-10-01T00:00:00Z", "request-a", "peers:inspect", "tenant-a", "router-a", "owner-a", ""}, "\n")
	// Reference generated with Node createHmac using the live service's contract.
	if got := signature(key, canonical); got != "U9d5fqfYHQr6XthW9YdvE7oT-t_jMzvilrh580eFht0" {
		t.Fatalf("Node/Go signature mismatch: %s", got)
	}
}

func TestInspectRouterIsReadOnlyAndTenantBound(t *testing.T) {
	for _, body := range []string{peerJSON, strings.Replace(peerJSON, "tenant-a", "tenant-b", 1)} {
		o := opts()
		o.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.Path != "/internal/provisioner/v1/peers/by-router/router-a" || r.Header.Get("x-mm-scope") != "peers:inspect" {
				t.Fatal("unsafe lookup")
			}
			return response(200, body), nil
		})
		c, _ := New(o)
		_, err := c.InspectRouter(context.Background(), identity)
		if (err == nil) != (body == peerJSON) {
			t.Fatalf("tenant validation: %v", err)
		}
	}
	o := opts()
	calls := 0
	o.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" {
			t.Fatal("lookup attempted enrollment")
		}
		return response(404, "not_found"), nil
	})
	c, _ := New(o)
	if _, err := c.InspectRouter(context.Background(), identity); err == nil || calls != 1 {
		t.Fatal("missing route must fail without mutation")
	}
}

func TestExistingHubCanonicalContract(t *testing.T) {
	o := opts()
	seen := map[string]bool{}
	o.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		path := strings.TrimPrefix(r.URL.Path, "/internal/provisioner")
		parts := []string{r.Method, path, r.Header.Get("x-mm-timestamp"), r.Header.Get("x-mm-request-id"), r.Header.Get("x-mm-scope"), r.Header.Get("x-mm-tenant-id"), r.Header.Get("x-mm-router-id"), r.Header.Get("x-mm-requested-by"), string(body)}
		key, _ := hex.DecodeString(testKey)
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(strings.Join(parts, "\n")))
		if r.Header.Get("x-mm-signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
			t.Fatal("incompatible signature")
		}
		requestID := r.Header.Get("x-mm-request-id")
		if seen[requestID] || len(requestID) != 32 {
			t.Fatal("request ID repeated or missing")
		}
		seen[requestID] = true
		if r.Method == "GET" && (len(body) != 0 || r.Header.Get("x-mm-scope") != "peers:inspect") {
			t.Fatal("wrong read contract")
		}
		if r.Method == "POST" && (!strings.Contains(string(body), `"tenantId":"tenant-a"`) || r.Header.Get("x-mm-scope") != "peers:create") {
			t.Fatal("wrong enrollment contract")
		}
		if r.Method == "POST" {
			return response(200, strings.TrimSuffix(peerJSON, "}")+`,"enrollmentVersion":1,"routerPublicKey":"`+base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))+`","tunnelAddress":"10.77.0.2/32"}`), nil
		}
		return response(200, peerJSON), nil
	})
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		p, err := c.InspectPeer(context.Background(), identity, "peer-a")
		if err != nil || p.LastHandshakeAt != nil {
			t.Fatal(err)
		}
	}
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	if _, err := c.EnrollPeer(context.Background(), identity, key, 443); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsConfiguration(t *testing.T) {
	for _, url := range []string{"http://hub.test", "https://user:pass@hub.test", "https://hub.test/other", "https://hub.test?key=x", "https://hub.test/#x"} {
		o := opts()
		o.URL = url
		if _, err := New(o); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	o := opts()
	o.SigningKey = "secret"
	if _, err := New(o); err == nil {
		t.Fatal("weak/legacy key accepted")
	}
}

func TestEnrollmentNeverFallsBackToReplacement(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	for _, status := range []int{404, 405, 409, 503} {
		o := opts()
		calls := 0
		o.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" {
				t.Fatal("unsafe enrollment fallback")
			}
			return response(status, "{}"), nil
		})
		c, _ := New(o)
		if _, err := c.EnrollPeer(context.Background(), identity, key, 443); err == nil || calls != 1 {
			t.Fatal("failed enrollment retried unsafely")
		}
	}
}

func TestRejectsResponsesAndRedirect(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{401, "SECRET"}, {302, "redirect"}, {200, strings.Replace(peerJSON, "tenant-a", "tenant-b", 1)}, {200, "not-json"}, {200, strings.Repeat("a", 65537)}} {
		o := opts()
		o.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
			res := response(tc.status, tc.body)
			res.Header.Set("Location", "https://untrusted.test/")
			return res, nil
		})
		c, _ := New(o)
		_, err := c.InspectPeer(context.Background(), identity, "peer-a")
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("unsafe result: %v", err)
		}
	}
}

func TestInvalidIdentityNeverSent(t *testing.T) {
	o := opts()
	o.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request transmitted")
		return nil, fmt.Errorf("unreachable")
	})
	c, _ := New(o)
	for _, id := range []Identity{{}, {"tenant-a\npeers:create", "router-a", "owner-a"}} {
		if _, err := c.InspectPeer(context.Background(), id, "peer-a"); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
	if _, err := c.InspectPeer(context.Background(), identity, "../other"); err == nil {
		t.Fatal("unsafe peer path accepted")
	}
	if _, err := c.EnrollPeer(context.Background(), identity, base64.StdEncoding.EncodeToString(make([]byte, 32)), 443); err == nil {
		t.Fatal("zero public key accepted")
	}
}
