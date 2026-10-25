package server

import (
	"strings"
	"testing"
)

func TestContainerManagementSourceMustBeExactPrivateHost(t *testing.T) {
	for _, s := range []string{"10.77.0.1/32", "192.168.30.1/32"} {
		got, e := configuredManagementSource("interface-does-not-exist", s)
		if e != nil || got != s {
			t.Fatal(got, e)
		}
	}
	for _, s := range []string{"10.77.0.1/24", "0.0.0.0/0", "127.0.0.1/32", "8.8.8.8/32", "::1/128", "not-an-address"} {
		if _, e := configuredManagementSource("interface-does-not-exist", s); e == nil {
			t.Fatal("unsafe source accepted", s)
		}
	}
	if _, e := configuredManagementSource("interface-does-not-exist", ""); e == nil {
		t.Fatal("unconfigured missing tunnel accepted")
	}
	p, e := newPendingManagement("business", "device", "router-key", "tunnel-script", "10.77.0.1/32")
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := encodePendingManagement(p)
	if e != nil {
		t.Fatal(e)
	}
	reloaded, e := decodePendingManagement(encoded)
	if e != nil || reloaded.private != p.private || reloaded.proof != p.proof || !strings.Contains(reloaded.tunnelScript, "tunnel-script") {
		t.Fatal("pending installation restart roundtrip failed", e)
	}
}
