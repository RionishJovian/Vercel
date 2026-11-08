package routeros

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VIP staff Wi-Fi codes are intentionally NOT regular guest vouchers. Regular
// vouchers retain the dedicated one-device guard and immutable first-use
// quotas. A VIP code grants simultaneous logins up to an explicit bound with
// no time/data quota, so only an OWNER-facing dedicated workflow can issue it.
type VIPWiFiCode struct {
	ID         string
	Code       string
	MaxDevices int
}

var vipCodePattern = regexp.MustCompile(`^VIP-[ABCDEFGHJKMNPQRSTVWXYZ]{8}$`)

const vipProfilePrefix = "mega-magic-vip-v1-"
const vipCommentPrefix = "mega-magic:vip:"

func validVIPWiFi(v VIPWiFiCode) bool {
	return voucherUUIDHexDashPattern.MatchString(v.ID) &&
		vipCodePattern.MatchString(v.Code) && v.MaxDevices >= 2 && v.MaxDevices <= 8
}

func vipProfileName(maxDevices int) string {
	return vipProfilePrefix + strconv.Itoa(maxDevices)
}

// EnsureVIPWiFiCode can run again after an interrupted SSH response. It must
// inspect before writing: existing foreign user/profile objects are NEVER
// changed or adopted. No standard voucher profile is modified.
func (a *DeviceApplier) EnsureVIPWiFiCode(ctx context.Context, biz, deviceID string, v VIPWiFiCode) error {
	if !validVIPWiFi(v) {
		return errors.New("invalid VIP Wi-Fi code policy")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, biz, deviceID)
	if err != nil { return err }
	return ensureVIPWiFiCode(ctx, r, v)
}

func ensureVIPWiFiCode(ctx context.Context, r CommandRunner, v VIPWiFiCode) error {
	if !validVIPWiFi(v) { return errors.New("invalid VIP Wi-Fi code policy") }
	cnt, err := r.Run(ctx, "/ip hotspot print count-only where disabled=no")
	if err != nil || strings.TrimSpace(cnt) != "1" {
		return errors.New("VIP provisioning requires exactly one enabled Hotspot server")
	}
	raw, err := r.Run(ctx, "/ip hotspot print value-list detail where disabled=no")
	if err != nil { return errors.New("cannot read VIP Hotspot server") }
	server := parseValueList(raw)["name"]
	if server == "" || !gatewayInterfacePattern.MatchString(server) {
		return errors.New("VIP Hotspot server identity unavailable")
	}
	name, profile, comment := routerOSQuoted(v.Code), vipProfileName(v.MaxDevices), vipCommentPrefix+v.ID
	// Guard: the profile is only ours when the concurrency and lack of on-login
	// script/rate limit EXACTLY match. No automatic repair of altered profiles.
	profileCount, err := r.Run(ctx, "/ip hotspot user profile print count-only where name="+routerOSQuoted(profile))
	if err != nil { return errors.New("cannot inspect VIP profile") }
	switch strings.TrimSpace(profileCount) {
	case "0":
		if _, err = r.Run(ctx, "/ip hotspot user profile add name="+routerOSQuoted(profile)+" shared-users="+strconv.Itoa(v.MaxDevices)); err != nil {
			return errors.New("cannot install VIP profile")
		}
	case "1":
	default:
		return errors.New("ambiguous VIP profile")
	}
	raw, err = r.Run(ctx, "/ip hotspot user profile print value-list detail where name="+routerOSQuoted(profile))
	if err != nil { return errors.New("cannot verify VIP profile") }
	p := parseGuardValueList(raw)
	if p["name"] != profile || p["shared-users"] != strconv.Itoa(v.MaxDevices) ||
		p["on-login"] != "" || p["on-logout"] != "" || strings.TrimSpace(p["rate-limit"]) != "" {
		return errors.New("VIP profile conflicts with existing RouterOS configuration")
	}
	// Credentials are the code itself (name and password). This is a dedicated
	// staff-only class and never claims to be a metered guest entitlement.
	read := "/ip hotspot user print value-list detail where name=" + name
	raw, err = r.Run(ctx, read)
	if err != nil { return errors.New("cannot inspect VIP login") }
	user := parseGuardValueList(raw)
	if user["name"] == "" {
		// Leave unlimited time and byte limits unset. Other RouterOS logins and
		// normal voucher profiles are not touched.
		cmd := "/ip hotspot user add name="+name+" password="+name+
			" server="+routerOSQuoted(server)+" profile="+routerOSQuoted(profile)+
			" comment="+routerOSQuoted(comment)
		if _, err = r.Run(ctx, cmd); err != nil { return errors.New("cannot create VIP login") }
		raw, err = r.Run(ctx, read)
		if err != nil { return errors.New("VIP login readback unavailable") }
		user = parseGuardValueList(raw)
	}
	if err := verifyVIPWiFiUser(user, v, server); err != nil { return err }
	// Detect a profile replacement that occurred during provisioning.
	raw, err = r.Run(ctx, "/ip hotspot user profile print value-list detail where name="+routerOSQuoted(profile))
	if err != nil { return errors.New("VIP profile final check unavailable") }
	p = parseGuardValueList(raw)
	if p["name"] != profile || p["shared-users"] != strconv.Itoa(v.MaxDevices) ||
		p["on-login"] != "" || p["on-logout"] != "" || strings.TrimSpace(p["rate-limit"]) != "" {
		return errors.New("VIP profile changed before verification")
	}
	return nil
}

