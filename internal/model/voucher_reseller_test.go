package model

import "testing"

// Locks the ported CHECK semantics in code. The alphabet must stay identical to
// voucher_resellers_code_prefix_check in database/init.sql; the behavioural test
// in scripts/migration compares this constant against the live constraint.
func TestIsValidCodePrefix(t *testing.T) {
	valid := []string{"RS", "SM", "AB", "ABCD", "Z9", "Q7"}
	for _, p := range valid {
		if !IsValidCodePrefix(p) {
			t.Errorf("IsValidCodePrefix(%q) = false, want true", p)
		}
	}

	invalid := []struct{ p, why string }{
		{"", "empty"},
		{"S", "too short"},
		{"ABCDE", "too long"},
		{"s", "lowercase"},
		{"rs", "lowercase"},
		{"S1", "contains the excluded digit 1"},
		{"S0", "contains the excluded digit 0"},
		{"SI", "contains the excluded letter I"},
		{"SO", "contains the excluded letter O"},
		{"S-", "punctuation"},
		{"S ", "space"},
	}
	for _, c := range invalid {
		if IsValidCodePrefix(c.p) {
			t.Errorf("IsValidCodePrefix(%q) = true, want false (%s)", c.p, c.why)
		}
	}

	// L is deliberately ALLOWED by the ported constraint (only I, O, 0, 1 are
	// excluded). Guards against someone "tidying" the alphabet and silently
	// rejecting prefixes the database accepts.
	if !IsValidCodePrefix("LP") {
		t.Error("IsValidCodePrefix(\"LP\") = false; L is allowed by the ported constraint")
	}
}
