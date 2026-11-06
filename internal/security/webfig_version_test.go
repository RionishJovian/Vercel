package security

import "testing"

func TestWebFigProxyUnsafe(t *testing.T) {
	for version, want := range map[string]bool{
		"":        true,
		"garbage": true,
		"5.26":    true,
		"6.49.20": true,
		"6.49.21": false,
		"7.0":     true,
		"7.19.1":  true,
		"7.20":    true,
		"7.22.9":  true,
		"7.23.3":  true,
		"7.23.4":  false,
		"7.24":    true,
		"7.24.1":  true,
		"7.24.2":  false,
		"7.25":    false,
		"8.0":     false,
	} {
		if got := WebFigProxyUnsafe(version); got != want {
			t.Errorf("%q: got %v", version, got)
		}
	}
}