func verifyVIPWiFiUser(user map[string]string, v VIPWiFiCode, server string) error {
	if user["name"] != v.Code || user["comment"] != vipCommentPrefix+v.ID ||
		user["server"] != server || user["profile"] != vipProfileName(v.MaxDevices) ||
		user["disabled"] == "yes" || user["disabled"] == "true" {
		return errors.New("VIP login belongs to another owner, is disabled, or differs from reviewed policy")
	}
	// RouterOS usually omits zero-valued limits. Reject nonzero limits to
	// avoid falsely claiming a fully unlimited staff VIP profile.
	if strings.TrimSpace(user["limit-bytes-total"]) != "" && user["limit-bytes-total"] != "0" {
		return errors.New("VIP login unexpectedly has a data limit")
	}
	if d := strings.TrimSpace(user["limit-uptime"]); d != "" && d != "0s" && d != "0" {
		return errors.New("VIP login unexpectedly has a time limit")
	}
	if user["mac-address"] != "" && user["mac-address"] != "00:00:00:00:00:00" {
		return errors.New("VIP login is unexpectedly bound to a single MAC")
	}
	return nil
}

// RevokeVIPWiFiCode disconnects ONLY the exact owner-tagged VIP account,
// verifies its disabled state and never acts on a foreign Hotspot login.
func (a *DeviceApplier) RevokeVIPWiFiCode(ctx context.Context, biz, deviceID string, v VIPWiFiCode) error {
	if !validVIPWiFi(v) { return errors.New("invalid VIP Wi-Fi identity") }
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, biz, deviceID)
	if err != nil { return err }
	return revokeVIPWiFiCode(ctx, r, v)
}

func revokeVIPWiFiCode(ctx context.Context, r CommandRunner, v VIPWiFiCode) error {
	if !validVIPWiFi(v) { return errors.New("invalid VIP Wi-Fi identity") }
	name, comment := routerOSQuoted(v.Code), routerOSQuoted(vipCommentPrefix+v.ID)
	read := "/ip hotspot user print value-list detail where name="+name
	raw, err := r.Run(ctx, read)
	if err != nil { return errors.New("cannot inspect VIP login") }
	u := parseGuardValueList(raw)
	if u["name"] != v.Code || u["comment"] != vipCommentPrefix+v.ID ||
		u["profile"] != vipProfileName(v.MaxDevices) {
		return errors.New("VIP login ownership differs; refusing to revoke")
	}
	if u["disabled"] != "yes" && u["disabled"] != "true" {
		if _, err = r.Run(ctx, "/ip hotspot user disable [find where name="+name+" comment="+comment+"]"); err != nil {
			return errors.New("VIP login disable failed")
		}
	}
	// Disconnect live sessions only for the code that was just verified ours.
	if _, err = r.Run(ctx, "/ip hotspot active remove [find where user="+name+"]"); err != nil {
		return errors.New("VIP session removal uncertain")
	}
	raw, err = r.Run(ctx, read)
	if err != nil { return errors.New("VIP revocation readback unavailable") }
	u = parseGuardValueList(raw)
	if u["name"] != v.Code || u["comment"] != vipCommentPrefix+v.ID ||
		(u["disabled"] != "yes" && u["disabled"] != "true") {
		return errors.New("VIP revocation did not verify")
	}
	cnt, err := r.Run(ctx, "/ip hotspot active print count-only where user="+name)
	if err != nil || strings.TrimSpace(cnt) != "0" {
		return errors.New("VIP active session disconnect not verified")
	}
	return nil
}

var _ = fmt.Sprintf
