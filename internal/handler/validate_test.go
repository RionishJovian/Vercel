package handler

import "testing"

func TestIsUUID(t *testing.T) {
	valid := []string{
		"7987488d-f0e1-4e0a-9514-fe3aa00d9e16",
		"00000000-0000-4000-8000-000000000000",
		"ABCDEF01-2345-6789-ABCD-EF0123456789",
	}
	for _, s := range valid {
		if !isUUID(s) {
			t.Errorf("isUUID(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"", "not-a-uuid", "7987488d-f0e1-4e0a-9514-fe3aa00d9e1", // too short
		"7987488d-f0e1-4e0a-9514-fe3aa00d9e166", // too long
		"7987488df0e14e0a9514fe3aa00d9e16",      // no hyphens
		"7987488d-f0e1-4e0a-9514-fe3aa00d9e1z",  // non-hex
		"7987488d_f0e1_4e0a_9514_fe3aa00d9e16",  // wrong separator
		"7987488d-f0e1-4e0a-9514-fe3aa00d9e16-", // trailing dash
		"'; drop table sites; --",               // injection attempt
	}
	for _, s := range invalid {
		if isUUID(s) {
			t.Errorf("isUUID(%q) = true, want false", s)
		}
	}
}
