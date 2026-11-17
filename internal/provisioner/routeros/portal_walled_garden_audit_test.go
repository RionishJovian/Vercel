package routeros

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// The walled garden is what a guest can reach before paying. A portal install may add at most two rules: TCP 443 to
// the hosted portal by name, and (only when the owner set one) one TCP port on a private media server. Anything
// wider, such as a rule with no port, no protocol, no HotSpot server, a wildcard host or a public address, would
// be free internet for everyone on the HotSpot.
func TestPortalInstallOpensNoInternetBypass(t *testing.T) {
	for _, media := range []string{"", "192.168.88.10"} {
		r := newPortalRunner()
		a := portalTestApplier(r)
		ctx := context.Background()
		request := PortalDeployment{Profile: "guest", MediaHost: media}
		plan, err := a.PreviewPortal(ctx, "biz", "device", request)
		if err != nil {
			t.Fatal(err)
		}
		request.Version, request.Confirmation = plan.Version, plan.Confirmation
		if result, err := a.ApplyPortal(ctx, "biz", "device", request); err != nil || !result.Verified {
			t.Fatalf("media %q: %+v %v", media, result, err)
		}
		var rules []string
		for _, cmd := range r.commands {
			if strings.Contains(cmd, "walled-garden") && strings.Contains(cmd, " add ") {
				rules = append(rules, cmd)
			}
			// Only the IP walled garden is managed; the HTTP-level one and hotspot bypass bindings are never touched.
			for _, banned := range []string{"/ip hotspot walled-garden add", "/ip hotspot ip-binding", "type=bypassed", "/ip firewall"} {
				if strings.Contains(cmd, banned) {
					t.Fatalf("portal install must not run %q: %s", banned, cmd)
				}
			}
		}
		want := 1
		if media != "" {
			want = 2
		}
		if len(rules) != want {
			t.Fatalf("media %q: want %d walled-garden rules, got %v", media, want, rules)
		}
		for _, rule := range rules {
			for _, need := range []string{"action=accept", "protocol=tcp", `server="guest-server"`, "dst-port="} {
				if !strings.Contains(rule, need) {
					t.Errorf("rule is not scoped by %q: %s", need, rule)
				}
			}
			if strings.Contains(rule, "*") || strings.Contains(rule, "0.0.0.0") || regexp.MustCompile(`dst-port=[^ ]*[,-]`).MatchString(rule) {
				t.Errorf("rule uses a wildcard, an any-address or a port range: %s", rule)
			}
			portal := strings.Contains(rule, `dst-host="`+portalHost+`"`) && strings.Contains(rule, "dst-port=443 ") && !strings.Contains(rule, "dst-address")
			mediaRule := media != "" && strings.Contains(rule, `dst-address="`+media+`"`) && strings.Contains(rule, "dst-port=8096 ") && !strings.Contains(rule, "dst-host")
			if !portal && !mediaRule {
				t.Errorf("unexpected walled-garden rule: %s", rule)
			}
		}
	}
	// A public media address would publish a route out of the garden; it is refused before any router command.
	for _, public := range []string{"8.8.8.8", "207.148.76.172", "0.0.0.0", "example.com"} {
		if _, _, err := MediaTarget(public, "443"); err == nil {
			t.Errorf("media host %q must be refused", public)
		}
	}
}
