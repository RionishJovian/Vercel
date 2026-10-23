package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type voucherGuardRunner struct {
	clock      string
	scripts    map[string]string // name -> full value-list detail text
	schedulers map[string]string
	profiles   map[string]string
	commands   []string
}

func (r *voucherGuardRunner) Run(ctx context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	switch {
	case command == ":put ([:tonsec [:timestamp]] / 1000000000)":
		return r.clock, nil
	case strings.HasPrefix(command, "/system script print count-only where name="):
		return guardCount(r.scripts, command), nil
	case strings.HasPrefix(command, "/system script print value-list detail where name="):
		return guardBody(r.scripts, command), nil
	case strings.HasPrefix(command, "/system script add name="):
		return "", guardAdd(r.scripts, command, "script")
	case strings.HasPrefix(command, "/system scheduler print count-only where name="):
		return guardCount(r.schedulers, command), nil
	case strings.HasPrefix(command, "/system scheduler print value-list detail where name="):
		return guardBody(r.schedulers, command), nil
	case strings.HasPrefix(command, "/system scheduler add name="):
		return "", guardAdd(r.schedulers, command, "scheduler")
	case strings.HasPrefix(command, "/ip hotspot user profile print count-only where name="):
		return guardCount(r.profiles, command), nil
	case strings.HasPrefix(command, "/ip hotspot user profile print value-list detail where name="):
		return guardBody(r.profiles, command), nil
	case strings.HasPrefix(command, "/ip hotspot user profile add name="):
		return "", guardAdd(r.profiles, command, "profile")
	}
	return "", errors.New("unexpected guard command: " + command)
}

func guardName(command string) string {
	rest := command[strings.Index(command, "name=")+len("name="):]
	if i := strings.Index(rest, " "); i >= 0 {
		rest = rest[:i]
	}
	return strings.Trim(rest, `"`)
}

func guardCount(store map[string]string, command string) string {
	if _, ok := store[guardName(command)]; ok {
		return "1"
	}
	return "0"
}

func guardBody(store map[string]string, command string) string {
	return store[guardName(command)]
}

// scanQuoted consumes one `"..."` token starting at the opening quote,
// honouring RouterOS backslash escapes, and returns the unescaped value and
// the remaining text.
func scanQuoted(s string) (value, rest string, ok bool) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				b.WriteByte(s[i+1])
				i++
			}
		case '"':
			return b.String(), s[i+1:], true
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), "", false
}

// guardAddArgs parses the `key=value` arguments of an `add` command, where a
// value is either a bare token or a quoted token (which may contain spaces and
// escaped quotes, as the guard's `source` does).
func guardAddArgs(command string) map[string]string {
	args := map[string]string{}
	at := strings.Index(command, " add ")
	if at < 0 {
		return args
	}
	rest := command[at+len(" add "):]
	for {
		rest = strings.TrimLeft(rest, " ")
		if rest == "" {
			break
		}
		eq := strings.Index(rest, "=")
		if eq < 0 {
			break
		}
		keyStart := strings.LastIndex(rest[:eq], " ")
		key := rest[keyStart+1 : eq]
		valStart := eq + 1
		if rest[valStart] == '"' {
			val, remainder, ok := scanQuoted(rest[valStart:])
			args[key] = val
			if !ok {
				break
			}
			rest = remainder
			continue
		}
		end := strings.Index(rest[valStart:], " ")
		if end < 0 {
			args[key] = rest[valStart:]
			break
		}
		args[key] = rest[valStart : valStart+end]
		rest = rest[valStart+end:]
	}
	return args
}

