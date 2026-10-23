package routeros

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	voucherCodePattern        = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{3,31}$`)
	voucherLoginPrefix        = "mega-magic:vlogin:"
	voucherUUIDHexDashPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	voucherMACPattern         = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
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
		if !voucherCodePattern.MatchString(l.Code) || !voucherSyncIDPattern.MatchString(l.VoucherID) ||
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
	// Install/adopt the bounded shared expiry guard and the versioned owned
	// login profile before any voucher user exists: first-use evidence and
	// local elapsed enforcement must already be active at first login.
	if err := ensureVoucherLoginGuard(ctx, r); err != nil {
		return err
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
			ok, migrate, legacyWithoutAnchor := voucherLoginUserMatch(user, l, server)
			if !ok {
				return fmt.Errorf("existing HotSpot user differs from voucher %s", l.VoucherID)
			}
			// Refuse before changing the profile: installing an on-login hook on
			// an already-used, unanchored voucher could give its next login a
			// fresh deadline. Historical first use cannot be reconstructed.
			if legacyWithoutAnchor {
				return fmt.Errorf("voucher %s has prior usage but no first-use anchor; elapsed-validity enforcement not active. Manual review required; original profile, counters and limits were preserved", l.VoucherID)
			}
			// A legacy install kept vouchers on the owner's default profile,
			// which has no login/logout hooks and so never captured first use
			// or enforced elapsed validity. Move just the profile onto the
			// versioned owned profile; every other field is preserved.
			if migrate {
				if err := migrateVoucherLoginProfile(ctx, r, l, server); err != nil {
					return err
				}
			}
			continue
		}
		// The code is the user's name AND password: RouterOS validates it
		// locally. limit-uptime is the connected-usage cap; the shared
		// expiry guard anchors first use on the router and enforces the
		// advertised elapsed validity from that immutable anchor.
		cmd := fmt.Sprintf("/ip hotspot user add name=%s password=%s server=%s profile=%s comment=%s limit-uptime=%ds limit-bytes-total=%d",
			routerOSQuoted(l.Code), routerOSQuoted(l.Code), routerOSQuoted(server), routerOSQuoted(voucherGuardProfile), routerOSQuoted(comment), l.Minutes*60, int64(l.DataMB)*1000000)
		if _, err := r.Run(ctx, cmd); err != nil {
			return fmt.Errorf("cannot create voucher login user for %s", l.VoucherID)
		}
		out, err = r.Run(ctx, read)
		if err != nil {
			return fmt.Errorf("voucher login readback failed for %s", l.VoucherID)
		}
		ok, migrate, legacyWithoutAnchor := voucherLoginUserMatch(parseValueList(out), l, server)
		_ = migrate
		_ = legacyWithoutAnchor
		if !ok {
			return fmt.Errorf("voucher login readback did not match for %s", l.VoucherID)
		}
	}
	return nil
}

// VoucherLoginMatchResult represents the outcome of matching an existing
// HotSpot user to a voucher login.
type VoucherLoginMatchResult struct {
	OK                  bool
	Migrate             bool
	LegacyWithoutAnchor bool // has non-zero usage but no first-use anchor
}

// voucherLoginUserMatch reports whether an existing HotSpot user is the same
// voucher allowance, and whether it is a legacy default-profile user that must
// be migrated onto the versioned owned profile. It validates the complete
// owned comment (exact prefix plus the specific voucher identifier) and the
// full allowance, so a loose match can never adopt someone else's account.
// A user on any profile other than the owned profile or the legacy "default"
// is foreign, and a disabled user is not adopted.
//
// Returns (ok, migrate, legacyWithoutAnchor):
// - ok: exact match on all fields including ownership comment
// - migrate: user is on "default" profile and needs migration to owned profile
// - legacyWithoutAnchor: user has non-zero usage (uptime/bytes) but no first-use anchor
func voucherLoginUserMatch(user map[string]string, l VoucherLogin, server string) (ok, migrate, legacyWithoutAnchor bool) {
	bytes, err := strconv.ParseInt(user["limit-bytes-total"], 10, 64)
	_, _, ours := parseVoucherLoginComment(user["comment"], l.VoucherID)
	if err != nil || bytes != int64(l.DataMB)*1000000 || user["name"] != l.Code ||
		user["server"] != server || !ours || parseUptimeSeconds(user["limit-uptime"]) != l.Minutes*60 {
		return false, false, false
	}
	if user["disabled"] == "yes" || user["disabled"] == "true" {
		return false, false, false
	}
	hasUsage := false
	if uptimeStr := user["uptime"]; uptimeStr != "" {
		if u := parseUptimeSeconds(uptimeStr); u > 0 {
			hasUsage = true
		}
	}
	if bytesIn := user["bytes-in"]; bytesIn != "" {
		if b, _ := strconv.ParseInt(bytesIn, 10, 64); b > 0 {
			hasUsage = true
		}
	}
	if bytesOut := user["bytes-out"]; bytesOut != "" {
		if b, _ := strconv.ParseInt(bytesOut, 10, 64); b > 0 {
			hasUsage = true
		}
	}
	hasAnchor := false
	if comment := user["comment"]; comment != "" {
		_, _, ok := parseVoucherLoginComment(comment, l.VoucherID)
		if ok {
			// parseVoucherLoginComment returns ok=true only if the anchor is present
			// (ft=...;fu=...). A base comment without anchor returns ok=true but ft=0.
			// We need to check if the anchor suffix is actually present.
			base := voucherLoginPrefix + l.VoucherID
			if strings.HasPrefix(comment, base) {
				rest := comment[len(base):]
				if strings.HasPrefix(rest, ";ft=") {
					hasAnchor = true
				}
			}
		}
	}
	switch user["profile"] {
	case voucherGuardProfile:
		return true, false, hasUsage && !hasAnchor
	case "default":
		return true, true, hasUsage && !hasAnchor
	default:
		return false, false, false
	}
}

// migrateVoucherLoginProfile moves a legacy default-profile voucher onto the
// versioned owned profile so its login/logout hooks capture first use and
// enforce elapsed validity from the next login. Only `profile` is changed:
// the comment (including any persisted first-use anchor), limits, counters and
// disabled state are preserved, and no historical login time or client MAC is
// invented. The change is read back and verified before it is accepted.
func migrateVoucherLoginProfile(ctx context.Context, r CommandRunner, l VoucherLogin, server string) error {
	setCmd := "/ip hotspot user set [find where name=" + routerOSQuoted(l.Code) + "] profile=" + routerOSQuoted(voucherGuardProfile)
	if _, err := r.Run(ctx, setCmd); err != nil {
		return fmt.Errorf("cannot migrate legacy voucher profile for %s", l.VoucherID)
	}
	read := "/ip hotspot user print value-list detail where name=" + routerOSQuoted(l.Code)
	out, err := r.Run(ctx, read)
	if err != nil {
		return fmt.Errorf("cannot read migrated voucher profile for %s", l.VoucherID)
	}
	ok, migrate, legacyWithoutAnchor := voucherLoginUserMatch(parseValueList(out), l, server)
	if !ok || migrate || legacyWithoutAnchor {
		return fmt.Errorf("legacy voucher profile migration did not verify for %s", l.VoucherID)
	}
	return nil
}
