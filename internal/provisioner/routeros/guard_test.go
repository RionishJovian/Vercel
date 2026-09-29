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

func TestValidateDisposableCIDR(t *testing.T) {
	for _, cidr := range []string{
		"172.20.0.0/16",
		"10.99.0.0/30",
		"192.168.99.0/24",
		"127.0.0.0/8",
		"fd42::/64",
		"::1/128",
	} {
		if err := ValidateDisposableCIDR(cidr); err != nil {
			t.Errorf("safe disposable CIDR %s rejected: %v", cidr, err)
		}
	}

	for _, cidr := range []string{
		"0.0.0.0/1",
		"128.0.0.0/1",
		"::/1",
		"8.8.8.8/32",
		"100.64.0.0/10",
		"10.0.0.0/8",    // overlaps the known production range
		"10.77.0.0/24",  // production itself
		"172.20.0.1/16", // noncanonical host bits
	} {
		if err := ValidateDisposableCIDR(cidr); err == nil {
			t.Errorf("unsafe disposable CIDR %s was accepted", cidr)
		}
	}
}