// guardAdd records what a real router stores after `add`: the full value-list
// detail for the named object, built from the actual command arguments so an
// add that ships a wrong field cannot be masked by the fixture.
func guardAdd(store map[string]string, command, kind string) error {
	args := guardAddArgs(command)
	name := args["name"]
	if name == "" {
		return errors.New("add missing name")
	}
	if _, exists := store[name]; exists {
		return errors.New("object exists")
	}
	var b strings.Builder
	b.WriteString("name: " + name + "\n")
	switch kind {
	case "script":
		b.WriteString("policy: " + args["policy"] + "\n")
		b.WriteString("dont-require-permissions: " + args["dont-require-permissions"] + "\n")
		b.WriteString("source: " + args["source"] + "\n")
	case "scheduler":
		if args["disabled"] == "yes" {
			b.WriteString("disabled: yes\n")
		}
		b.WriteString("interval: " + args["interval"] + "\n")
		b.WriteString("on-event: " + args["on-event"] + "\n")
		b.WriteString("policy: " + args["policy"] + "\n")
	case "profile":
		b.WriteString("shared-users: " + args["shared-users"] + "\n")
		b.WriteString("on-login: " + args["on-login"] + "\n")
		b.WriteString("on-logout: " + args["on-logout"] + "\n")
	}
	store[name] = b.String()
	return nil
}

func guardScriptDetail(source string) string {
	return "name: " + voucherGuardScriptName + "\npolicy: read,write\ndont-require-permissions: yes\nsource: " + source + "\n"
}

func guardSchedulerDetail(interval, onEvent, policy string, disabled bool) string {
	var b strings.Builder
	b.WriteString("name: " + voucherGuardScheduler + "\n")
	if disabled {
		b.WriteString("disabled: yes\n")
	}
	b.WriteString("interval: " + interval + "\n")
	b.WriteString("on-event: " + onEvent + "\n")
	b.WriteString("policy: " + policy + "\n")
	return b.String()
}

func guardProfileDetail(onLogin, onLogout string) string {
	var b strings.Builder
	b.WriteString("name: " + voucherGuardProfile + "\n")
	if onLogin != "" {
		b.WriteString("on-login: " + onLogin + "\n")
	}
	if onLogout != "" {
		b.WriteString("on-logout: " + onLogout + "\n")
	}
	return b.String()
}

