package routeros

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"mega-magic/internal/guestaccess"
)

var _ guestaccess.BindingVerifier = (*DeviceApplier)(nil)

func validateBinding(g guestaccess.BindingSnapshot) error {
	mac, err := net.ParseMAC(g.ClientMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || strings.ToUpper(mac.String()) != g.ClientMAC ||
		g.BusinessID == "" || g.DeviceID == "" || !recoveryUUID.MatchString(g.VoucherID) || g.TotalBytes <= 0 || g.TotalBytes > 1048576000000 || g.PriorUsedBytes < 0 || g.PriorUsedBytes >= g.TotalBytes || g.ExpiresAt.IsZero() || g.Revision.IsZero() {
		return fmt.Errorf("invalid current binding authority")
	}
	return nil
}

// Verify never creates, enables, disconnects or resets a user. For now it only
// measures disconnected bindings: live sessions need a separate accounting
// snapshot implementation, not addition of potentially overlapping counters.
func (a *DeviceApplier) Verify(ctx context.Context, g guestaccess.BindingSnapshot) (int64, error) {
	if err := validateBinding(g); err != nil {
		return 0, err
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, g.BusinessID, g.DeviceID)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return verifyGuestBinding(ctx, r, g, time.Now())
}

func verifyGuestBinding(ctx context.Context, r CommandRunner, g guestaccess.BindingSnapshot, now time.Time) (int64, error) {
	if err := validateBinding(g); err != nil {
		return 0, err
	}
	started := time.Now()
	claim := guestaccess.RecoveryClaim{BusinessID: g.BusinessID, DeviceID: g.DeviceID, VoucherID: g.VoucherID, OldMAC: g.ClientMAC, NewMAC: g.ClientMAC, ExpiresAt: g.ExpiresAt}
	if _, err := recoveryClock(ctx, r, claim, now); err != nil {
		return 0, err
	}
	host, err := recoveryHost(ctx, r, g.ClientMAC)
	if err != nil {
		return 0, err
	}
	u, err := readRecoveryUser(ctx, r, claim, g.ClientMAC)
	if err != nil {
		return 0, err
	}
	allocated := g.TotalBytes - g.PriorUsedBytes
	cap, e := strconv.ParseInt(u["limit-bytes-total"], 10, 64)
	if e != nil || cap != allocated || (u["disabled"] != "no" && u["disabled"] != "false") || u["server"] != host["server"] || u["profile"] == "" {
		return 0, fmt.Errorf("current binding allowance differs or is disabled")
	}
	// Find only the current MAC's guard; old transferred bindings have separate
	// scripts. Ambiguity is denied, never resolved by picking the first row.
	selector := "on-event~" + routerOSQuoted(g.ClientMAC)
	if err := recoveryCount(ctx, r, "/system scheduler print count-only where "+selector, "1"); err != nil {
		return 0, err
	}
	out, err := r.Run(ctx, "/system scheduler print value-list detail where "+selector)
	guard := parseValueList(out)
	name := guard["name"]
	var event string
	if name == "mm-voucher-"+g.VoucherID {
		if g.PriorUsedBytes != 0 {
			return 0, fmt.Errorf("transferred binding uses original guard")
		}
		event = voucherExpiryEvent(g.VoucherID, g.ClientMAC, g.ExpiresAt, name)
	} else if strings.HasPrefix(name, "mm-recovery-") && recoveryUUID.MatchString(strings.TrimPrefix(name, "mm-recovery-")) {
		event = recoveryExpiryEvent(claim, name)
	} else {
		return 0, fmt.Errorf("current binding expiry guard is not owned")
	}
	if err != nil || guard["on-event"] != event || parseUptimeSeconds(guard["interval"]) != 10 || (guard["disabled"] != "no" && guard["disabled"] != "false") || guard["start-time"] != "startup" {
		return 0, fmt.Errorf("current binding fixed expiry guard did not verify")
	}
	// The router-side checks bracket persistent-counter reads. An active login
	// racing this snapshot is denied; do not kick an already-connected guest.
	usage := ":local id [/ip hotspot user find " + recoverySelector(claim, g.ClientMAC) + "]; :if ([:len $id] != 1) do={ :error \"current user count differs\" }; :if ([:len [/ip hotspot active find user=" + routerOSQuoted(g.ClientMAC) + "]] != 0) do={ :error \"current session is active\" }; :put (\"bytes-in: \" . [/ip hotspot user get $id bytes-in]); :put (\"bytes-out: \" . [/ip hotspot user get $id bytes-out]); :if ([:len [/ip hotspot active find user=" + routerOSQuoted(g.ClientMAC) + "]] != 0) do={ :error \"current session became active\" }"
	out, err = r.Run(ctx, usage)
	if err != nil {
		return 0, fmt.Errorf("cannot verify disconnected binding usage")
	}
	stats := parseValueList(out)
	in, e1 := strconv.ParseInt(stats["bytes-in"], 10, 64)
	outBytes, e2 := strconv.ParseInt(stats["bytes-out"], 10, 64)
	if e1 != nil || e2 != nil || in < 0 || outBytes < 0 || in >= allocated || outBytes >= allocated-in {
		return 0, fmt.Errorf("binding counters invalid or exhausted")
	}
	if err := recoveryCount(ctx, r, "/ip hotspot active print count-only where user="+routerOSQuoted(g.ClientMAC), "0"); err != nil {
		return 0, err
	}
	latest, err := readRecoveryUser(ctx, r, claim, g.ClientMAC)
	if err != nil || (latest["disabled"] != "no" && latest["disabled"] != "false") || latest["limit-bytes-total"] != u["limit-bytes-total"] || latest["server"] != u["server"] || latest["profile"] != u["profile"] {
		return 0, fmt.Errorf("binding changed during verification")
	}
	if _, err := recoveryClock(ctx, r, claim, now.Add(time.Since(started))); err != nil {
		return 0, err
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return in + outBytes, nil
}
