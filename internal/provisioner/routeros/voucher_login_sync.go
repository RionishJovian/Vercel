package routeros

import (
	"context"
	"fmt"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// VoucherLoginState is the RouterBOARD's own accounting for one issued
// voucher: usage counters, enable state and the live session's client MAC.
// The sync reads it back so the app reports what the router actually served.
// FirstUseEpoch/FirstMAC are the router-persisted first-use anchor written
// once by the shared expiry guard: epoch seconds of first successful use and
// the client identity seen at that login. They survive a short session and a
// Hub/app outage; zero/empty means no anchor has been persisted yet.
type VoucherLoginState struct {
	VoucherID     string
	Exists        bool
	Disabled      bool
	UptimeSeconds int64
	BytesUsed     int64
	ActiveMAC     string
	FirstUseEpoch int64
	FirstMAC      string
}

const (
	voucherSyncMarker       = "__MM_VS__"
	voucherSyncMaxBatch     = 200
	voucherSyncCommandBatch = 50
	voucherSyncMaxBytes     = 128 * 1024
)

var (
	voucherSyncIDPattern     = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	voucherSyncNumberPattern = regexp.MustCompile(`^[0-9]+$`)
	voucherSyncCompactUptime = regexp.MustCompile(`^(?:([0-9]+)w)?(?:([0-9]+)d)?(?:([0-9]+)h)?(?:([0-9]+)m)?(?:([0-9]+)(?:\.([0-9]{1,9}))?s)?$`)
	voucherSyncClockUptime   = regexp.MustCompile(`^(?:([0-9]+)w)?(?:([0-9]+)d)?([0-9]+):([0-5][0-9]):([0-5][0-9])(?:\.([0-9]{1,9}))?$`)
)

func validateVoucherSyncRefs(refs []VoucherLogin) error {
	if len(refs) == 0 || len(refs) > voucherSyncMaxBatch {
		return fmt.Errorf("sync between 1 and %d voucher logins at once", voucherSyncMaxBatch)
	}
	ids, codes := make(map[string]bool, len(refs)), make(map[string]bool, len(refs))
	for _, ref := range refs {
		if !voucherSyncIDPattern.MatchString(ref.VoucherID) || !voucherCodePattern.MatchString(ref.Code) {
			return fmt.Errorf("invalid voucher accounting reference")
		}
		id := strings.ToLower(ref.VoucherID)
		if ids[id] || codes[ref.Code] {
			return fmt.Errorf("duplicate voucher accounting reference")
		}
		ids[id], codes[ref.Code] = true, true
	}
	return nil
}

// voucherSyncScript uses only read commands and long-standing RouterOS script
// primitives. A code belonging to somebody else or an ambiguous session is an
// error, never evidence for one of our vouchers. End markers make partial SSH
// output distinguishable from complete records, including absent users.
func voucherSyncScript(refs []VoucherLogin) string {
	var b strings.Builder
	b.WriteString("{ ")
	for _, ref := range refs {
		code := routerOSQuoted(ref.Code)
		b.WriteString(":put " + routerOSQuoted(voucherSyncMarker+ref.VoucherID+"__start") + "; ")
		b.WriteString(":local u [/ip hotspot user find where name=" + code + "]; ")
		b.WriteString(":if ([:len $u] = 0) do={ :put \"absent\"; } else={ ")
		b.WriteString(":if ([:len $u] != 1) do={ :error \"ambiguous voucher user\"; }; ")
		// Ownership is a comment prefix match: the shared expiry guard appends
		// first-use evidence (;ft=..;fu=..) after provisioning, and Go validates
		// the exact comment shape. A foreign or malformed comment is an error,
		// never evidence for one of our vouchers.
		b.WriteString(":if ([:find [/ip hotspot user get $u comment] " + routerOSQuoted(voucherLoginPrefix+ref.VoucherID) + "] != 0) do={ :error \"voucher ownership differs\"; }; ")
		b.WriteString(":put (\"comment=\" . [/ip hotspot user get $u comment]); ")
		b.WriteString(":put (\"disabled=\" . [/ip hotspot user get $u disabled]); ")
		b.WriteString(":put (\"uptime=\" . [/ip hotspot user get $u uptime]); ")
		b.WriteString(":put (\"bytes-in=\" . [/ip hotspot user get $u bytes-in]); ")
		b.WriteString(":put (\"bytes-out=\" . [/ip hotspot user get $u bytes-out]); ")
		b.WriteString(":local a [/ip hotspot active find where user=" + code + "]; ")
		b.WriteString(":if ([:len $a] > 1) do={ :error \"ambiguous voucher session\"; }; ")
		b.WriteString(":if ([:len $a] = 0) do={ :put \"mac=none\"; } else={ :put (\"mac=\" . [/ip hotspot active get $a mac-address]); }; ")
		b.WriteString("}; :put " + routerOSQuoted(voucherSyncMarker+ref.VoucherID+"__end") + "; ")
	}
	b.WriteString("}")
	return b.String()
}

// ReadVoucherLogins reads one bounded batch. Callers with larger inventories
// must page the references and complete all reads before changing the ledger.
func (a *DeviceApplier) ReadVoucherLogins(ctx context.Context, businessID, deviceID string, refs []VoucherLogin) ([]VoucherLoginState, error) {
	if err := validateVoucherSyncRefs(refs); err != nil {
		return nil, err
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return readVoucherLogins(ctx, r, refs)
}

func readVoucherLogins(ctx context.Context, r CommandRunner, refs []VoucherLogin) ([]VoucherLoginState, error) {
	if err := validateVoucherSyncRefs(refs); err != nil {
		return nil, err
	}
	// Keep each SSH command below the operating system's single-argument
	// bound. A failed later chunk never returns a partial accounting batch.
	states := make([]VoucherLoginState, 0, len(refs))
	for offset := 0; offset < len(refs); offset += voucherSyncCommandBatch {
		end := min(offset+voucherSyncCommandBatch, len(refs))
		chunk := refs[offset:end]
		raw, err := r.Run(ctx, voucherSyncScript(chunk))
		if err != nil {
			return nil, fmt.Errorf("cannot read voucher login state: %w", err)
		}
		read, err := parseVoucherLoginStates(raw, chunk)
		if err != nil {
			return nil, err
		}
		states = append(states, read...)
	}
	return states, nil
}

func parseVoucherLoginStates(raw string, refs []VoucherLogin) ([]VoucherLoginState, error) {
	if len(raw) > voucherSyncMaxBytes {
		return nil, fmt.Errorf("voucher accounting output exceeds the batch limit")
	}
	states := make([]VoucherLoginState, 0, len(refs))
	var current *VoucherLoginState
	var fields map[string]bool
	absent := false
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, voucherSyncMarker) {
			if current == nil {
				if len(states) >= len(refs) || line != voucherSyncMarker+refs[len(states)].VoucherID+"__start" {
					return nil, fmt.Errorf("unexpected voucher accounting start marker")
				}
				current = &VoucherLoginState{VoucherID: refs[len(states)].VoucherID}
				fields, absent = make(map[string]bool, 6), false
				continue
			}
			if line != voucherSyncMarker+current.VoucherID+"__end" {
				return nil, fmt.Errorf("unexpected or duplicate voucher accounting marker")
			}
			if (!absent && len(fields) != 6) || (absent && len(fields) != 0) {
				return nil, fmt.Errorf("incomplete voucher accounting record")
			}
			current.Exists = !absent
			states = append(states, *current)
			current = nil
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("voucher state outside a marker")
		}
		if line == "absent" {
			if absent || len(fields) != 0 {
				return nil, fmt.Errorf("conflicting voucher accounting absence")
			}
			absent = true
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || absent || fields[key] {
			return nil, fmt.Errorf("invalid or duplicate voucher accounting field")
		}
		fields[key] = true
		switch key {
		case "comment":
			ft, mac, ok := parseVoucherLoginComment(value, current.VoucherID)
			if !ok {
				return nil, fmt.Errorf("voucher accounting ownership differs")
			}
			current.FirstUseEpoch, current.FirstMAC = ft, mac
		case "disabled":
			switch value {
			case "yes", "true":
				current.Disabled = true
			case "no", "false":
				current.Disabled = false
			default:
				return nil, fmt.Errorf("invalid voucher disabled state")
			}
		case "uptime":
			uptime, err := parseVoucherSyncUptime(value)
			if err != nil {
				return nil, err
			}
			current.UptimeSeconds = uptime
		case "bytes-in", "bytes-out":
			n, err := voucherSyncUnsigned(value)
			if err != nil || n > math.MaxInt64-current.BytesUsed {
				return nil, fmt.Errorf("invalid or overflowing voucher byte counter")
			}
			current.BytesUsed += n
		case "mac":
			if value == "none" {
				continue
			}
			mac, err := net.ParseMAC(value)
			if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
				return nil, fmt.Errorf("invalid voucher session MAC")
			}
			current.ActiveMAC = strings.ToUpper(mac.String())
		default:
			return nil, fmt.Errorf("unknown voucher accounting field")
		}
	}
	if current != nil || len(states) != len(refs) {
		return nil, fmt.Errorf("incomplete voucher accounting batch: %d of %d records", len(states), len(refs))
	}
	return states, nil
}

