package model

import "testing"

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"routeros bare", `/user add name=admin password=hunter2`, `/user add name=admin password=***`},
		{"routeros quoted", `/user add name=admin password="hu nter2"`, `/user add name=admin password=***`},
		{"colon form", `password: hunter2`, `password: ***`},
		{"uppercase key", `PASSWORD=hunter2`, `PASSWORD=***`},
		{"passphrase", `/interface wireless security-profiles set passphrase='a b c'`,
			`/interface wireless security-profiles set passphrase=***`},
		{"token", `token=abc123`, `token=***`},
		{"json body", `{"password":"x","user":"bob"}`, `{"password":***,"user":"bob"}`},
		{"psk", `/interface wireguard peers add psk=AAAABBBB`, `/interface wireguard peers add psk=***`},
		// must NOT touch unrelated fields
		{"no secret", `/interface enable [find name="ether12"]`, `/interface enable [find name="ether12"]`},
		{"name not a secret", `/user add name=admin group=full`, `/user add name=admin group=full`},
		{"empty", ``, ``},
		{"secret word alone without value", `password`, `password`},
	}
	for _, c := range cases {
		if got := RedactSecrets(c.in); got != c.want {
			t.Errorf("%s:\n  in   %q\n  got  %q\n  want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestRedactSecretsKeepsRealBody is a regression guard built from a REAL migrated
// body: redaction must be a no-op on the data we actually hold, so it cannot
// silently mangle every row in the log.
func TestRedactSecretsKeepsRealBody(t *testing.T) {
	real := `{"script":"/interface enable [find name=\"ether12\"]","insight_id":"iface-down-76cbd76b-01e1-4b42-8f7e-8ad248084b47-ether12","source":"apply_insight_fix"}`
	if got := RedactSecrets(real); got != real {
		t.Errorf("redaction altered a real body:\n  got %q", got)
	}
	if ContainsSecret(real) {
		t.Error("a real body was flagged as containing a secret")
	}
}

func TestContainsSecret(t *testing.T) {
	if !ContainsSecret(`password=hunter2`) {
		t.Error("expected password= to be detected")
	}
	if ContainsSecret(`/interface enable [find name="ether12"]`) {
		t.Error("plain command must not be flagged")
	}
}

// TestRedactSecretsNeverLeaks is the property that matters: whatever the input,
// no credential VALUE may survive.
func TestRedactSecretsNeverLeaks(t *testing.T) {
	const value = "SuperSecret123"
	inputs := []string{
		`password=` + value,
		`password="` + value + `"`,
		`password: ` + value,
		`{"password":"` + value + `"}`,
		`/user add name=a password=` + value + ` group=full`,
		`PASSWORD=` + value,
		`passphrase=` + value,
		`token=` + value,
	}
	for _, in := range inputs {
		if out := RedactSecrets(in); contains(out, value) {
			t.Errorf("value leaked: in %q out %q", in, out)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
