package auth

import "testing"

func TestValidPassword(t *testing.T) {
	long := ""
	for i := 0; i < 129; i++ {
		long += "a"
	}
	for _, tc := range []struct {
		p  string
		ok bool
	}{
		{"Abc123", true}, {"aB3dEf", true}, {"Zz9zZz9zZz9", true},
		{"Abc12", false},  // 5 characters
		{"abc123", false}, // no uppercase
		{"ABC123", false}, // no lowercase
		{"Abcdef", false}, // no digit
		{"", false}, {"      ", false},
		{"A1" + long, false}, // over 128
	} {
		if got := ValidPassword(tc.p); got != tc.ok {
			t.Errorf("ValidPassword(%q) = %v, want %v", tc.p, got, tc.ok)
		}
	}
}
