package routeros

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

type voucherSyncRunnerFunc func(context.Context, string) (string, error)

func (f voucherSyncRunnerFunc) Run(ctx context.Context, cmd string) (string, error) {
	return f(ctx, cmd)
}

func voucherSyncRecord(ref VoucherLogin, fields string) string {
	return voucherSyncMarker + ref.VoucherID + "__start\n" + fields + "\n" + voucherSyncMarker + ref.VoucherID + "__end\n"
}

func voucherSyncPresentFields(ref VoucherLogin) string {
	return "comment=" + voucherLoginPrefix + ref.VoucherID + "\ndisabled=no\nuptime=1w2d03:04:05\nbytes-in=1024\nbytes-out=2048\nmac=02:ab:cd:ef:12:34"
}

func TestVoucherSyncScriptReadsDocumentedOwnedUserFields(t *testing.T) {
	refs := voucherTestLogins()
	cmd := voucherSyncScript(refs)
	for _, ref := range refs {
		for _, fragment := range []string{
			`:put "__MM_VS__` + ref.VoucherID + `__start";`,
			`:put "__MM_VS__` + ref.VoucherID + `__end";`,
			`/ip hotspot user find where name="` + ref.Code + `"`,
			`[:find [/ip hotspot user get $u comment] "mega-magic:vlogin:` + ref.VoucherID + `"] != 0`,
			`/ip hotspot active find where user="` + ref.Code + `"`,
		} {
			if !strings.Contains(cmd, fragment) {
				t.Errorf("emitted command lacks %q", fragment)
			}
		}
	}
	for _, fragment := range []string{
		`:if ([:len $u] != 1) do={ :error "ambiguous voucher user"; };`,
		`:if ([:len $a] > 1) do={ :error "ambiguous voucher session"; };`,
		`:put ("uptime=" . [/ip hotspot user get $u uptime]);`,
		`:put ("disabled=" . [/ip hotspot user get $u disabled]);`,
		`:put ("bytes-in=" . [/ip hotspot user get $u bytes-in]);`,
		`:put ("bytes-out=" . [/ip hotspot user get $u bytes-out]);`,
		`:put ("mac=" . [/ip hotspot active get $a mac-address]);`,
	} {
		if strings.Count(cmd, fragment) != len(refs) {
			t.Errorf("expected a field/guard for every voucher: %q", fragment)
		}
	}
	if strings.Contains(cmd, "uptime-used") || strings.Contains(cmd, voucherSyncMarker+`"`) {
		t.Fatalf("invalid field or nested marker quote in command: %s", cmd)
	}
	if strings.Index(cmd, "[:len $u] != 1") > strings.Index(cmd, "get $u comment") ||
		strings.Index(cmd, "voucher ownership differs") > strings.Index(cmd, `:put ("disabled="`) {
		t.Fatal("user count and ownership guards must precede accounting reads")
	}
	for _, mutation := range []string{" add ", " set ", " remove ", " enable ", " disable ", " reset-counters"} {
		if strings.Contains(cmd, mutation) {
			t.Errorf("read-only accounting command contains %q", mutation)
		}
	}
}

func TestVoucherSyncRejectsInvalidReferencesBeforeRouterAccess(t *testing.T) {
	refs := voucherTestLogins()
	many := make([]VoucherLogin, 201)
	cases := map[string][]VoucherLogin{
		"empty": {}, "over batch bound": many,
		"bad UUID":              {{VoucherID: strings.Repeat("-", 36), Code: "VALIDCODE"}},
		"invalid UUID position": {{VoucherID: "1111111-11111-4111-8111-111111111111", Code: "VALIDCODE"}},
		"unsafe code":           {{VoucherID: refs[0].VoucherID, Code: `CODE";:error "unsafe`}},
		"duplicate ID":          {{VoucherID: refs[0].VoucherID, Code: "CODE1111"}, {VoucherID: refs[0].VoucherID, Code: "CODE2222"}},
		"duplicate code":        {{VoucherID: refs[0].VoucherID, Code: "CODE1111"}, {VoucherID: refs[1].VoucherID, Code: "CODE1111"}},
		"case duplicate ID":     {{VoucherID: "abcdef01-2345-6789-abcd-abcdef012345", Code: "CODE1111"}, {VoucherID: "ABCDEF01-2345-6789-ABCD-ABCDEF012345", Code: "CODE2222"}},
	}
	for name, invalid := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			_, err := readVoucherLogins(context.Background(), voucherSyncRunnerFunc(func(context.Context, string) (string, error) {
				called = true
				return "", nil
			}), invalid)
			if err == nil || called {
				t.Fatalf("invalid refs must fail before command execution: called=%v, err=%v", called, err)
			}
			// A nil source would panic if validation happened after acquiring credentials.
			a := &DeviceApplier{}
			if _, err := a.ReadVoucherLogins(context.Background(), "business", "device", invalid); err == nil {
				t.Fatal("public reader accepted invalid references")
			}
		})
	}
}

