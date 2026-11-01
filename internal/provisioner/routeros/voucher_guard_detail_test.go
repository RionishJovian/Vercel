package routeros

import "testing"

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
	out := "1  name=\"mega-magic-voucher-v1\" shared-users=1 on-login=\"/system script run mega-magic-voucher-guard-v1-login\" on-logout=\"\"\n"
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