func TestVoucherGuardProbeRejectsUnsupportedOrUntrustworthyClock(t *testing.T) {
	for name, tc := range map[string]struct {
		clock  string
		runErr error
		want   string
	}{
		"probeFailed":  {runErr: errors.New("unknown command"), want: "anchor voucher first-use evidence"},
		"notANumber":   {clock: "nil", want: "anchor voucher first-use evidence"},
		"intervalForm": {clock: "2735w1d21:41:43", want: "anchor voucher first-use evidence"},
		"factoryClock": {clock: "42", want: "clock is not trustworthy"},
		"negative":     {clock: "-5", want: "anchor voucher first-use evidence"},
		// Finding 3: nanosecond-scale values should be rejected as implausible
		"nanosecondClock":  {clock: "1792941234000000000", want: "outside plausible epoch-second range"},
		"millisecondClock": {clock: "1792941234000", want: "outside plausible epoch-second range"},
		"farFutureClock":   {clock: "5000000000", want: "outside plausible epoch-second range"},
	} {
		t.Run(name, func(t *testing.T) {
			runner := voucherSyncRunnerFunc(func(_ context.Context, command string) (string, error) {
				if command != ":put ([:tonsec [:timestamp]] / 1000000000)" {
					t.Fatalf("probe must run first, got %q", command)
				}
				if tc.runErr != nil {
					return "", tc.runErr
				}
				return tc.clock, nil
			})
			err := ensureVoucherLoginGuard(context.Background(), runner)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestVoucherGuardInstallsLoginExpiryScriptsSchedulerAndOwnedProfile(t *testing.T) {
	r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{}, schedulers: map[string]string{}, profiles: map[string]string{}}
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	// The full login source must round-trip: the fixture stores exactly the source
	// shipped in the add command, and the verifier requires exact equality.
	escapedLoginSource := voucherGuardSourceLogin
	if detail := r.scripts[voucherGuardScriptName+"-login"]; !strings.Contains(detail, "source: "+escapedLoginSource) {
		t.Fatalf("login guard script source did not round-trip: %q", detail)
	}
	// The full expiry source must round-trip
	escapedExpirySource := voucherGuardSourceExpiry
	if detail := r.scripts[voucherGuardScriptName+"-expiry"]; !strings.Contains(detail, "source: "+escapedExpirySource) {
		t.Fatalf("expiry guard script source did not round-trip: %q", detail)
	}
	if !policyAllowsReadWrite("read,write") || policyAllowsReadWrite("read") || policyAllowsReadWrite("") {
		t.Fatal("policy helper misreads read/write")
	}
	if _, ok := r.schedulers[voucherGuardScheduler]; !ok {
		t.Fatal("shared expiry scheduler was not installed")
	}
	if _, ok := r.profiles[voucherGuardProfile]; !ok {
		t.Fatal("versioned owned login profile was not installed")
	}
	// Retry is idempotent: adopting existing objects must not re-add them.
	before := len(r.commands)
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("adoption failed: %v", err)
	}
	for _, command := range r.commands[before:] {
		if strings.Contains(command, " add ") {
			t.Errorf("retry added a duplicate object: %q", command)
		}
	}
}

func TestVoucherGuardMultilineReadbackAndLiteralDrift(t *testing.T) {
	// Literal colons in RouterOS commands, quoted ownership markers and MACs
	// must not split the source field into fake properties.
	detail := guardScriptDetail(voucherGuardSourceLogin) + "run-count: 2\n"
	fields := parseGuardValueList(detail)
	if normalizeScriptSource(fields["source"]) != normalizeScriptSource(voucherGuardSourceLogin) || fields["run-count"] != "2" || !voucherGuardScriptIsOurs(fields) {
		t.Fatalf("multiline readback did not preserve installed source: %#v", fields)
	}
	fields["source"] = strings.Replace(voucherGuardSourceLogin, `"mega-magic:voucher-guard:v1"`, `"mega-magic:voucher-guard:v1  "`, 1)
	if voucherGuardScriptIsOurs(fields) {
		t.Fatal("whitespace inside a quoted literal must remain detectable drift")
	}
	fields["source"] = voucherGuardSourceLogin
	delete(fields, "dont-require-permissions")
	if voucherGuardScriptIsOurs(fields) {
		t.Fatal("missing permission readback must not count as verified")
	}
}

func TestVoucherGuardReadbackDetectsWrongInstalledSource(t *testing.T) {
	r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{}, schedulers: map[string]string{}, profiles: map[string]string{}}
	changed := false
	wrapper := runnerFunc(func(ctx context.Context, cmd string) (string, error) {
		if strings.HasPrefix(cmd, "/system script add ") && !changed {
			cmd = strings.Replace(cmd, "1000000000", "1000000001", 1)
			changed = true
		}
		return r.Run(ctx, cmd)
	})
	if err := ensureVoucherLoginGuard(context.Background(), wrapper); !errors.Is(err, errVoucherGuardConflict) || !changed {
		t.Fatalf("wrong shipped source must fail its real readback, changed=%v err=%v", changed, err)
	}
}

// Codex reproduced ensureVoucherLoginGuard accepting a disabled scheduler with
// a 1d interval, a marker-only script, and a profile missing its logout hook.
// Each must now be rejected as configuration drift.
func TestVoucherGuardRejectsAlteredScheduler(t *testing.T) {
	note := voucherGuardProfileNote
	for name, detail := range map[string]string{
		"disabled":        guardSchedulerDetail(voucherGuardInterval, note, "read,write", true),
		"wrong interval":  guardSchedulerDetail("1d", note, "read,write", false),
		"wrong event":     guardSchedulerDetail(voucherGuardInterval, "/system script run something-else", "read,write", false),
		"policy no write": guardSchedulerDetail(voucherGuardInterval, note, "read", false),
	} {
		t.Run(name, func(t *testing.T) {
			r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{}, schedulers: map[string]string{voucherGuardScheduler: detail}, profiles: map[string]string{}}
			if err := ensureVoucherLoginGuard(context.Background(), r); !errors.Is(err, errVoucherGuardConflict) {
				t.Fatalf("altered scheduler accepted; err=%v", err)
			}
		})
	}
}

