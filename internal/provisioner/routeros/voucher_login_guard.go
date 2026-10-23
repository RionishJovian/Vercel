package routeros

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	voucherGuardScriptName  = "mega-magic-voucher-guard-v1"
	voucherGuardScheduler   = "mega-magic-voucher-guard-v1"
	voucherGuardProfile     = "mega-magic-voucher-v1"
	voucherGuardMarker      = "mega-magic:voucher-guard:v1"
	voucherGuardProfileNote = "/system script run " + voucherGuardScriptName
	voucherGuardInterval    = "60s"
)

var (
	errVoucherGuardUnsupported = errors.New("gateway cannot anchor voucher first-use evidence")
	errVoucherGuardClock       = errors.New("gateway clock is not trustworthy for voucher first-use evidence")
	errVoucherGuardConflict    = errors.New("gateway already has a foreign or altered object under the voucher guard name")
)

const (
	voucherGuardEpochFloor = 1_000_000_000 // 2001-09-09, well after RouterOS 7 era
	voucherGuardEpochCeil  = 4_102_444_800 // 2100-01-01, well beyond plausible deployments
)

var voucherGuardEpochPattern = regexp.MustCompile(`^[0-9]+$`)

func probeVoucherGuardClock(ctx context.Context, r CommandRunner) (int64, error) {
	out, err := r.Run(ctx, ":put ([:tonsec [:timestamp]] / 1000000000)")
	if err != nil {
		return 0, fmt.Errorf("%w: :timestamp/:tonsec epoch-second probe failed", errVoucherGuardUnsupported)
	}
	text := strings.TrimSpace(out)
	if !voucherGuardEpochPattern.MatchString(text) {
		return 0, fmt.Errorf("%w: probe returned %q", errVoucherGuardUnsupported, text)
	}
	epoch, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: gateway clock reads %q", errVoucherGuardClock, text)
	}
	if epoch < voucherGuardEpochFloor || epoch > voucherGuardEpochCeil {
		return 0, fmt.Errorf("%w: gateway clock %d outside plausible epoch-second range [%d, %d]",
			errVoucherGuardClock, epoch, voucherGuardEpochFloor, voucherGuardEpochCeil)
	}
	return epoch, nil
}

func parseRouterOSDuration(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return 0, errors.New("negative duration")
		}
		return n, nil
	}
	if strings.HasSuffix(s, "s") {
		if n, err := strconv.ParseInt(strings.TrimSuffix(s, "s"), 10, 64); err == nil {
			if n < 0 {
				return 0, errors.New("negative duration")
			}
			return n, nil
		}
		return 0, errors.New("invalid seconds suffix")
	}
	if strings.HasSuffix(s, "m") {
		if n, err := strconv.ParseInt(strings.TrimSuffix(s, "m"), 10, 64); err == nil {
			if n < 0 || n > math.MaxInt64/60 {
				return 0, errors.New("invalid minutes value")
			}
			return n * 60, nil
		}
		return 0, errors.New("invalid minutes suffix")
	}
	if strings.HasSuffix(s, "h") {
		if n, err := strconv.ParseInt(strings.TrimSuffix(s, "h"), 10, 64); err == nil {
			if n < 0 || n > math.MaxInt64/3600 {
				return 0, errors.New("invalid hours value")
			}
			return n * 3600, nil
		}
		return 0, errors.New("invalid hours suffix")
	}
	parts := strings.Split(s, ":")
	if len(parts) == 3 {
		h, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || h < 0 || h > math.MaxInt64/3600 {
			return 0, errors.New("invalid hours component")
		}
		m, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || m < 0 || m > 59 {
			return 0, errors.New("invalid minutes component")
		}
		sec, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || sec < 0 || sec > 59 {
			return 0, errors.New("invalid seconds component")
		}
		total := h*3600 + m*60 + sec
		if total < 0 {
			return 0, errors.New("arithmetic overflow")
		}
		return total, nil
	}
	return 0, errors.New("unrecognized duration format")
}

