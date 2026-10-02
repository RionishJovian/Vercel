package routeros

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// GuestVoucherAccess is a fixed, database-reserved entitlement, not a fresh
// duration granted on every HTTP retry. Both upload and download consume DataMB.
type GuestVoucherAccess struct {
	VoucherID string
	ClientMAC string
	Minutes   int
	DataMB    int
	ExpiresAt time.Time
}

func (a *DeviceApplier) ConfigureGuestVoucher(ctx context.Context, businessID, deviceID string, g GuestVoucherAccess) error {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return configureGuestVoucher(ctx, r, g, time.Now())
}

func configureGuestVoucher(ctx context.Context, r CommandRunner, g GuestVoucherAccess, now time.Time) error {
	mac, err := net.ParseMAC(g.ClientMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" ||
		!guestRequestIDPattern.MatchString(g.VoucherID) || g.Minutes < 1 || g.Minutes > 525600 ||
		g.DataMB < 1 || g.DataMB > 1048576 || !now.Before(g.ExpiresAt) || g.ExpiresAt.After(now.Add(time.Duration(g.Minutes)*time.Minute+time.Second)) {
		return fmt.Errorf("invalid or expired guest voucher allowance")
	}
	g.ClientMAC = strings.ToUpper(mac.String())
	// :timestamp is RouterOS epoch time: timezone and string-date ordering are
	// irrelevant. Reject unsupported syntax or an unsynchronized router BEFORE
	// writing a user. This helper requires timestamp-capable RouterOS.
	out, err := r.Run(ctx, ":put [:tonsec [:timestamp]]")
	if err != nil {
		return fmt.Errorf("router cannot verify an elapsed deadline")
	}
	nanos, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil || nanos <= 0 || absDuration(time.Unix(0, nanos).Sub(now)) > 30*time.Second {
		return fmt.Errorf("router clock must be synchronized before guest voucher provisioning")
	}
	out, err = r.Run(ctx, "/ip hotspot host print value-list detail where mac-address="+g.ClientMAC)
	if err != nil {
		return fmt.Errorf("cannot verify Hotspot client")
	}
	host := parseValueList(out)
	server := host["server"]
	if server == "" || strings.ToUpper(host["mac-address"]) != g.ClientMAC {
		return fmt.Errorf("client is not present on this gateway Hotspot")
	}
	out, err = r.Run(ctx, "/ip hotspot print value-list detail where name="+routerOSQuoted(server))
	if err != nil {
		return fmt.Errorf("cannot read Hotspot server")
	}
	profile := parseValueList(out)["profile"]
	if profile == "" {
		return fmt.Errorf("Hotspot server profile is unavailable")
	}
	out, err = r.Run(ctx, "/ip hotspot profile print value-list detail where name="+routerOSQuoted(profile))
	if err != nil {
		return fmt.Errorf("cannot read Hotspot login method")
	}
	fields := parseValueList(out)
	macLogin := false
	for _, method := range strings.Split(fields["login-by"], ",") {
		if strings.TrimSpace(method) == "mac" {
			macLogin = true
		}
	}
	if !macLogin {
		return fmt.Errorf("guest voucher access requires MAC login on this Hotspot")
	}
	name := routerOSQuoted(g.ClientMAC)
	comment := "mega-magic:voucher:" + g.VoucherID
	read := "/ip hotspot user print value-list detail where name=" + name
	out, err = r.Run(ctx, read)
	if err != nil {
		return fmt.Errorf("cannot read Hotspot user")
	}
	user := parseValueList(out)
	created := user["name"] == ""
	if !created && (user["comment"] != comment || !voucherUserMatches(user, g, server) || user["disabled"] == "yes" || user["disabled"] == "true") {
		return fmt.Errorf("existing user is foreign, disabled or has different quotas")
	}
	// A recurring check survives a missed one-shot event during reboot. At most
	// ten seconds of scheduler granularity; do not advertise exact-second expiry.
	// It never grants access, resets usage, changes other users or creates bypass.
	scheduler := "mm-voucher-" + g.VoucherID
	event := voucherExpiryEvent(g.VoucherID, g.ClientMAC, g.ExpiresAt, scheduler)
	out, err = r.Run(ctx, "/system scheduler print count-only where name="+routerOSQuoted(scheduler))
	if err != nil {
		return fmt.Errorf("cannot verify elapsed guard")
	}
	switch strings.TrimSpace(out) {
	case "0":
		_, err = r.Run(ctx, "/system scheduler add name="+routerOSQuoted(scheduler)+" start-time=startup interval=10s policy=read,write on-event="+routerOSQuoted(event))
		if err != nil {
			return fmt.Errorf("cannot install elapsed guard")
		}
	case "1":
	default:
		return fmt.Errorf("ambiguous elapsed guard")
	}
	out, err = r.Run(ctx, "/system scheduler print value-list detail where name="+routerOSQuoted(scheduler))
	guard := parseValueList(out)
	if err != nil || guard["name"] != scheduler || guard["on-event"] != event || parseUptimeSeconds(guard["interval"]) != 10 || guard["disabled"] == "yes" || guard["disabled"] == "true" {
		return fmt.Errorf("elapsed guard readback did not match")
	}
	if !created {
		if err := verifyVoucherDeadline(ctx, r, g.ExpiresAt); err != nil {
			containGuestVoucher(r, g)
			return err
		}
		return nil
	} // Retry preserves counters and deadline.
	cmd := fmt.Sprintf("/ip hotspot user add name=%s mac-address=%s server=%s password=%s profile=default comment=%s limit-uptime=%ds limit-bytes-total=%d disabled=yes", name, g.ClientMAC, routerOSQuoted(server), routerOSQuoted(fields["mac-auth-password"]), routerOSQuoted(comment), g.Minutes*60, int64(g.DataMB)*1000000)
	if _, err = r.Run(ctx, cmd); err != nil {
		return fmt.Errorf("cannot create guest voucher user")
	}
	out, err = r.Run(ctx, read)
	if err != nil || !voucherUserMatches(parseValueList(out), g, server) {
		return fmt.Errorf("guest voucher quota readback failed; user remains disabled")
	}
	// Configuration can take longer than the remaining validity. Never enable a
	// newly created user on the strength of the clock read at the start.
	if err := verifyVoucherDeadline(ctx, r, g.ExpiresAt); err != nil {
		return err
	}
	if _, err = r.Run(ctx, "/ip hotspot user enable [find name="+name+" comment="+routerOSQuoted(comment)+"]"); err != nil {
		containGuestVoucher(r, g)
		return fmt.Errorf("cannot enable verified guest voucher user")
	}
	out, err = r.Run(ctx, read)
	user = parseValueList(out)
	if err != nil || !voucherUserMatches(user, g, server) || user["disabled"] == "yes" || user["disabled"] == "true" {
		containGuestVoucher(r, g)
		return fmt.Errorf("guest voucher activation did not verify")
	}
	return nil
}

func voucherExpiryEvent(id, mac string, expiry time.Time, scheduler string) string {
	name, comment := routerOSQuoted(mac), routerOSQuoted("mega-magic:voucher:"+id)
	return fmt.Sprintf(":if ([:timestamp] >= [:totime %s]) do={ :if ([:len [/ip hotspot user find name=%s comment=%s]] > 0) do={ /ip hotspot user disable [find name=%s comment=%s]; /ip hotspot active remove [find user=%s] }; /system scheduler remove [find name=%s] }", routerOSQuoted(fmt.Sprintf("%ds", expiry.Unix())), name, comment, name, comment, name, routerOSQuoted(scheduler))
}

func verifyVoucherDeadline(ctx context.Context, r CommandRunner, expires time.Time) error {
	out, err := r.Run(ctx, ":put [:tonsec [:timestamp]]")
	nanos, clockErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil || clockErr != nil || nanos <= 0 || !time.Unix(0, nanos).Before(expires) {
		return fmt.Errorf("guest voucher deadline elapsed or cannot be verified")
	}
	return nil
}

// Best-effort containment uses a fresh bounded context because a failed enable
// request may have succeeded remotely even when its original context expired.
// The deadline guard remains installed if the router is unreachable.
func containGuestVoucher(r CommandRunner, g GuestVoucherAccess) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	selector := "name=" + routerOSQuoted(g.ClientMAC) + " comment=" + routerOSQuoted("mega-magic:voucher:"+g.VoucherID)
	// Disconnect only when the owned user exists; never kick an unrelated user.
	_, _ = r.Run(ctx, ":if ([:len [/ip hotspot user find "+selector+"]] > 0) do={ /ip hotspot user disable [find "+selector+"]; /ip hotspot active remove [find user="+routerOSQuoted(g.ClientMAC)+"] }")
}

func voucherUserMatches(user map[string]string, g GuestVoucherAccess, server string) bool {
	bytes, err := strconv.ParseInt(user["limit-bytes-total"], 10, 64)
	return err == nil && bytes == int64(g.DataMB)*1000000 && parseUptimeSeconds(user["limit-uptime"]) == g.Minutes*60 &&
		user["name"] == g.ClientMAC && strings.ToUpper(user["mac-address"]) == g.ClientMAC && user["server"] == server && user["comment"] == "mega-magic:voucher:"+g.VoucherID
}
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
