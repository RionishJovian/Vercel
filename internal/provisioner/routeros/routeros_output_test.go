package routeros

import (
	"context"
	"strings"
	"testing"
)

// Exact output from RouterOS 7.24.6 (CHR). Simulated routers never showed these layouts.
const (
	realScriptPrint = "0  ;;; mega-magic:broadband:v1\r\n" +
		"   name=\"mega-magic-broadband-expiry-v1\" owner=\"admin\" policy=read,write\r\n" +
		"   dont-require-permissions=yes run-count=0 source=\r\n" +
		"      :local now 1; :put $now;\r\n"
	realSchedulerPrint = "          ;;; mega-magic:broadband:v1                          \r\n" +
		"        name: mega-magic-broadband-expiry-v1                   \r\n" +
		"  start-time: startup                                          \r\n" +
		"    interval: 15s                                              \r\n" +
		"    on-event: /system script run mega-magic-broadband-expiry-v1\r\n" +
		"       owner: admin                                            \r\n" +
		"      policy: read                                             \r\n" +
		"              write                                            \r\n" +
		"   run-count: 0                                                \r\n"
	realDisabledSchedulerPrint = "Flags: X - DISABLED\r\n        name: zz-off    \r\n    interval: 1h        \r\n    on-event: :put 1    \r\n"
	realWrappedComment         = "               ;;; mega-magic:broadband:v1,                  \r\n" +
		"               ;;; 1793815908                                \r\n" +
		"             name: mm-bb-aaaaaaaaaaaa4aaa8aaa004047000001    \r\n" +
		"         only-one: yes                                       \r\n"
	realDeviceMode = "                 mode: advanced     \r\n     allowed-versions: 7.13+,6.49.8+\r\n              flagged: no           \r\n            scheduler: yes          \r\n                fetch: yes          \r\n              hotspot: yes          \r\n        attempt-count: 0            \r\n"
)

func TestParsesRealRouterOSOutput(t *testing.T) {
	script := parseGuardValueList(realScriptPrint)
	if script["comment"] != "mega-magic:broadband:v1" || script["name"] != "mega-magic-broadband-expiry-v1" || !strings.Contains(script["source"], ":local now 1") || script["dont-require-permissions"] != "yes" || script["disabled"] != "no" {
		t.Errorf("script with a comment on its first line: %q", script)
	}
	sched := parseGuardValueList(realSchedulerPrint)
	if sched["comment"] != "mega-magic:broadband:v1" || sched["disabled"] != "no" || !policyAllowsReadWrite(sched["policy"]) || parseUptimeSeconds(sched["interval"]) != 15 {
		t.Errorf("enabled scheduler: %q", sched)
	}
	if off := parseGuardValueList(realDisabledSchedulerPrint); off["disabled"] != "yes" {
		t.Errorf("a disabled record shows only the X flag: %q", off)
	}
	if wrapped := parseGuardValueList(realWrappedComment); wrapped["comment"] != "mega-magic:broadband:v1,1793815908" {
		t.Errorf("a long comment wraps across lines: %q", wrapped["comment"])
	}
	if f := parseDeviceMode(realDeviceMode); !f.Known || f.Mode != "advanced" || !f.Hotspot || !f.Scheduler {
		t.Errorf("device mode: %+v", f)
	}
	if up := parseUptimeSeconds("00:00:21"); up != 21 {
		t.Errorf("session uptime: %d", up)
	}
}

func TestSpeedProfileUsesNoComment(t *testing.T) {
	// RouterOS has no comment on Hotspot user profiles and refuses the whole command.
	var add string
	r := runnerFunc(func(_ context.Context, cmd string) (string, error) {
		if strings.Contains(cmd, "user profile add") {
			add = cmd
		}
		if strings.Contains(cmd, "count-only") {
			if add == "" {
				return "0", nil
			}
			return "1", nil
		}
		return "name: mm-speed-u5-d20\nrate-limit: 5M/20M\nshared-users: 1\n", nil
	})
	if _, e := ensureGuestSpeedProfile(context.Background(), r, 20, 5); e != nil {
		t.Fatal(e)
	}
	if add == "" || strings.Contains(add, "comment") {
		t.Fatalf("speed profile command: %q", add)
	}
}

func TestWaitingGuestIsLoggedInAfterTheirUserIsCreated(t *testing.T) {
	// RouterOS tries MAC login only when it first sees a host, so a guest already waiting stays locked out.
	var cmds []string
	retryMACLogin(context.Background(), runnerFunc(func(_ context.Context, cmd string) (string, error) {
		cmds = append(cmds, cmd)
		return "", nil
	}), "AA:BB:CC:DD:EE:FF")
	if len(cmds) != 1 || !strings.Contains(cmds[0], "/ip hotspot host remove [find mac-address=AA:BB:CC:DD:EE:FF]") || !strings.Contains(cmds[0], "/ip hotspot active find mac-address=AA:BB:CC:DD:EE:FF") {
		t.Fatalf("%q", cmds)
	}
}

func TestProtectionRulesArePlacedByRuleIDNotNumber(t *testing.T) {
	// Rule number 0 is a dynamic rule once Hotspot runs, and RouterOS refuses it as a position.
	cmd := networkWriteCommand(networkStep{Key: "filters", SelectorField: "comment", SelectorValue: "x", First: true, Values: map[string]string{"chain": "input"}}, false)
	if strings.Contains(cmd, "place-before=0") || !strings.Contains(cmd, "find where dynamic=no") {
		t.Fatal(cmd)
	}
}