// voucherGuardSourceLogin is the on-login hook script.
// It captures first-use evidence ONLY for the specific user that just logged in.
// Uses RouterOS login hook variable $user (the username that just logged in).
// Only stamps anchor on base-form comments (54 chars, no existing anchor).
const voucherGuardSourceLogin = `:local mmGuardVersion "mega-magic:voucher-guard:v1"; {
  :local now ([:tonsec [:timestamp]] / 1000000000);
  :if ([:typeof $now] != "num" or $now < 1000000000 or $now > 4102444800) do={ :return; };
  :local loginUser $user;
  :if ([:len $loginUser] = 0) do={ :return; };
  :local c [/ip hotspot user get $loginUser comment];
  :if ([:len $c] < 54 or [:pick $c 0 18] != "mega-magic:vlogin:") do={ :return; };
  :local id [:pick $c 18 54];
  :local validHex true;
  :for i from=0 to=35 do={
    :local ch [:pick $id $i ($i+1)];
    :if ($i = 8 or $i = 13 or $i = 18 or $i = 23) do={
      :if ($ch != "-") do={ :set validHex false; };
    } else={
      :if (($ch < "0" or $ch > "9") and ($ch < "a" or $ch > "f") and ($ch < "A" or $ch > "F")) do={ :set validHex false; };
    };
  };
  :if ([:len $id] != 36 or $validHex = false) do={ :return; };
  :local p [:find $c ";ft="];
  :if ([:typeof $p] != "nil") do={ :return; };
  :if ([:len $c] != 54) do={ :return; };
  :local a [/ip hotspot active find where user=$loginUser];
  :if ([:len $a] != 1) do={ :return; };
  :local mac [/ip hotspot active get $a mac-address];
  # Validate MAC format exactly: 17 chars, colons at 2,5,8,11,14
  :if ([:len $mac] != 17 or [:pick $mac 2 3] != ":" or [:pick $mac 5 6] != ":" or [:pick $mac 8 9] != ":" or [:pick $mac 11 12] != ":" or [:pick $mac 14 15] != ":") do={ :return; };
  # Validate each byte is valid hex
  :local validMacHex true;
  :for i from=0 to=16 do={
    :if ($i = 2 or $i = 5 or $i = 8 or $i = 11 or $i = 14) do={ :continue; };
    :local ch [:pick $mac $i ($i+1)];
    :if (($ch < "0" or $ch > "9") and ($ch < "a" or $ch > "f") and ($ch < "A" or $ch > "F")) do={ :set validMacHex false; };
  };
  :if ($validMacHex = false) do={ :return; };
  # Validate unicast (first byte LSB = 0) and not all-zero
  :local firstByte [:pick $mac 0 2];
  :local firstVal ([:tonum ("0x" . $firstByte)]);
  :if (($firstVal & 1) != 0) do={ :return; }; # multicast/broadcast rejected
  :if ($mac = "00:00:00:00:00:00") do={ :return; }; # all-zero rejected
  /ip hotspot user set $loginUser comment=($c . ";ft=" . $now . ";fu=" . $mac);
}`

// voucherGuardSourceExpiry is the scheduler script.
// It runs periodically to enforce elapsed expiry for vouchers that already have a first-use anchor.
// Validates complete anchor before disabling: valid epoch, valid MAC format, no trailing fields.
// Never creates new anchors.
const voucherGuardSourceExpiry = `:local mmGuardVersion "mega-magic:voucher-guard:v1"; {
  :local now ([:tonsec [:timestamp]] / 1000000000);
  :if ([:typeof $now] != "num" or $now < 1000000000 or $now > 4102444800) do={ :return; };
  :foreach u in=[/ip hotspot user find where comment~"mega-magic:vlogin:" disabled=no] do={
    :local c [/ip hotspot user get $u comment];
    :if ([:len $c] < 54 or [:pick $c 0 18] != "mega-magic:vlogin:") do={ :continue; };
    :local id [:pick $c 18 54];
    :local validHex true;
    :for i from=0 to=35 do={
      :local ch [:pick $id $i ($i+1)];
      :if ($i = 8 or $i = 13 or $i = 18 or $i = 23) do={
        :if ($ch != "-") do={ :set validHex false; };
      } else={
        :if (($ch < "0" or $ch > "9") and ($ch < "a" or $ch > "f") and ($ch < "A" or $ch > "F")) do={ :set validHex false; };
      };
    };
    :if ([:len $id] != 36 or $validHex = false) do={ :continue; };
    :local p [:find $c ";ft="];
    :if ([:typeof $p] = "nil" or $p != 54) do={ :continue; };
    :local f [:find $c ";fu="];
    :if ([:typeof $f] = "nil") do={ :continue; };
    :local fu [:pick $c ($f + 4) [:len $c]];
    :if ([:len $fu] != 17 or [:pick $fu 2 3] != ":" or [:pick $fu 5 6] != ":" or [:pick $fu 8 9] != ":" or [:pick $fu 11 12] != ":" or [:pick $fu 14 15] != ":") do={ :continue; };
    # Validate each MAC byte is valid hex
    :local validMacHex true;
    :for i from=0 to=16 do={
      :if ($i = 2 or $i = 5 or $i = 8 or $i = 11 or $i = 14) do={ :continue; };
      :local ch [:pick $fu $i ($i+1)];
      :if (($ch < "0" or $ch > "9") and ($ch < "a" or $ch > "f") and ($ch < "A" or $ch > "F")) do={ :set validMacHex false; };
    };
    :if ($validMacHex = false) do={ :continue; };
    # Validate unicast (first byte LSB = 0) and not all-zero
    :local firstByte [:pick $fu 0 2];
    :local firstVal ([:tonum ("0x" . $firstByte)]);
    :if (($firstVal & 1) != 0) do={ :continue; }; # multicast/broadcast rejected
    :if ($fu = "00:00:00:00:00:00") do={ :continue; }; # all-zero rejected
    :local epochText [:pick $c ($p + 4) $f];
    :if ([:len $epochText] != 10) do={ :continue; };
    :local validEpoch true;
    :for i from=0 to=9 do={
      :local digit [:pick $epochText $i ($i+1)];
      :if ($digit < "0" or $digit > "9") do={ :set validEpoch false; };
    };
    :if ($validEpoch = false) do={ :continue; };
    :local ft [:tonum $epochText];
    :local lim ([:tonsec [/ip hotspot user get $u limit-uptime]] / 1000000000);
    :if ([:typeof $ft] != "num" or [:typeof $lim] != "num" or $ft < 1000000000 or $ft > 4102444800 or $lim <= 0) do={ :continue; };
    :if (($now - $ft) >= $lim) do={
      /ip hotspot user disable $u;
      /ip hotspot active remove [find user=[/ip hotspot user get $u name]];
    };
  };
}`

