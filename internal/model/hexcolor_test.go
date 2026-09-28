package model

import "testing"

// TestIsHexColor pins the validator that guards the GUEST page's stylesheet.
//
// primary_hex / glass_tint_hex are owner-supplied strings interpolated into a
// public page's CSS. The guard must accept every legitimate way of writing a hex
// colour and reject everything else — in particular anything carrying a `;`,
// `{`, `}` or `url(` that could break out of the declaration and inject rules.
// A validator that is merely "close enough" is the whole vulnerability, so the
// reject table is the point of this test, not the accept table.
func TestIsHexColor(t *testing.T) {
	accept := []string{
		"#fff", "#FFF", "#000",
		"#ffff", "#ffffffff",
		"#ffb547", "#7ad0ff", "#0e5a5a",
		"#7C3AED", "#22d3ee",
		"#00000000",
	}
	for _, s := range accept {
		if !IsHexColor(s) {
			t.Errorf("IsHexColor(%q) = false, want true — a legitimate colour was rejected", s)
		}
	}

	reject := []struct{ in, why string }{
		{"", "empty"},
		{"#", "hash only"},
		{"#ff", "too short"},
		{"#fffff", "5 digits is not a valid hex form"},
		{"#fffffff", "7 digits is not a valid hex form"},
		{"#fffffffff", "too long"},
		{"red", "named colour"},
		{"rgb(255,0,0)", "functional notation"},
		{"ffb547", "missing the leading hash"},
		{"#ffb547; } body { display:none } .x{", "CSS breakout via semicolon+braces"},
		{"#fff;}body{display:none", "declaration escape"},
		{"red;background:url(javascript:alert(1))", "named colour plus url() payload"},
		{"#fff url(/x.png)", "trailing content after a valid colour"},
		{"#ffb547\nbody{display:none}", "newline injection"},
		{"#ggg", "non-hex letters"},
		{"#ffb54z", "one non-hex character"},
		{"var(--brand)", "CSS variable indirection"},
		{"expression(alert(1))", "legacy expression payload"},
		{"#fff/*", "comment opener"},
	}
	for _, c := range reject {
		if IsHexColor(c.in) {
			t.Errorf("IsHexColor(%q) = true, want false — %s reached the stylesheet", c.in, c.why)
		}
	}
}
