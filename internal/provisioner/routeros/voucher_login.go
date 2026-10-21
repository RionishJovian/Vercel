package routeros

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VoucherLogin is one issued internet voucher provisioned on the gateway
// itself. The RouterBOARD validates the code locally at login time (the code
// is the hotspot user's name and password), so a guest never needs the hosted
// site or the WAN to authenticate. Uptime and data limits are enforced by
// RouterOS from the guest's first login.
type VoucherLogin struct {
	VoucherID string
	Code      string
	Minutes   int
	DataMB    int
}

// voucherCodePattern keeps codes safe as RouterOS hotspot user names:
// printable, unambiguous, no quoting traps.
var voucherCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{3,31}$`)

const voucherLoginPrefix = "mega-magic:vlogin:"

func (a *DeviceApplier) ProvisionVoucherLogins(ctx context.Context, businessID, deviceID string, logins []VoucherLogin) error {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return provisionVoucherLogins(ctx, r, logins)
}

func provisionVoucherLogins(ctx context.Context, r CommandRunner, logins []VoucherLogin) error {
	if len(logins) == 0 || len(logins) > 200 {
		return fmt.Errorf("provision between 1 and 200 voucher logins at once")
	}
	seen := map[string]bool{}
	for _, l := range logins {
		if !voucherCodePattern.MatchString(l.Code) || !guestRequestIDPattern.MatchString(l.VoucherID) ||
			l.Minutes < 1 || l.Minutes > 525600 || l.DataMB < 1 || l.DataMB > 1048576 {
			return fmt.Errorf("invalid voucher login allowance")
		}
		if seen[l.Code] {
			return fmt.Errorf("duplicate voucher code %q", l.Code)
		}
		seen[l.Code] = true
	}
	out, err := r.Run(ctx, "/ip hotspot print count-only where disabled=no")
	if err != nil {
		return fmt.Errorf("cannot read the gateway's HotSpot servers")
	}
	if strings.TrimSpace(out) != "1" {
		return fmt.Errorf("voucher login needs exactly one enabled HotSpot server on this gateway")
	}
	out, err = r.Run(ctx, "/ip hotspot print value-list detail where disabled=no")
	if err != nil {
		return fmt.Errorf("cannot read the gateway's HotSpot server")
	}
	server := parseValueList(out)["name"]
	if server == "" || !gatewayInterfacePattern.MatchString(server) {
		return fmt.Errorf("invalid HotSpot server on this gateway")
	}
	for _, l := range logins {
		comment := voucherLoginPrefix + l.VoucherID
		read := "/ip hotspot user print value-list detail where name=" + routerOSQuoted(l.Code)
		out, err := r.Run(ctx, read)
		if err != nil {
			return fmt.Errorf("cannot read HotSpot user for voucher %s", l.VoucherID)
		}
		user := parseValueList(out)
		if user["name"] != "" {
			// Retries reuse the same allowance: same voucher, same server and
			// limits, still enabled. Anything else belongs to someone else.
			if !voucherLoginUserMatches(user, l, server) {
				return fmt.Errorf("existing HotSpot user differs from voucher %s", l.VoucherID)
			}
			continue
		}
		// The code is the user's name AND password: RouterOS validates it
		// locally. limit-uptime counts from the guest's first login, so an
		// issued voucher stays valid until it is actually used.
		cmd := fmt.Sprintf("/ip hotspot user add name=%s password=%s server=%s profile=default comment=%s limit-uptime=%ds limit-bytes-total=%d",
			routerOSQuoted(l.Code), routerOSQuoted(l.Code), routerOSQuoted(server), routerOSQuoted(comment), l.Minutes*60, int64(l.DataMB)*1000000)
		if _, err := r.Run(ctx, cmd); err != nil {
			return fmt.Errorf("cannot create voucher login user for %s", l.VoucherID)
		}
		out, err = r.Run(ctx, read)
		if err != nil || !voucherLoginUserMatches(parseValueList(out), l, server) {
			return fmt.Errorf("voucher login readback did not match for %s", l.VoucherID)
		}
	}
	return nil
}

func voucherLoginUserMatches(user map[string]string, l VoucherLogin, server string) bool {
	bytes, err := strconv.ParseInt(user["limit-bytes-total"], 10, 64)
	return err == nil && bytes == int64(l.DataMB)*1000000 && parseUptimeSeconds(user["limit-uptime"]) == l.Minutes*60 &&
		user["name"] == l.Code && user["server"] == server && user["profile"] == "default" &&
		user["comment"] == voucherLoginPrefix+l.VoucherID && user["disabled"] != "yes" && user["disabled"] != "true"
}