func TestVoucherGuardRejectsMarkerOnlyOrAlteredScript(t *testing.T) {
	for name, source := range map[string]string{
		"marker only":    `:local mmGuardVersion "mega-magic:voucher-guard:v1";`,
		"foreign body":   `{ :log info "not ours" }`,
		"missing damage": strings.Replace(voucherGuardSourceLogin, "/ip hotspot user set $loginUser comment=($c . \";ft=\" . $now . \";fu=\" . $mac);", "", 1),
		"wrong policy":   voucherGuardSourceLogin,
	} {
		t.Run(name, func(t *testing.T) {
			r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{voucherGuardScriptName + "-login": guardScriptDetail(source)}, schedulers: map[string]string{}, profiles: map[string]string{}}
			if name == "wrong policy" {
				r.scripts[voucherGuardScriptName+"-login"] = strings.Replace(guardScriptDetail(source), "policy: read,write", "policy: read", 1)
			}
			if err := ensureVoucherLoginGuard(context.Background(), r); !errors.Is(err, errVoucherGuardConflict) {
				t.Fatalf("altered script accepted; err=%v", err)
			}
		})
	}
}

func TestVoucherGuardRejectsMissingOrAlteredProfileHooks(t *testing.T) {
	note := voucherGuardProfileNote
	for name, detail := range map[string]string{
		"no logout hook":   guardProfileDetail(note, ""),
		"no login hook":    guardProfileDetail("", note),
		"wrong login hook": guardProfileDetail("/system script run something-else", note),
		"shared code":      "shared-users: 2\non-login: /system script run " + voucherGuardScriptName + "-login\non-logout: \n",
	} {
		t.Run(name, func(t *testing.T) {
			r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{}, schedulers: map[string]string{}, profiles: map[string]string{voucherGuardProfile: detail}}
			if err := ensureVoucherLoginGuard(context.Background(), r); !errors.Is(err, errVoucherGuardConflict) {
				t.Fatalf("altered profile accepted; err=%v", err)
			}
		})
	}
}

func TestVoucherGuardNeverOverwritesForeignObjects(t *testing.T) {
	r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{}, schedulers: map[string]string{}, profiles: map[string]string{}}
	r.scripts[voucherGuardScriptName+"-login"] = guardScriptDetail(`{ :log info "not ours" }`)
	r.scripts[voucherGuardScriptName+"-expiry"] = guardScriptDetail(`{ :log info "not ours" }`)
	r.schedulers[voucherGuardScheduler] = guardSchedulerDetail(voucherGuardInterval, "/system script run for-a-different-guard", "read,write", false)
	r.profiles[voucherGuardProfile] = guardProfileDetail("/system script run something-else", "/system script run something-else")
	err := ensureVoucherLoginGuard(context.Background(), r)
	if !errors.Is(err, errVoucherGuardConflict) {
		t.Fatalf("err=%v, want foreign-object conflict", err)
	}
	// No object may have been overwritten to force adoption.
	if !strings.Contains(r.scripts[voucherGuardScriptName+"-login"], "not ours") {
		t.Fatal("foreign login script was overwritten")
	}
	if !strings.Contains(r.scripts[voucherGuardScriptName+"-expiry"], "not ours") {
		t.Fatal("foreign expiry script was overwritten")
	}
	if !strings.Contains(r.profiles[voucherGuardProfile], "something-else") {
		t.Fatal("foreign profile was overwritten")
	}
}

// Finding 5: Verify real readback formats - scheduler interval parsing
func TestVoucherGuardSchedulerIntervalParsing(t *testing.T) {
	// Scheduler runs the expiry script
	expectedOnEvent := "/system script run " + voucherGuardScriptName + "-expiry"
	for name, interval := range map[string]string{
		"60s":      "60s",
		"1m":       "1m",
		"00:01:00": "00:01:00",
		"1h":       "1h",
		"30s":      "30s",
		"2m":       "2m",
		"invalid":  "invalid",
		"":         "",
	} {
		t.Run(name, func(t *testing.T) {
			detail := guardSchedulerDetail(interval, expectedOnEvent, "read,write", false)
			r := &voucherGuardRunner{clock: "1792941234", scripts: map[string]string{}, schedulers: map[string]string{voucherGuardScheduler: detail}, profiles: map[string]string{}}
			err := ensureVoucherLoginGuard(context.Background(), r)
			if name == "60s" || name == "1m" || name == "00:01:00" {
				if err != nil {
					t.Fatalf("valid interval %q rejected: %v", interval, err)
				}
			} else {
				if !errors.Is(err, errVoucherGuardConflict) {
					t.Fatalf("invalid interval %q accepted: %v", interval, err)
				}
			}
		})
	}
}

