package routeros

import (
	"context"
	"fmt"
	"net"
	"strings"

	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

// GuestProvisionAdapter exposes fixed-entitlement operations only, not arbitrary
// RouterOS commands. It is intentionally not registered on public routes yet.
type GuestProvisionAdapter struct{ Applier *DeviceApplier }

var _ guestaccess.ProvisionRouter = GuestProvisionAdapter{}

func (a GuestProvisionAdapter) Ensure(ctx context.Context, g model.GuestVoucherGrant) error {
	if a.Applier == nil || g.ExpiresAt == nil {
		return fmt.Errorf("guest provisioning unavailable")
	}
	return a.Applier.ConfigureGuestVoucher(ctx, g.BusinessID, g.DeviceID, GuestVoucherAccess{
		VoucherID: g.VoucherID, ClientMAC: g.ClientMAC, Minutes: g.Minutes, DataMB: g.DataMB, ExpiresAt: *g.ExpiresAt,
	})
}

func (a GuestProvisionAdapter) Contain(ctx context.Context, g model.GuestVoucherGrant) error {
	mac, e := net.ParseMAC(g.ClientMAC)
	if a.Applier == nil || e != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || !guestRequestIDPattern.MatchString(g.VoucherID) {
		return fmt.Errorf("invalid guest containment scope")
	}
	r, e := runnerForDevice(ctx, a.Applier.src, a.Applier.factory, a.Applier.timeout, g.BusinessID, g.DeviceID)
	if e != nil {
		return e
	}
	return containGuestProvision(ctx, r, g.VoucherID, strings.ToUpper(mac.String()))
}

func containGuestProvision(ctx context.Context, r CommandRunner, id, mac string) error {
	name := routerOSQuoted(mac)
	selector := "name=" + name + " comment=" + routerOSQuoted("mega-magic:voucher:"+id)
	// The ownership conditional guards both session and cookie revocation.
	cmd := ":if ([:len [/ip hotspot user find " + selector + "]] > 0) do={ /ip hotspot user disable [find " + selector + "]; /ip hotspot active remove [find user=" + name + "]; /ip hotspot cookie remove [find user=" + name + "] }"
	if _, e := r.Run(ctx, cmd); e != nil {
		return fmt.Errorf("guest containment could not be confirmed")
	}
	out, e := r.Run(ctx, "/ip hotspot user print value-list detail where "+selector)
	if e != nil {
		return fmt.Errorf("guest containment readback failed")
	}
	user := parseValueList(out)
	if user["name"] == "" {
		return nil
	}
	if user["disabled"] != "yes" && user["disabled"] != "true" {
		return fmt.Errorf("guest user remains enabled")
	}
	for _, path := range []string{"active", "cookie"} {
		out, e = r.Run(ctx, "/ip hotspot "+path+" print count-only where user="+name)
		if e != nil || strings.TrimSpace(out) != "0" {
			return fmt.Errorf("guest session revocation did not verify")
		}
	}
	return nil
}
