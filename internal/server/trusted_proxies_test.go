package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func clientIPFor(t *testing.T, trusted []string, peer, xff string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies(trusted); err != nil {
		t.Fatal(err)
	}
	var got string
	r.GET("/", func(c *gin.Context) { got = c.ClientIP() })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer + ":4000"
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	r.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestTrustedProxyClientIP(t *testing.T) {
	trusted := trustedProxyCIDRs(nil, []string{"172.18.0.0/16"})
	if got := clientIPFor(t, trusted, "172.18.0.4", "203.0.113.10"); got != "203.0.113.10" {
		t.Fatalf("trusted peer: want forwarded client, got %s", got)
	}
	if got := clientIPFor(t, trusted, "198.51.100.9", "10.0.0.1"); got != "198.51.100.9" {
		t.Fatalf("direct caller spoof must be ignored, got %s", got)
	}
	// An explicit list replaces the defaults: a sibling container outside it is not trusted.
	explicit := trustedProxyCIDRs([]string{"172.18.0.4/32"}, []string{"172.18.0.0/16"})
	if got := clientIPFor(t, explicit, "172.18.0.9", "10.0.0.1"); got != "172.18.0.9" {
		t.Fatalf("out-of-list peer must be ignored, got %s", got)
	}
	// A guest-written header in front of the real one never wins.
	if got := clientIPFor(t, trusted, "172.18.0.4", "10.0.0.1, 203.0.113.10"); got != "203.0.113.10" {
		t.Fatalf("rightmost untrusted must win, got %s", got)
	}
}