// Finding 4: Verify first-login capture path - on-login hook evidence
func TestVoucherGuardFirstLoginCapture(t *testing.T) {
	// The login hook script must enforce that first-use is captured at actual login:
	// 1. Uses $user variable (the specific user that just logged in)
	// 2. Comment is exactly 54 chars (base form: prefix + UUID, no anchor yet)
	// 3. Active session exists (len $a = 1) - MAC comes from active session
	// 4. No zero-traffic check needed; on-login hook fires at actual login
	requiredFragments := []string{
		`$user`,          // uses RouterOS login hook $user variable
		`[:len $c] < 54`, // exact base form check (len < 54 means no anchor)
		`[/ip hotspot active find where user=$loginUser]`, // live session lookup for login user
		`[:len $a] != 1`, // exactly one active session
		`/ip hotspot user set $loginUser comment=($c . ";ft=" . $now . ";fu=" . $mac)`, // anchor write for login user
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(voucherGuardSourceLogin, fragment) {
			t.Errorf("login guard source lacks required first-login capture fragment: %q", fragment)
		}
	}
	// Must NOT contain fragments that would allow non-first login to anchor
	forbiddenFragments := []string{
		"reset-counters",
		`uptime]] > 0`,
		`bytes-in] > 0`,
		`bytes-out] > 0`,
		`:local mac "-"`,
		// Old zero-traffic check removed - first login captured via hook
		`[/ip hotspot user get $u bytes-in]`,
		`$uin = 0 and $uout = 0`,
		// Should not scan all users
		`foreach u in=`,
	}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(voucherGuardSourceLogin, fragment) {
			t.Errorf("login guard source contains forbidden fragment for first-login capture: %q", fragment)
		}
	}
}

// Expiry script tests
func TestVoucherGuardExpiryScriptValidation(t *testing.T) {
	// Expiry validates complete anchor before disabling
	requiredFragments := []string{
		`[:find $c ";ft="]`,
		`[:find $c ";fu="]`,
		`[:len $fu] != 17`,
		`[:pick $fu 2 3] != ":"`,
		`[:pick $fu 5 6] != ":"`,
		`[:pick $fu 8 9] != ":"`,
		`[:pick $fu 11 12] != ":"`,
		`[:pick $fu 14 15] != ":"`,
		`$ft < 1000000000 or $ft > 4102444800 or $lim <= 0`,
		`($now - $ft) >= $lim`,
		`/ip hotspot user disable $u`,
		`/ip hotspot active remove [find user=[/ip hotspot user get $u name]]`,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(voucherGuardSourceExpiry, fragment) {
			t.Errorf("expiry guard source lacks required fragment: %q", fragment)
		}
	}
	// Must NOT create anchors
	forbiddenFragments := []string{
		`/ip hotspot user set $u comment=($c . ";ft=" . $now . ";fu=" . $mac)`,
		`:local mac`,
	}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(voucherGuardSourceExpiry, fragment) {
			t.Errorf("expiry guard source contains forbidden fragment: %q", fragment)
		}
	}
}

