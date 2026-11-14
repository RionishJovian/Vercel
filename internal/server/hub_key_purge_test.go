package server

import (
	"strings"
	"testing"
	"time"
)

func managementWithKey(t *testing.T) (pendingManagement, string) {
	t.Helper()
	p, err := newPendingManagement("tenant", "gateway", "router-key", "{\n:put 1\n/interface/wireguard add name=x private-key=\"SECRETSECRETSECRETSECRETSECRETSECRETSECRET1=\" comment=y\n}", "10.77.0.9/32")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodePendingManagement(p)
	if err != nil {
		t.Fatal(err)
	}
	return p, raw
}

func TestHandshakeRedactionRemovesOnlyTheWireGuardKey(t *testing.T) {
	before, raw := managementWithKey(t)
	if !strings.Contains(raw, "SECRETSECRET") {
		t.Fatal("fixture must contain the key")
	}
	redacted, err := redactManagementTunnelKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(redacted, "SECRETSECRET") {
		t.Fatal("the WireGuard private key survived redaction")
	}
	after, err := decodePendingManagement(redacted)
	if err != nil {
		t.Fatalf("the envelope must stay usable to finish the connection: %v", err)
	}
	if after.private != before.private || after.proof != before.proof || after.user != before.user || after.routerKey != before.routerKey || after.sourceCIDR != before.sourceCIDR || !after.expires.Equal(before.expires) {
		t.Fatal("redaction must leave the management credentials and expiry untouched")
	}
	if !strings.Contains(after.tunnelScript, redactedRouterKey) {
		t.Fatal("redaction must leave a visible marker")
	}
	again, err := redactManagementTunnelKey(redacted)
	if err != nil || again != redacted {
		t.Fatal("redaction must be idempotent")
	}
	if _, err := redactManagementTunnelKey("not json"); err == nil {
		t.Fatal("a corrupt envelope must be reported, not silently kept")
	}
}

func TestNoScriptIsReissuedAfterTheKeyWasRemoved(t *testing.T) {
	p, raw := managementWithKey(t)
	// A real tunnel block, so the only reason to refuse is the removed key.
	p.tunnelScript = "{\n# Scoped tunnel only. Does not enable SSH/REST or change WAN/LAN/firewall/users.\n/interface/wireguard add private-key=\"" + strings.Repeat("A", 43) + "=\"\n}"
	if _, err := automaticEnrollmentScript("gateway", p); err != nil {
		t.Fatalf("a normal script must still build: %v", err)
	}
	redactedRaw, _ := redactManagementTunnelKey(raw)
	redacted, _ := decodePendingManagement(redactedRaw)
	redacted.tunnelScript = privateKeyAssignment.ReplaceAllString(p.tunnelScript, redactedRouterKey)
	if _, err := automaticEnrollmentScript("gateway", redacted); err == nil {
		t.Fatal("a script without its key must never be handed to a router")
	}
	_ = time.Now
}