// policyAllowsReadWrite reports whether a RouterOS `policy` value (comma or
// space separated) grants both read and write. The guard must hold write to
// set comments, disable users and remove sessions, so a policy that lacks
// either permission is a drift and is rejected.
func policyAllowsReadWrite(policy string) bool {
	read, write := false, false
	for _, p := range strings.FieldsFunc(policy, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' }) {
		switch strings.TrimSpace(p) {
		case "read":
			read = true
		case "write":
			write = true
		}
	}
	return read && write
}

// voucherGuardScriptIsOurs verifies the versioned guard script by its exact
// source and its permissions, not merely by a marker somewhere in the output.
// A script whose source has been altered (even one that still contains the
// marker), whose policy dropped read/write, or whose
// dont-require-permissions differs from the install contract is treated as a
// foreign object and never overwritten.
func voucherGuardScriptIsOurs(v map[string]string) bool {
	// Normalize both sources for comparison (handle whitespace differences)
	stored := normalizeScriptSource(v["source"])
	expected := normalizeScriptSource(voucherGuardSourceLogin)
	if stored != expected {
		return false
	}
	if !policyAllowsReadWrite(v["policy"]) {
		return false
	}
	if dp := v["dont-require-permissions"]; dp != "yes" && dp != "true" {
		return false
	}
	return true
}

// Ignore display indentation and line endings only. Collapsing all whitespace
// would also change string literals and hide meaningful configuration drift.
func normalizeScriptSource(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func voucherGuardSchedulerIsOurs(v map[string]string) bool {
	switch v["disabled"] {
	case "yes", "true":
		return false
	}
	intervalSec, err := parseRouterOSDuration(v["interval"])
	if err != nil || intervalSec != 60 {
		return false
	}
	// Scheduler runs the expiry script
	expectedOnEvent := "/system script run " + voucherGuardScriptName + "-expiry"
	if strings.TrimSpace(v["on-event"]) != expectedOnEvent {
		return false
	}
	if !policyAllowsReadWrite(v["policy"]) {
		return false
	}
	return true
}

func voucherGuardProfileIsOurs(v map[string]string) bool {
	expectedOnLogin := "/system script run " + voucherGuardScriptName + "-login"
	return strings.TrimSpace(v["on-login"]) == expectedOnLogin &&
		strings.TrimSpace(v["on-logout"]) == "" && v["shared-users"] == "1"
}

func readNamedValueList(ctx context.Context, r CommandRunner, menu, name string) (map[string]string, error) {
	out, err := r.Run(ctx, menu+" print value-list detail where name="+routerOSQuoted(name))
	if err != nil {
		return nil, err
	}
	return parseGuardValueList(out), nil
}

// RouterOS script source contains colons on nearly every line. Only a field
// label (letters/digits/hyphens) followed by a colon starts a new property;
// commands such as :local and quoted MAC addresses remain part of the source.
var guardPropertyPattern = regexp.MustCompile(`^([a-z][a-z0-9-]*):(?: |$)`)

func parseGuardValueList(out string) map[string]string {
	m := map[string]string{}
	var currentKey string
	var currentValue strings.Builder
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		field := guardPropertyPattern.FindStringSubmatch(line)
		if field == nil {
			// Continuation line
			if currentKey != "" {
				currentValue.WriteString("\n")
				currentValue.WriteString(line)
			}
			continue
		}
		// Save previous key-value pair
		if currentKey != "" {
			m[currentKey] = strings.TrimSpace(currentValue.String())
		}
		currentKey = field[1]
		currentValue.Reset()
		currentValue.WriteString(strings.TrimSpace(line[len(currentKey)+1:]))
	}
	// Save last key-value pair
	if currentKey != "" {
		m[currentKey] = strings.TrimSpace(currentValue.String())
	}
	return m
}