func TestVoucherSyncCompleteAccountingAndDisconnectedUsage(t *testing.T) {
	refs := voucherTestLogins()
	fields := voucherSyncPresentFields(refs[0])
	for _, active := range []bool{true, false} {
		t.Run(fmt.Sprint("active=", active), func(t *testing.T) {
			body := fields
			if !active {
				body = strings.Replace(body, "mac=02:ab:cd:ef:12:34", "mac=none", 1)
			}
			output := voucherSyncRecord(refs[0], body) + voucherSyncRecord(refs[1], "absent")
			states, err := readVoucherLogins(context.Background(), voucherSyncRunnerFunc(func(context.Context, string) (string, error) {
				return strings.ReplaceAll(output, "\n", "\r\n"), nil
			}), refs)
			if err != nil {
				t.Fatal(err)
			}
			wantMAC := ""
			if active {
				wantMAC = "02:AB:CD:EF:12:34"
			}
			want := VoucherLoginState{VoucherID: refs[0].VoucherID, Exists: true, UptimeSeconds: 788645, BytesUsed: 3072, ActiveMAC: wantMAC}
			if states[0] != want || states[1] != (VoucherLoginState{VoucherID: refs[1].VoucherID}) {
				t.Fatalf("accounting mismatch: %+v", states)
			}
		})
	}
}

func TestVoucherSyncRejectsIncompleteOrMalformedOutput(t *testing.T) {
	ref := voucherTestLogins()[0]
	fields := voucherSyncPresentFields(ref)
	good := voucherSyncRecord(ref, fields)
	mutate := func(old, new string) string { return strings.Replace(good, old, new, 1) }
	cases := map[string]string{
		"empty":                "",
		"marker only":          voucherSyncMarker + ref.VoucherID + "__start\n",
		"missing end":          strings.TrimSuffix(good, voucherSyncMarker+ref.VoucherID+"__end\n"),
		"missing counter":      mutate("bytes-in=1024\n", ""),
		"duplicate counter":    mutate("bytes-in=1024", "bytes-in=1024\nbytes-in=1024"),
		"duplicate field":      mutate("disabled=no", "disabled=no\ndisabled=no"),
		"unknown field":        mutate("disabled=no", "disabled=no\nstate=good"),
		"wrong duration field": mutate("uptime=", "uptime-used="),
		"wrong ownership":      mutate(voucherLoginPrefix+ref.VoucherID, voucherLoginPrefix+voucherTestLogins()[1].VoucherID),
		"invalid bool":         mutate("disabled=no", "disabled=off"),
		"empty bool":           mutate("disabled=no", "disabled="),
		"invalid duration":     mutate("uptime=1w2d03:04:05", "uptime=bad1h"),
		"negative counter":     mutate("bytes-in=1024", "bytes-in=-1"),
		"signed counter":       mutate("bytes-in=1024", "bytes-in=+1"),
		"counter overflow":     mutate("bytes-in=1024", "bytes-in=9223372036854775808"),
		"sum overflow":         mutate("bytes-in=1024", "bytes-in=9223372036854775807"),
		"invalid mac":          mutate("mac=02:ab:cd:ef:12:34", "mac=garbage"),
		"empty mac":            mutate("mac=02:ab:cd:ef:12:34", "mac="),
		"multicast mac":        mutate("mac=02:ab:cd:ef:12:34", "mac=01:00:5e:00:00:01"),
		"zero mac":             mutate("mac=02:ab:cd:ef:12:34", "mac=00:00:00:00:00:00"),
		"EUI64 mac":            mutate("mac=02:ab:cd:ef:12:34", "mac=02:ab:cd:ef:12:34:56:78"),
		"data before marker":   "unexpected\n" + good,
		"data after end":       good + "bytes-in=100\n",
		"duplicate voucher":    good + good,
		"foreign marker":       strings.ReplaceAll(good, ref.VoucherID, voucherTestLogins()[1].VoucherID),
		"wrong end":            mutate("__end", "__start"),
		"absence with data":    voucherSyncRecord(ref, "absent\n"+fields),
		"absence after data":   voucherSyncRecord(ref, fields+"\nabsent"),
		"duplicate absent":     voucherSyncRecord(ref, "absent\nabsent"),
		"unterminated absent":  voucherSyncMarker + ref.VoucherID + "__start\nabsent\n",
		"empty record":         voucherSyncRecord(ref, ""),
		"oversized":            strings.Repeat(" ", voucherSyncMaxBytes+1),
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			states, err := parseVoucherLoginStates(output, []VoucherLogin{ref})
			if err == nil || states != nil {
				t.Fatalf("malformed accounting must return no states, got %+v err=%v", states, err)
			}
		})
	}
	refs := voucherTestLogins()
	if _, err := parseVoucherLoginStates(voucherSyncRecord(refs[1], "absent")+voucherSyncRecord(refs[0], "absent"), refs); err == nil {
		t.Fatal("out-of-order records must be rejected")
	}
}