func TestVoucherGuardSourceAnchorsFirstUseAndEnforcesElapsedValidity(t *testing.T) {
	// Test login script
	for _, fragment := range []string{
		voucherGuardMarker,
		`[:tonsec [:timestamp]] / 1000000000`,
		`[:pick $c 0 18] != "mega-magic:vlogin:"`,
		`[:len $id] != 36`,
		`$i = 8 or $i = 13 or $i = 18 or $i = 23`,
		`$ch != "-"`,
		`$ch < "0" or $ch > "9"`,
		`$ch < "a" or $ch > "f"`,
		`$ch < "A" or $ch > "F"`,
		`:set validHex false`,
		`:local validHex true`,
		`[:find $c ";ft="]`,
		`[:len $c] != 54`,
		`[:len $a] != 1`,
		`/ip hotspot user set $loginUser comment=($c . ";ft=" . $now . ";fu=" . $mac)`,
	} {
		if !strings.Contains(voucherGuardSourceLogin, fragment) {
			t.Errorf("login guard source lacks %q", fragment)
		}
	}
	// First use captured at actual login via on-login hook; no zero-traffic check.
	for _, fragment := range []string{
		"reset-counters",
		`uptime]] > 0`,
		`bytes-in] > 0`,
		`bytes-out] > 0`,
		`:local mac "-"`,
		`[/ip hotspot user get $u bytes-in]`,
		`$uin = 0 and $uout = 0`,
		`foreach u in=`,
	} {
		if strings.Contains(voucherGuardSourceLogin, fragment) {
			t.Errorf("login guard source contains forbidden counter-based or synthetic stamp %q", fragment)
		}
	}

	// Test expiry script
	for _, fragment := range []string{
		voucherGuardMarker,
		`[:tonsec [:timestamp]] / 1000000000`,
		`/ip hotspot user find where comment~"mega-magic:vlogin:" disabled=no`,
		`[:pick $c 0 18] != "mega-magic:vlogin:"`,
		`[:len $id] != 36`,
		`$i = 8 or $i = 13 or $i = 18 or $i = 23`,
		`$ch != "-"`,
		`$ch < "0" or $ch > "9"`,
		`$ch < "a" or $ch > "f"`,
		`$ch < "A" or $ch > "F"`,
		`:set validHex false`,
		`:local validHex true`,
		`[:find $c ";ft="]`,
		`[:find $c ";fu="]`,
		`[:len $fu] != 17`,
		`[:pick $fu 2 3] != ":"`,
		`[:pick $fu 5 6] != ":"`,
		`[:pick $fu 8 9] != ":"`,
		`[:pick $fu 11 12] != ":"`,
		`[:pick $fu 14 15] != ":"`,
		`$ft < 1000000000 or $ft > 4102444800 or $lim <= 0`,
		`($now - $ft) >= $lim`,
		`/ip hotspot user disable $u`,
		`/ip hotspot active remove [find user=[/ip hotspot user get $u name]]`,
	} {
		if !strings.Contains(voucherGuardSourceExpiry, fragment) {
			t.Errorf("expiry guard source lacks %q", fragment)
		}
	}
	// Expiry never creates anchors
	for _, fragment := range []string{
		"reset-counters",
		`uptime]] > 0`,
		`bytes-in] > 0`,
		`bytes-out] > 0`,
		`:local mac "-"`,
		`/ip hotspot user set $u comment=($c . ";ft=" . $now . ";fu=" . $mac)`,
		`:local mac`,
	} {
		if strings.Contains(voucherGuardSourceExpiry, fragment) {
			t.Errorf("expiry guard source contains forbidden fragment %q", fragment)
		}
	}
}

// Regression test: Duration parsing rejects malformed values
func TestParseRouterOSDurationRejectsMalformed(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantSecs int64
	}{
		// Valid cases
		{"60s", "60s", false, 60},
		{"1m", "1m", false, 60},
		{"00:01:00", "00:01:00", false, 60},
		{"60", "60", false, 60},
		{"1h", "1h", false, 3600},
		// Invalid cases
		{"00:01:garbage", "00:01:garbage", true, 0},
		{"00:60:00", "00:60:00", true, 0},       // invalid minutes
		{"00:00:60", "00:00:60", true, 0},       // invalid seconds
		{"-1:00:00", "-1:00:00", true, 0},       // negative hours
		{"00:-1:00", "00:-1:00", true, 0},       // negative minutes
		{"00:00:-1", "00:00:-1", true, 0},       // negative seconds
		{"25:00:00", "25:00:00", false, 90000},  // RouterOS allows hours > 23
		{"00:01:00:00", "00:01:00:00", true, 0}, // too many components
		{"1x", "1x", true, 0},                   // invalid suffix
		{"empty", "", true, 0},                  // empty
		{"abc", "abc", true, 0},                 // not a number
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			secs, err := parseRouterOSDuration(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %d", tc.input, secs)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.input, err)
				}
				if secs != tc.wantSecs {
					t.Fatalf("expected %d seconds for %q, got %d", tc.wantSecs, tc.input, secs)
				}
			}
		})
	}
}
