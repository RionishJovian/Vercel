package server

import (
	"mega-magic/internal/config"
	"testing"
)

func TestConfiguredHTTPListenHost(t *testing.T) {
	for _, tc := range []struct{ host, want string }{{"127.0.0.1", "127.0.0.1:8080"}, {"::1", "[::1]:8080"}, {"0.0.0.0", "0.0.0.0:8080"}, {"", ":8080"}} {
		if got := httpListenAddress(config.ServerConfig{Host: tc.host, Port: 8080}); got != tc.want {
			t.Fatalf("%q: %q, want %q", tc.host, got, tc.want)
		}
	}
}
