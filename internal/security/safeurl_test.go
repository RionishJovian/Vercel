package security

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.5", "172.16.1.1", "172.19.0.2", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "224.0.0.1", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
	}
	for _, s := range blocked {
		if !IsBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"}
	for _, s := range allowed {
		if IsBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
	if !IsBlockedIP(netip.Addr{}) {
		t.Error("an invalid address must be blocked")
	}
}

func TestValidateWebhookURL(t *testing.T) {
	good := []string{"https://example.com/hook", "https://hooks.example.org:8443/a?b=c", "https://8.8.8.8/x"}
	for _, u := range good {
		if err := ValidateWebhookURL(u); err != nil {
			t.Errorf("%s should be accepted: %v", u, err)
		}
	}
	bad := []string{
		"", "not a url", "http://example.com/hook", "ftp://example.com", "https://",
		"https://user:pass@example.com/", "https://127.0.0.1/", "https://[::1]/", "https://169.254.169.254/latest",
		"https://10.0.0.8:5432/", "https://localhost/", "https://db/", "https://redis:6379/", "https://api.internal/",
		"https://printer.local/", "https://172.19.0.3:8080/",
	}
	for _, u := range bad {
		if err := ValidateWebhookURL(u); err == nil {
			t.Errorf("%q should be rejected", u)
		}
	}
}

func TestSafeHTTPClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := SafeHTTPClient(3 * time.Second)
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a request to a loopback server must be refused")
	}
	if !errors.Is(err, ErrBlockedAddress) && !strings.Contains(err.Error(), ErrBlockedAddress.Error()) {
		t.Fatalf("expected a blocked-address error, got: %v", err)
	}
}

func TestSafeHTTPClientDoesNotFollowRedirects(t *testing.T) {
	c := SafeHTTPClient(time.Second)
	if c.CheckRedirect == nil {
		t.Fatal("redirects must be handled explicitly")
	}
	if err := c.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirects must not be followed, got: %v", err)
	}
}
