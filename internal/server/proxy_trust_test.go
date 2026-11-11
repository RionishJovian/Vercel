package server

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestProxyTrustRequiresExactControlledPeers(t *testing.T) {
	for _, tc := range []struct{ name, remote, xff, real, want string }{
		{"two controlled hops", "172.18.0.5:8080", "198.51.100.10, 172.18.0.1", "", "198.51.100.10"},
		{"spoofed left address", "172.18.0.5:8080", "203.0.113.66, 198.51.100.10, 172.18.0.1", "", "198.51.100.10"},
		{"untrusted docker neighbor", "172.18.0.9:8080", "203.0.113.66", "", "172.18.0.9"},
		{"proxy restart new peer fails closed", "172.18.0.6:8080", "203.0.113.66", "", "172.18.0.6"},
		{"no forwarded header", "172.18.0.5:8080", "", "", "172.18.0.5"},
		{"malformed forwarded header", "172.18.0.5:8080", "malformed", "", "172.18.0.5"},
		{"unapproved real IP header", "172.18.0.5:8080", "", "203.0.113.66", "172.18.0.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			if err := configureProxyTrust(r, []string{"127.0.0.1", "172.18.0.5", "172.18.0.1"}); err != nil {
				t.Fatal(err)
			}
			r.GET("/", func(c *gin.Context) { c.String(200, c.ClientIP()) })
			q := httptest.NewRequest("GET", "/", nil)
			q.RemoteAddr = tc.remote
			q.Header.Set("X-Forwarded-For", tc.xff)
			q.Header.Set("X-Real-IP", tc.real)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Body.String() != tc.want {
				t.Fatalf("got %s, want %s", w.Body, tc.want)
			}
		})
	}
	for _, peer := range []string{"0.0.0.0/0", "172.18.0.0/16", "web", "not an IP", "::/0"} {
		if err := configureProxyTrust(gin.New(), []string{peer}); err == nil {
			t.Fatalf("accepted non-exact peer %q", peer)
		}
	}
}

func TestProxyTrustIPv6AndHeaderEdgeCases(t *testing.T) {
	for _, tc := range []struct{ name, remote, xff, want string }{
		{"IPv6 client behind trusted proxy", "172.18.0.5:80", "2001:db8:1:2::9, 172.18.0.1", "2001:db8:1:2::9"},
		{"IPv6 spoof from untrusted peer ignored", "[2001:db8::bad]:80", "2001:db8:1:2::9", "2001:db8::bad"},
		{"IPv4-mapped spoof from untrusted peer ignored", "172.18.0.77:80", "::ffff:203.0.113.9", "172.18.0.77"},
		{"hostname in forwarded header", "172.18.0.5:80", "evil.example, 172.18.0.1", "172.18.0.5"},
		{"empty forwarded entries", "172.18.0.5:80", ", , 172.18.0.1", "172.18.0.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			if err := configureProxyTrust(r, []string{"127.0.0.1", "172.18.0.5", "172.18.0.1"}); err != nil {
				t.Fatal(err)
			}
			r.GET("/", func(c *gin.Context) { c.String(200, c.ClientIP()) })
			q := httptest.NewRequest("GET", "/", nil)
			q.RemoteAddr = tc.remote
			q.Header.Set("X-Forwarded-For", tc.xff)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Body.String() != tc.want {
				t.Fatalf("got %s, want %s", w.Body, tc.want)
			}
		})
	}
}
