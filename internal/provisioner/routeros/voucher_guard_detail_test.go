package routeros

import (
	"regexp"
	"strings"
	"testing"
)

// Real RouterOS 7.25 output for `/system script print value-list detail where name=...`.
func TestParseGuardValueListRouterOS725ScriptDetail(t *testing.T) {
	out := "0  name=\"mega-magic-voucher-guard-v1-login\" owner=\"mm-4fddeacf4dbf\" policy=read,write dont-require-permissions=yes run-count=0 source=\n" +
		"      :local mmGuardVersion \"mega-magic:voucher-guard:v1\"; {\n" +
		"        :local now ([:tonsec [:timestamp]] / 1000000000);\n" +
		"      }\n"
	if !looksLikeGuardDetail(out) {
		t.Fatal("detail layout not recognised")
	}
	m := parseGuardValueList(out)
	if m["name"] != "mega-magic-voucher-guard-v1-login" || m["owner"] != "mm-4fddeacf4dbf" {
		t.Fatalf("header fields: %#v", m)
	}
	if !policyAllowsReadWrite(m["policy"]) || m["dont-require-permissions"] != "yes" {
		t.Fatalf("permission fields: %#v", m)
	}
	want := normalizeScriptSource(":local mmGuardVersion \"mega-magic:voucher-guard:v1\"; {\n:local now ([:tonsec [:timestamp]] / 1000000000);\n}")
	if normalizeScriptSource(m["source"]) != want {
		t.Fatalf("source: %q", m["source"])
	}
}

func TestParseGuardValueListKeepsOlderLayout(t *testing.T) {
	out := "name: mega-magic-voucher-v1\nshared-users: 1\non-login: /system script run mega-magic-voucher-guard-v1-login\n"
	m := parseGuardValueList(out)
	if looksLikeGuardDetail(out) || m["shared-users"] != "1" || m["name"] != "mega-magic-voucher-v1" {
		t.Fatalf("older layout broke: %#v", m)
	}
}

func TestParseGuardDetailQuotedProfileScript(t *testing.T) {
	out := "1  name=\"mega-magic-voucher-v3\" shared-users=1 on-login=\":global mmGuardLoginUser $user; /system script run mega-magic-voucher-guard-v3-login\" on-logout=\"\"\n"
	m := parseGuardValueList(out)
	if !voucherGuardProfileIsOurs(m) {
		t.Fatalf("profile not recognised: %#v", m)
	}
}

func TestParseGuardValueListDetailWithFlagsHeader(t *testing.T) {
	out := "Flags: X - disabled \n" +
		" 0   name=\"mega-magic-voucher-guard-v1\" start-date=2026-11-01 start-time=12:45:07 interval=1m\n" +
		"     on-event=\"/system script run mega-magic-voucher-guard-v1-expiry\" owner=\"mm-4fddeacf4dbf\"\n" +
		"     policy=read,write run-count=0 next-run=2026-11-01 12:46:07\n"
	m := parseGuardValueList(out)
	if m["name"] != "mega-magic-voucher-guard-v1" || m["interval"] != "1m" || m["policy"] != "read,write" ||
		m["on-event"] != "/system script run mega-magic-voucher-guard-v1-expiry" {
		t.Fatalf("flags-header detail not parsed: %#v", m)
	}
}

func TestParseValueListCommentLineKeepsColons(t *testing.T) {
	out := ";;; mega-magic:vlogin:4886ae24\nname: WPWJGMNP\nlimit-uptime: 1h\n"
	m := parseValueList(out)
	if m["comment"] != "mega-magic:vlogin:4886ae24" || m["name"] != "WPWJGMNP" || m["limit-uptime"] != "1h" {
		t.Fatalf("comment line mis-parsed: %#v", m)
	}
}

// RouterOS 7.25 has no ordering operators for strings: "a" > "9" aborts the
// script with "cannot compare if string is more than string", which silently
// disabled first-use stamping and elapsed-expiry on a real hAP ax2.
func TestGuardScriptsNeverOrderStringsWithComparisonOperators(t *testing.T) {
	bad := regexp.MustCompile(`[<>]=?\s*"|"\s*[<>]=?`)
	for name, src := range map[string]string{"login": voucherGuardSourceLogin, "expiry": voucherGuardSourceExpiry} {
		for i, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if bad.MatchString(line) {
				t.Errorf("%s script line %d orders a string: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
}
