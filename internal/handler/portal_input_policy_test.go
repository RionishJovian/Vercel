package handler

import "testing"

func TestGuestPaymentDetails(t *testing.T) {
	for _, tc := range []struct {
		phone, digits string
		valid         bool
	}{
		{"+959123456789", "123456", true}, {"09123456789", "000001", true},
		{"", "123456", false}, {"09123456789", "abcdef", false},
		{"09123456789", "12345", false}, {"09<script>", "123456", false},
		{"09+123456789", "123456", false},
	} {
		if got := validGuestPaymentDetails(tc.phone, tc.digits); got != tc.valid {
			t.Errorf("%q/%q = %v", tc.phone, tc.digits, got)
		}
	}
}

func TestPortalImageURLPolicy(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"", true}, {"/assets/qr.png", true}, {"https://example.com/qr.png", true},
		{"javascript:alert(1)", false}, {"data:image/svg+xml,x", false},
		{"//example.com/qr.png", false}, {"http://example.com/qr.png", false},
		{"https://user:password@example.com/x", false}, {"/\\example.com/x", false},
		{"https://example.com/x\n", false},
	} {
		if got := safePortalImageURL(tc.url); got != tc.valid {
			t.Errorf("%q = %v", tc.url, got)
		}
	}
}