func TestVoucherSyncUptimeFormatsAndOverflow(t *testing.T) {
	valid := map[string]int64{
		"0": 0, "0s": 0, "00:00:00": 0, "30m": 1800,
		"1w2d3h4m5s": 788645, "1w2d03:04:05": 788645,
		"1d": 86400, "2w": 1209600, "25:00:00": 90000,
		"1h30m": 5400, "00:00:01.999999999": 1, "0.5s": 0,
		"9223372036854775807": math.MaxInt64, "9223372036854775807s": math.MaxInt64,
	}
	for raw, want := range valid {
		got, err := parseVoucherSyncUptime(raw)
		if err != nil || got != want {
			t.Errorf("%q: got %d err=%v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", " ", "-1s", "+1s", "wat1m", "1q2s", "1s2m", "1h1h", "1h2", "2:60:00", "24:00", "1.5h", "1.1234567890s", "9223372036854775808", "9223372036854775807w", "15250284452472w1d", "99999999999999999999:00:00"} {
		if _, err := parseVoucherSyncUptime(raw); err == nil {
			t.Errorf("invalid/overflowing duration %q accepted", raw)
		}
	}
}

func TestVoucherSyncMaximumBatchAndTransportFailure(t *testing.T) {
	refs := make([]VoucherLogin, voucherSyncMaxBatch)
	for i := range refs {
		refs[i] = VoucherLogin{VoucherID: fmt.Sprintf("%08x-1111-4111-8111-111111111111", i), Code: fmt.Sprintf("CODE%028d", i)}
	}
	calls := 0
	states, err := readVoucherLogins(context.Background(), voucherSyncRunnerFunc(func(_ context.Context, command string) (string, error) {
		if len(command) > 64*1024 {
			t.Fatalf("accounting command exceeds SSH argument bound: %d", len(command))
		}
		offset := calls * voucherSyncCommandBatch
		end := min(offset+voucherSyncCommandBatch, len(refs))
		calls++
		var output strings.Builder
		for _, ref := range refs[offset:end] {
			output.WriteString(voucherSyncRecord(ref, "absent"))
		}
		return output.String(), nil
	}), refs)
	if err != nil || len(states) != voucherSyncMaxBatch || calls != 4 {
		t.Fatalf("maximum batch failed: %d records err=%v", len(states), err)
	}
	_, err = readVoucherLogins(context.Background(), voucherSyncRunnerFunc(func(context.Context, string) (string, error) { return "", context.DeadlineExceeded }), refs[:1])
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("transport error not preserved: %v", err)
	}
}

func TestVoucherSyncLaterChunkFailureReturnsNoPartialStates(t *testing.T) {
	refs := make([]VoucherLogin, voucherSyncCommandBatch+1)
	for i := range refs {
		refs[i] = VoucherLogin{VoucherID: fmt.Sprintf("%08x-1111-4111-8111-111111111111", i), Code: fmt.Sprintf("CODE%08d", i)}
	}
	calls := 0
	states, err := readVoucherLogins(context.Background(), voucherSyncRunnerFunc(func(context.Context, string) (string, error) {
		calls++
		if calls > 1 {
			return "", context.DeadlineExceeded
		}
		var output strings.Builder
		for _, ref := range refs[:voucherSyncCommandBatch] {
			output.WriteString(voucherSyncRecord(ref, "absent"))
		}
		return output.String(), nil
	}), refs)
	if states != nil || !errors.Is(err, context.DeadlineExceeded) || calls != 2 {
		t.Fatalf("later chunk failure leaked partial accounting: states=%v calls=%d err=%v", states, calls, err)
	}
}

func TestVoucherSyncParsesRouterPersistedFirstUseAnchor(t *testing.T) {
	refs := voucherTestLogins()
	stamped := "comment=" + voucherLoginPrefix + refs[0].VoucherID + ";ft=1792941234;fu=02:ab:cd:ef:12:34\n" +
		"disabled=no\nuptime=30m\nbytes-in=1024\nbytes-out=2048\nmac=none"
	anonymous := strings.Replace(stamped, ";fu=02:ab:cd:ef:12:34", ";fu=-", 1)
	for name, body := range map[string]string{"withClient": stamped, "withoutClient": anonymous} {
		t.Run(name, func(t *testing.T) {
			output := voucherSyncRecord(refs[0], body) + voucherSyncRecord(refs[1], "absent")
			states, err := parseVoucherLoginStates(output, refs)
			if name == "withoutClient" {
				// Synthetic "-" MAC is no longer accepted by parseVoucherLoginComment.
				// The guard now requires a real client MAC for first-use anchoring.
				if err == nil {
					t.Fatal("expected synthetic '-' MAC to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if states[0].FirstUseEpoch != 1792941234 {
				t.Fatalf("first-use anchor lost: %+v", states[0])
			}
			wantMAC := "02:AB:CD:EF:12:34"
			if states[0].FirstMAC != wantMAC {
				t.Fatalf("first-use identity=%q, want %q", states[0].FirstMAC, wantMAC)
			}
		})
	}
	for name, comment := range map[string]string{
		"unknown suffix":     voucherLoginPrefix + refs[0].VoucherID + ";ft=1792941234",
		"suffix without ft":  voucherLoginPrefix + refs[0].VoucherID + ";fu=02:ab:cd:ef:12:34",
		"zero anchor":        voucherLoginPrefix + refs[0].VoucherID + ";ft=0;fu=-",
		"negative anchor":    voucherLoginPrefix + refs[0].VoucherID + ";ft=-5;fu=-",
		"signed anchor":      voucherLoginPrefix + refs[0].VoucherID + ";ft=+5;fu=-",
		"interval anchor":    voucherLoginPrefix + refs[0].VoucherID + ";ft=2735w1d21:41:43;fu=-",
		"empty anchor":       voucherLoginPrefix + refs[0].VoucherID + ";ft=;fu=-",
		"multicast identity": voucherLoginPrefix + refs[0].VoucherID + ";ft=1792941234;fu=01:00:5e:00:00:01",
		"zero identity":      voucherLoginPrefix + refs[0].VoucherID + ";ft=1792941234;fu=00:00:00:00:00:00",
		"garbage identity":   voucherLoginPrefix + refs[0].VoucherID + ";ft=1792941234;fu=garbage",
		"foreign base":       voucherLoginPrefix + refs[1].VoucherID + ";ft=1792941234;fu=-",
	} {
		t.Run(name, func(t *testing.T) {
			body := "comment=" + comment + "\ndisabled=no\nuptime=30m\nbytes-in=1024\nbytes-out=2048\nmac=none"
			if states, err := parseVoucherLoginStates(voucherSyncRecord(refs[0], body), refs[:1]); err == nil || states != nil {
				t.Fatalf("malformed anchor %q accepted: %+v", comment, states)
			}
		})
	}
}
