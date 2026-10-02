package routeros

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mega-magic/internal/guestaccess"
)

var recoveryUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var _ guestaccess.RecoveryRouter = (*DeviceApplier)(nil)

func validateRecovery(g guestaccess.RecoveryClaim) error {
	for _, raw := range []string{g.OldMAC, g.NewMAC} {
		mac, err := net.ParseMAC(raw)
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || strings.ToUpper(mac.String()) != raw {
			return fmt.Errorf("invalid recovery binding")
		}
	}
	if !recoveryUUID.MatchString(g.VoucherID) || !recoveryUUID.MatchString(g.TransferID) || g.OldMAC == g.NewMAC || g.TotalBytes <= 0 || g.TotalBytes > 1048576000000 || g.UsedBytes < 0 || g.UsedBytes >= g.TotalBytes || g.ExpiresAt.IsZero() {
		return fmt.Errorf("invalid recovery allowance")
	}
	return nil
}

func (a *DeviceApplier) recoveryRunner(ctx context.Context, g guestaccess.RecoveryClaim) (CommandRunner, error) {
	if err := validateRecovery(g); err != nil {
		return nil, err
	}
	return runnerForDevice(ctx, a.src, a.factory, a.timeout, g.BusinessID, g.DeviceID)
}

func (a *DeviceApplier) StopAndRead(ctx context.Context, g guestaccess.RecoveryClaim) (int64, error) {
	r, err := a.recoveryRunner(ctx, g)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return stopRecoveryAndRead(ctx, r, g, time.Now())
}
func (a *DeviceApplier) Activate(ctx context.Context, g guestaccess.RecoveryClaim, remaining int64) error {
	r, err := a.recoveryRunner(ctx, g)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return activateRecovery(ctx, r, g, remaining, time.Now())
}
func (a *DeviceApplier) Contain(ctx context.Context, g guestaccess.RecoveryClaim) error {
	r, err := a.recoveryRunner(ctx, g)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return containRecovery(ctx, r, g)
}

func recoveryComment(g guestaccess.RecoveryClaim) string { return "mega-magic:voucher:" + g.VoucherID }
func recoverySelector(g guestaccess.RecoveryClaim, mac string) string {
	return "name=" + routerOSQuoted(mac) + " comment=" + routerOSQuoted(recoveryComment(g))
}
func recoveryDisabled(user map[string]string) bool {
	return user["disabled"] == "yes" || user["disabled"] == "true"
}

func recoveryCount(ctx context.Context, r CommandRunner, cmd string, want string) error {
	out, err := r.Run(ctx, cmd)
	if err != nil || strings.TrimSpace(out) != want {
		return fmt.Errorf("recovery object count did not verify")
	}
	return nil
}
func readRecoveryUser(ctx context.Context, r CommandRunner, g guestaccess.RecoveryClaim, mac string) (map[string]string, error) {
	name := "name=" + routerOSQuoted(mac)
	if err := recoveryCount(ctx, r, "/ip hotspot user print count-only where "+name, "1"); err != nil {
		return nil, err
	}
	out, err := r.Run(ctx, "/ip hotspot user print value-list detail where "+name)
	if err != nil {
		return nil, fmt.Errorf("cannot read recovery user")
	}
	u := parseValueList(out)
	if u["name"] != mac || strings.ToUpper(u["mac-address"]) != mac || u["comment"] != recoveryComment(g) || u["server"] == "" || u["server"] == "all" {
		return nil, fmt.Errorf("recovery user ownership or binding differs")
	}
	return u, nil
}

func recoveryClock(ctx context.Context, r CommandRunner, g guestaccess.RecoveryClaim, now time.Time) (time.Time, error) {
	out, err := r.Run(ctx, ":put [:tonsec [:timestamp]]")
	nanos, e := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	at := time.Unix(0, nanos)
	if err != nil || e != nil || nanos <= 0 || absDuration(at.Sub(now)) > 5*time.Second || !at.Before(g.ExpiresAt) {
		return time.Time{}, fmt.Errorf("recovery clock or deadline did not verify")
	}
	return at, nil
}

