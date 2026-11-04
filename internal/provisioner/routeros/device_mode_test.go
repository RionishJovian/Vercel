package routeros

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

const advancedPrint = `mode: advanced
allowed-versions: 7.13+,6.49.8+
flagged: no
flagging-enabled: yes
scheduler: yes
socks: yes
fetch: yes
hotspot: yes
attempt-count: 0
`

func TestParseDeviceMode(t *testing.T) {
	for name, tc := range map[string]struct {
		out               string
		known, hot, sched bool
	}{
		"advanced, manual's example":         {advancedPrint, true, true, true},
		"home, flags printed":                {"mode: home\nscheduler: no\nhotspot: no\n", true, false, false},
		"home, flags absent":                 {"mode: home\n", true, false, false},
		"basic, flags absent":                {"mode: basic\n", true, false, true},
		"home with hotspot switched on":      {"mode: home\nscheduler: yes\nhotspot: yes\n", true, true, true},
		"advanced with hotspot switched off": {"mode: advanced\nhotspot: no\nscheduler: yes\n", true, false, true},
		"true and false spellings":           {"mode: home\nhotspot: true\nscheduler: false\n", true, true, false},
		"indented output":                    {"     mode: home\n  hotspot: no\n", true, false, false},
		"no mode":                            {"hotspot: no\n", false, false, false},
		"unknown mode":                       {"mode: something\n", false, false, false},
		"empty":                              {"", false, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := parseDeviceMode(tc.out)
			if got.Known != tc.known || got.Hotspot != tc.hot || got.Scheduler != tc.sched {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestRequireDeviceFeatures(t *testing.T) {
	mode := func(out string, err error) CommandRunner {
		return runnerFunc(func(_ context.Context, cmd string) (string, error) {
			if cmd != deviceModeCommand {
				t.Fatal("device-mode check ran something else:", cmd)
			}
			return out, err
		})
	}
	ctx := context.Background()
	if e := requireDeviceFeatures(ctx, mode(advancedPrint, nil), true, true); e != nil {
		t.Fatal(e)
	}
	if e := requireDeviceFeatures(ctx, mode("", errors.New("bad command name device-mode")), true, true); e != nil {
		t.Fatal("a router that cannot report device mode must not be blocked:", e)
	}
	e := requireDeviceFeatures(ctx, mode("mode: home\n", nil), true, true)
	if e == nil || !strings.Contains(e.Error(), "hotspot=yes scheduler=yes") || !strings.Contains(e.Error(), "(home)") || !strings.Contains(e.Error(), "reset or mode button") {
		t.Fatal(e)
	}
	e = requireDeviceFeatures(ctx, mode("mode: basic\n", nil), true, true)
	if e == nil || !strings.Contains(e.Error(), "hotspot=yes") || strings.Contains(e.Error(), "scheduler=yes") {
		t.Fatal("asked for more than the router needs:", e)
	}
	if e := requireDeviceFeatures(ctx, mode("mode: basic\n", nil), false, true); e != nil {
		t.Fatal("basic allows Scheduler:", e)
	}
}

func TestSetupsStopBeforeWritingWhenDeviceModeRefuses(t *testing.T) {
	writes := func(t *testing.T) CommandRunner {
		return runnerFunc(func(_ context.Context, cmd string) (string, error) {
			if cmd == deviceModeCommand {
				return "mode: home\n", nil
			}
			if strings.Contains(cmd, ":timestamp") { // the voucher guard reads the clock first
				return strconv.FormatInt(time.Now().Unix(), 10), nil
			}
			t.Fatal("ran past the device-mode check:", cmd)
			return "", nil
		})
	}
	if e := ensureBBGuard(context.Background(), writes(t)); e == nil || !strings.Contains(e.Error(), "device mode") {
		t.Fatal("PPPoE guard:", e)
	}
	if e := ensureVoucherLoginGuard(context.Background(), writes(t)); e == nil || !strings.Contains(e.Error(), "device mode") {
		t.Fatal("voucher guard:", e)
	}
	if e := gatewayPreflight(context.Background(), writes(t), GatewaySetup{Hotspot: true}, nil, nil); e == nil || !strings.Contains(e.Error(), "device mode") {
		t.Fatal("gateway setup:", e)
	}
}

// Finding 19: an interface name from configuration must not be able to add commands.
func TestApplierQuotesItsInterfaceName(t *testing.T) {
	var commands []string
	run := runnerFunc(func(_ context.Context, cmd string) (string, error) {
		commands = append(commands, cmd)
		return "", nil
	})
	a := NewApplier(run, `wg; /system reset-configuration`, 0, 0)
	_, _ = a.Snapshot(context.Background(), "biz", "peer", nil)
	if len(commands) < 3 {
		t.Fatal("snapshot ran too few commands:", commands)
	}
	for _, cmd := range commands {
		if strings.Contains(cmd, "wg; /system") && !strings.Contains(cmd, `"wg; /system reset-configuration"`) {
			t.Fatal("interface name reached the command unquoted:", cmd)
		}
	}
}