// parseVoucherLoginComment validates one owned voucher comment and extracts
// the router-persisted first-use anchor. ok=false means the comment is foreign
// or malformed: never evidence for one of our vouchers. The anchor is written
// exactly once by the shared expiry guard as `;ft=<epoch-seconds>;fu=<MAC>`.
// Validation rules (must match voucherGuardSource expiry branch):
// - Base prefix: "mega-magic:vlogin:" + voucherID (54 chars total)
// - Exactly one ";ft=" field followed by exactly one ";fu=" field
// - ft: positive integer epoch seconds
// - fu: valid unicast MAC (17 chars, colons at positions 2,5,8,11,14)
// - No trailing data after MAC
func parseVoucherLoginComment(value, voucherID string) (firstUseEpoch int64, firstMAC string, ok bool) {
	base := voucherLoginPrefix + voucherID
	if !strings.HasPrefix(value, base) {
		return 0, "", false
	}
	rest := value[len(base):]
	if rest == "" {
		return 0, "", true // base comment, no anchor yet
	}
	if !strings.HasPrefix(rest, ";ft=") {
		return 0, "", false
	}
	rest = rest[len(";ft="):]
	i := strings.Index(rest, ";fu=")
	if i < 0 {
		return 0, "", false
	}
	ftText, fu := rest[:i], rest[i+len(";fu="):]
	if !voucherSyncNumberPattern.MatchString(ftText) {
		return 0, "", false
	}
	ft, err := strconv.ParseInt(ftText, 10, 64)
	if err != nil || ft < voucherGuardEpochFloor || ft > voucherGuardEpochCeil {
		return 0, "", false
	}
	// Validate MAC format exactly: 17 chars, colons at 2,5,8,11,14
	if len(fu) != 17 ||
		fu[2] != ':' || fu[5] != ':' || fu[8] != ':' || fu[11] != ':' || fu[14] != ':' {
		return 0, "", false
	}
	mac, err := net.ParseMAC(fu)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return 0, "", false
	}
	return ft, strings.ToUpper(mac.String()), true
}