func recoveryHost(ctx context.Context, r CommandRunner, mac string) (map[string]string, error) {
	selector := "mac-address=" + mac
	if err := recoveryCount(ctx, r, "/ip hotspot host print count-only where "+selector, "1"); err != nil {
		return nil, err
	}
	out, err := r.Run(ctx, "/ip hotspot host print value-list detail where "+selector)
	if err != nil {
		return nil, fmt.Errorf("cannot verify replacement Hotspot client")
	}
	h := parseValueList(out)
	if strings.ToUpper(h["mac-address"]) != mac || h["server"] == "" {
		return nil, fmt.Errorf("replacement client is not on this Hotspot")
	}
	return h, nil
}

func stopRecoveryAndRead(ctx context.Context, r CommandRunner, g guestaccess.RecoveryClaim, now time.Time) (int64, error) {
	if err := validateRecovery(g); err != nil {
		return 0, err
	}
	if _, err := recoveryClock(ctx, r, g, now); err != nil {
		return 0, err
	}
	host, err := recoveryHost(ctx, r, g.NewMAC)
	if err != nil {
		return 0, err
	}
	user, err := readRecoveryUser(ctx, r, g, g.OldMAC)
	if err != nil {
		return 0, err
	}
	cap, err := strconv.ParseInt(user["limit-bytes-total"], 10, 64)
	if err != nil || cap != g.TotalBytes-g.UsedBytes || recoveryDisabled(user) || user["server"] != host["server"] {
		return 0, fmt.Errorf("original recovery allowance is not live or differs")
	}
	if err := stopRecoveryBinding(ctx, r, g, g.OldMAC); err != nil {
		return 0, err
	}
	// Read persistent counters AFTER disconnect, not stale counters mid-session.
	out, err := r.Run(ctx, recoveryUsageCommand(g))
	if err != nil {
		return 0, fmt.Errorf("cannot verify final guest usage")
	}
	stats := parseValueList(out)
	in, e1 := strconv.ParseInt(stats["bytes-in"], 10, 64)
	outBytes, e2 := strconv.ParseInt(stats["bytes-out"], 10, 64)
	if e1 != nil || e2 != nil || in < 0 || outBytes < 0 || in > cap || outBytes > cap-in {
		return 0, fmt.Errorf("final guest usage is invalid or exhausted")
	}
	return in + outBytes, nil
}

func recoveryUsageCommand(g guestaccess.RecoveryClaim) string {
	return ":local id [/ip hotspot user find " + recoverySelector(g, g.OldMAC) + "]; :if ([:len $id] != 1) do={ :error \"recovery user count differs\" }; :put (\"bytes-in: \" . [/ip hotspot user get $id bytes-in]); :put (\"bytes-out: \" . [/ip hotspot user get $id bytes-out])"
}

func stopRecoveryBinding(ctx context.Context, r CommandRunner, g guestaccess.RecoveryClaim, mac string) error {
	if _, err := readRecoveryUser(ctx, r, g, mac); err != nil {
		return err
	}
	if _, err := r.Run(ctx, "/ip hotspot user disable [find "+recoverySelector(g, mac)+"]"); err != nil {
		return fmt.Errorf("cannot disable original guest binding")
	}
	if _, err := r.Run(ctx, "/ip hotspot active remove [find user="+routerOSQuoted(mac)+"]"); err != nil {
		return fmt.Errorf("cannot disconnect original guest")
	}
	if _, err := r.Run(ctx, "/ip hotspot cookie remove [find user="+routerOSQuoted(mac)+"]"); err != nil {
		return fmt.Errorf("cannot revoke original guest cookies")
	}
	u, err := readRecoveryUser(ctx, r, g, mac)
	if err != nil || !recoveryDisabled(u) {
		return fmt.Errorf("original binding remains enabled")
	}
	if err := recoveryCount(ctx, r, "/ip hotspot active print count-only where user="+routerOSQuoted(mac), "0"); err != nil {
		return err
	}
	return recoveryCount(ctx, r, "/ip hotspot cookie print count-only where user="+routerOSQuoted(mac), "0")
}

