package database

import (
	"errors"
	"mega-magic/internal/model"
	"testing"
	"time"
)

func TestGuestVoucherClaimPolicy(t *testing.T) {
	now := time.Date(2026, 10, 31, 23, 59, 30, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Second)
	const mac = "AA:BB:CC:DD:EE:FE"
	for _, tc := range []struct {
		name, state, status, bound string
		expires, lease             *time.Time
		want                       error
	}{
		{"first use", "unused", "unused", "", nil, nil, nil},
		{"active issued code", "unused", "active", "", nil, nil, nil},
		{"legacy used code", "unused", "used", "", nil, nil, ErrGuestVoucherUnavailable},
		{"worker in progress", "provisioning", "used", mac, &future, &future, ErrGuestVoucherBusy},
		{"crash retry same client", "provisioning", "used", mac, &future, &past, nil},
		{"crash retry foreign client", "provisioning", "used", "AA:BB:CC:DD:EE:FC", &future, &past, ErrGuestVoucherUnavailable},
		{"active same client", "active", "used", mac, &future, nil, nil},
		{"elapsed deadline exact", "active", "used", mac, &now, nil, ErrGuestVoucherUnavailable},
		{"revoked during provisioning", "provisioning", "revoked", mac, &future, &past, ErrGuestVoucherUnavailable},
		{"expired cannot renew", "expired", "used", mac, &future, nil, ErrGuestVoucherUnavailable},
		{"missing deadline", "active", "used", mac, nil, nil, ErrGuestVoucherUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &model.GuestVoucherGrant{State: tc.state, ClientMAC: tc.bound, ExpiresAt: tc.expires, LeaseUntil: tc.lease}
			got := guestVoucherClaimAllowed(g, tc.status, mac, now)
			if !errors.Is(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