func voucherSyncUnsigned(value string) (int64, error) {
	if !voucherSyncNumberPattern.MatchString(value) {
		return 0, fmt.Errorf("invalid unsigned voucher accounting value")
	}
	return strconv.ParseInt(value, 10, 64)
}

// RouterOS time values may be compact (1w2d3h4m5s), clock-formatted
// (1w2d03:04:05), or include fractional seconds. Fractions are truncated at
// the API's whole-second precision; malformed/overflowing values are errors.
func parseVoucherSyncUptime(value string) (int64, error) {
	if voucherSyncNumberPattern.MatchString(value) {
		return voucherSyncUnsigned(value)
	}
	parts := voucherSyncCompactUptime.FindStringSubmatch(value)
	if parts == nil || value == "" {
		parts = voucherSyncClockUptime.FindStringSubmatch(value)
	}
	if parts == nil {
		return 0, fmt.Errorf("invalid voucher uptime")
	}
	var total int64
	for i, factor := range []int64{604800, 86400, 3600, 60, 1} {
		if parts[i+1] == "" {
			continue
		}
		n, err := voucherSyncUnsigned(parts[i+1])
		if err != nil || n > (math.MaxInt64-total)/factor {
			return 0, fmt.Errorf("overflowing voucher uptime")
		}
		total += n * factor
	}
	return total, nil
}