func activateRecovery(ctx context.Context, r CommandRunner, g guestaccess.RecoveryClaim, remaining int64, now time.Time) error {
	started := time.Now()
	if err := validateRecovery(g); err != nil {
		return err
	}
	if remaining <= 0 || remaining != g.TotalBytes-g.UsedBytes {
		return fmt.Errorf("replacement allowance differs from ledger")
	}
	at, err := recoveryClock(ctx, r, g, now)
	if err != nil {
		return err
	}
	old, err := readRecoveryUser(ctx, r, g, g.OldMAC)
	if err != nil || !recoveryDisabled(old) {
		return fmt.Errorf("original binding is not disabled")
	}
	if err := recoveryCount(ctx, r, "/ip hotspot user print count-only where comment="+routerOSQuoted(recoveryComment(g))+" disabled=no", "0"); err != nil {
		return err
	}
	host, err := recoveryHost(ctx, r, g.NewMAC)
	if err != nil {
		return err
	}
	if old["server"] != host["server"] || old["profile"] == "" {
		return fmt.Errorf("replacement server or user profile differs")
	}
	if err := recoveryCount(ctx, r, "/ip hotspot user print count-only where name="+routerOSQuoted(g.NewMAC), "0"); err != nil {
		return fmt.Errorf("replacement MAC already has a user; do not overwrite")
	}
	out, err := r.Run(ctx, "/ip hotspot print value-list detail where name="+routerOSQuoted(host["server"]))
	if err != nil {
		return fmt.Errorf("cannot verify replacement Hotspot")
	}
	profile := parseValueList(out)["profile"]
	if profile == "" {
		return fmt.Errorf("replacement server profile missing")
	}
	out, err = r.Run(ctx, "/ip hotspot profile print value-list detail where name="+routerOSQuoted(profile))
	if err != nil {
		return fmt.Errorf("cannot verify replacement login")
	}
	p := parseValueList(out)
	macLogin := false
	for _, method := range strings.Split(p["login-by"], ",") {
		if strings.TrimSpace(method) == "mac" {
			macLogin = true
		}
	}
	if !macLogin {
		return fmt.Errorf("replacement Hotspot needs MAC login")
	}
	// Unique per-transfer guard avoids rewriting the original user's guard.
	scheduler := "mm-recovery-" + g.TransferID
	event := recoveryExpiryEvent(g, scheduler)
	if err := recoveryCount(ctx, r, "/system scheduler print count-only where name="+routerOSQuoted(scheduler), "0"); err != nil {
		return fmt.Errorf("recovery guard already exists; reconciliation required")
	}
	if _, err = r.Run(ctx, "/system scheduler add name="+routerOSQuoted(scheduler)+" start-time=startup interval=10s policy=read,write on-event="+routerOSQuoted(event)); err != nil {
		return fmt.Errorf("cannot install recovery expiry guard")
	}
	out, err = r.Run(ctx, "/system scheduler print value-list detail where name="+routerOSQuoted(scheduler))
	guard := parseValueList(out)
	if err != nil || guard["name"] != scheduler || guard["on-event"] != event || parseUptimeSeconds(guard["interval"]) != 10 || recoveryDisabled(guard) {
		return fmt.Errorf("recovery guard did not verify")
	}
	seconds := int64(g.ExpiresAt.Sub(at) / time.Second)
	if seconds < 1 {
		return fmt.Errorf("recovery time exhausted")
	}
	cmd := fmt.Sprintf("/ip hotspot user add name=%s mac-address=%s server=%s password=%s profile=%s comment=%s limit-bytes-total=%d limit-uptime=%ds disabled=yes", routerOSQuoted(g.NewMAC), g.NewMAC, routerOSQuoted(host["server"]), routerOSQuoted(p["mac-auth-password"]), routerOSQuoted(old["profile"]), routerOSQuoted(recoveryComment(g)), remaining, seconds)
	if _, err = r.Run(ctx, cmd); err != nil {
		return fmt.Errorf("cannot create disabled replacement binding")
	}
	u, err := readRecoveryUser(ctx, r, g, g.NewMAC)
	if err != nil || !recoveryUserQuota(u, remaining, seconds, host["server"], old["profile"]) || !recoveryDisabled(u) {
		return fmt.Errorf("replacement quota did not verify")
	}
	if _, err := recoveryClock(ctx, r, g, now.Add(time.Since(started))); err != nil {
		return err
	}
	if err := verifyRecoveryStopped(ctx, r, g.OldMAC); err != nil {
		return err
	}
	if err := recoveryCount(ctx, r, "/ip hotspot user print count-only where comment="+routerOSQuoted(recoveryComment(g))+" disabled=no", "0"); err != nil {
		return err
	}
	if _, err = r.Run(ctx, "/ip hotspot user enable [find "+recoverySelector(g, g.NewMAC)+"]"); err != nil {
		return fmt.Errorf("replacement activation uncertain")
	}
	u, err = readRecoveryUser(ctx, r, g, g.NewMAC)
	if err != nil || recoveryDisabled(u) || !recoveryUserQuota(u, remaining, seconds, host["server"], old["profile"]) {
		return fmt.Errorf("replacement activation did not verify")
	}
	if err := verifyRecoveryStopped(ctx, r, g.OldMAC); err != nil {
		return err
	}
	_, err = recoveryClock(ctx, r, g, now.Add(time.Since(started)))
	return err
}

