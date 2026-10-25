package routeros

import (
	"testing"
)

func TestVoucherLoginUserMatch(t *testing.T) {
	for name, tc := range map[string]struct {
		user       map[string]string
		l          VoucherLogin
		server     string
		wantOK     bool
		wantMigrate bool
		wantLegacyWithoutAnchor bool
	}{
		"exact match": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: true,
		},
		"legacy default profile": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          "default",
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: true,
			wantMigrate: true,
		},
		"different profile": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          "other",
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"disabled user": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "yes",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"different limits": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "500000000",
				"limit-uptime":      "1800",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"different voucher ID": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:xyz78901-2345-6789-abcd-9876543210zy",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"legacy with usage but no anchor": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          "default",
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
				"uptime":           "1800",
				"bytes-in":         "1000000",
				"bytes-out":        "500000",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: true,
			wantMigrate: true,
			wantLegacyWithoutAnchor: true,
		},
		"owned profile with usage but no anchor": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
				"uptime":           "1800",
				"bytes-in":         "1000000",
				"bytes-out":        "500000",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: true,
			wantMigrate: false,
			wantLegacyWithoutAnchor: true,
		},
		"owned profile with anchor": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab;ft=1792941234;fu=AA:BB:CC:DD:EE:FF",
				"disabled":         "no",
				"uptime":           "3600",
				"bytes-in":         "5000000",
				"bytes-out":        "2000000",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: true,
			wantMigrate: false,
			wantLegacyWithoutAnchor: false,
		},
		"malformed comment - wrong prefix": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:wrong-prefix-abcd1234-5678-90ab-cdef-1234567890ab",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"malformed comment - invalid UUID dashes": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234567890abcdef1234567890abcd",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"malformed comment - invalid anchor timestamp": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab;ft=notanumber;fu=AA:BB:CC:DD:EE:FF",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"malformed comment - invalid anchor MAC": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab;ft=1792941234;fu=invalid-mac",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"malformed comment - synthetic dash MAC": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab;ft=1792941234;fu=-",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
		"malformed comment - missing fu": {
			user: map[string]string{
				"name":             "VOUCHER123",
				"limit-bytes-total": "1000000000",
				"limit-uptime":      "3600",
				"server":           "hotspot1",
				"profile":          voucherGuardProfile,
				"comment":          "mega-magic:vlogin:abcd1234-5678-90ab-cdef-1234567890ab;ft=1792941234",
				"disabled":         "no",
			},
			l: VoucherLogin{
				Code:     "VOUCHER123",
				DataMB:   1000,
				Minutes:  60,
				VoucherID: "abcd1234-5678-90ab-cdef-1234567890ab",
			},
			server: "hotspot1",
			wantOK: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ok, migrate, legacyWithoutAnchor := voucherLoginUserMatch(tc.user, tc.l, tc.server)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if migrate != tc.wantMigrate {
				t.Errorf("migrate = %v, want %v", migrate, tc.wantMigrate)
			}
			if legacyWithoutAnchor != tc.wantLegacyWithoutAnchor {
				t.Errorf("legacyWithoutAnchor = %v, want %v", legacyWithoutAnchor, tc.wantLegacyWithoutAnchor)
			}
		})
	}
}