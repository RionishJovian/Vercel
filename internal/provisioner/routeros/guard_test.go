package routeros

import "testing"

func TestGuardTarget_allowsLab(t *testing.T) {
	cases := []struct {
		host string
		port int
	}{
		{"172.20.0.250", 22},
		{"172.20.0.250", 0},
		{"localhost", 22},
	}
	for _, c := range cases {
		if err := GuardTarget(c.host, c.port); err != nil {
			t.Fatalf("GuardTarget(%q,%d) = %v, want nil (lab/other)", c.host, c.port, err)
		}
	}
}

// Prod-shaped inputs MUST trip the guard. The production identifiers appear here
// only as negative test inputs — never as a target.
func TestGuardTarget_tripsOnProdShapedInput(t *testing.T) {
	cases := []struct {
		name string
		host string
		port int
	}{
		{"live CCR2004", "10.77.0.4", 22},
		{"live hub peer", "10.77.0.1", 22},
		{"any 10.77.0.x", "10.77.0.99", 22},
		{"live hub domain", "hub.mikromagic.com", 22},
		{"bare prod domain", "mikromagic.com", 22},
		{"prod subdomain", "anything.mikromagic.com", 22},
		{"prod wg port", "172.20.0.250", ProdHubPort},
		{"empty host", "", 22},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := GuardTarget(c.host, c.port); err == nil {
				t.Fatalf("GuardTarget(%q,%d) = nil, want refusal", c.host, c.port)
			}
		})
	}
}