func recoveryExpiryEvent(g guestaccess.RecoveryClaim, scheduler string) string {
	return fmt.Sprintf(":if ([:timestamp] >= [:totime %s]) do={ :if ([:len [/ip hotspot user find %s]] > 0) do={ /ip hotspot user disable [find %s]; /ip hotspot active remove [find user=%s]; /ip hotspot cookie remove [find user=%s] }; /system scheduler remove [find name=%s] }", routerOSQuoted(fmt.Sprintf("%ds", g.ExpiresAt.Unix())), recoverySelector(g, g.NewMAC), recoverySelector(g, g.NewMAC), routerOSQuoted(g.NewMAC), routerOSQuoted(g.NewMAC), routerOSQuoted(scheduler))
}

func verifyRecoveryStopped(ctx context.Context, r CommandRunner, mac string) error {
	if err := recoveryCount(ctx, r, "/ip hotspot active print count-only where user="+routerOSQuoted(mac), "0"); err != nil {
		return err
	}
	return recoveryCount(ctx, r, "/ip hotspot cookie print count-only where user="+routerOSQuoted(mac), "0")
}

func recoveryUserQuota(u map[string]string, remaining, seconds int64, server, profile string) bool {
	cap, err := strconv.ParseInt(u["limit-bytes-total"], 10, 64)
	return err == nil && cap == remaining && int64(parseUptimeSeconds(u["limit-uptime"])) == seconds && u["server"] == server && u["profile"] == profile
}

func containRecovery(ctx context.Context, r CommandRunner, g guestaccess.RecoveryClaim) error {
	var first error
	for _, mac := range []string{g.OldMAC, g.NewMAC} {
		// Never mutate a foreign user, even after a partially completed transfer.
		out, err := r.Run(ctx, "/ip hotspot user print count-only where "+recoverySelector(g, mac))
		if err != nil {
			if first == nil {
				first = fmt.Errorf("cannot inspect recovery containment")
			}
			continue
		}
		if strings.TrimSpace(out) == "0" {
			continue
		}
		if strings.TrimSpace(out) != "1" {
			if first == nil {
				first = fmt.Errorf("ambiguous recovery containment")
			}
			continue
		}
		if err = stopRecoveryBinding(ctx, r, g, mac); err != nil && first == nil {
			first = err
		}
	}
	return first
}