func ensureOrAdoptGuardObject(ctx context.Context, r CommandRunner, menu, name, addCmd string, isOurs func(map[string]string) bool) error {
	add := menu + " add " + addCmd
	count, err := r.Run(ctx, menu+" print count-only where name="+routerOSQuoted(name))
	if err != nil {
		return fmt.Errorf("read %s %q: %w", menu, name, err)
	}
	switch strings.TrimSpace(count) {
	case "0":
		if _, err := r.Run(ctx, add); err != nil {
			return fmt.Errorf("install %s %q: %w", menu, name, err)
		}
	case "1":
		// adopt below, after content verification
	default:
		return fmt.Errorf("%s %q did not resolve to exactly one object", menu, name)
	}
	fields, err := readNamedValueList(ctx, r, menu, name)
	if err != nil {
		return fmt.Errorf("verify %s %q: %w", menu, name, err)
	}
	if !isOurs(fields) {
		return errVoucherGuardConflict
	}
	count, err = r.Run(ctx, menu+" print count-only where name="+routerOSQuoted(name))
	if err != nil || strings.TrimSpace(count) != "1" {
		return fmt.Errorf("%s %q did not verify", menu, name)
	}
	return nil
}

func ensureVoucherLoginGuard(ctx context.Context, r CommandRunner) error {
	if _, err := probeVoucherGuardClock(ctx, r); err != nil {
		return err
	}
	// Install login hook script
	scriptLoginAdd := "name=" + routerOSQuoted(voucherGuardScriptName+"-login") +
		" policy=read,write dont-require-permissions=yes source=" + routerOSQuoted(voucherGuardSourceLogin)
	if err := ensureOrAdoptGuardObject(ctx, r, "/system script", voucherGuardScriptName+"-login", scriptLoginAdd, voucherGuardScriptIsOurs); err != nil {
		return err
	}
	// Install expiry script
	scriptExpiryAdd := "name=" + routerOSQuoted(voucherGuardScriptName+"-expiry") +
		" policy=read,write dont-require-permissions=yes source=" + routerOSQuoted(voucherGuardSourceExpiry)
	if err := ensureOrAdoptGuardObject(ctx, r, "/system script", voucherGuardScriptName+"-expiry", scriptExpiryAdd, func(v map[string]string) bool {
		return normalizeScriptSource(v["source"]) == normalizeScriptSource(voucherGuardSourceExpiry) && policyAllowsReadWrite(v["policy"]) && (v["dont-require-permissions"] == "yes" || v["dont-require-permissions"] == "true")
	}); err != nil {
		return err
	}
	// Scheduler runs the expiry script
	schedAdd := "name=" + routerOSQuoted(voucherGuardScheduler) +
		" interval=" + voucherGuardInterval + " on-event=" + routerOSQuoted("/system script run "+voucherGuardScriptName+"-expiry") + " policy=read,write"
	if err := ensureOrAdoptGuardObject(ctx, r, "/system scheduler", voucherGuardScheduler, schedAdd, voucherGuardSchedulerIsOurs); err != nil {
		return err
	}
	// Profile with on-login hook
	profileAdd := "name=" + routerOSQuoted(voucherGuardProfile) +
		" shared-users=1 on-login=" + routerOSQuoted("/system script run "+voucherGuardScriptName+"-login") + " on-logout=" + routerOSQuoted("")
	if err := ensureOrAdoptGuardObject(ctx, r, "/ip hotspot user profile", voucherGuardProfile, profileAdd, voucherGuardProfileIsOurs); err != nil {
		return err
	}
	return nil
}
