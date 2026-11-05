package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const (
	testDeviceMAC = "02:00:00:00:00:40"
	testModemMAC  = "02:00:00:00:00:99"
	testOwnMAC    = "02:00:00:00:00:01"
)

// fakeRouter answers the fixed reads the plans make; the first matching fragment wins.
type fakeRouter struct {
	replies  [][2]string
	commands []string
}

func (f *fakeRouter) Run(_ context.Context, cmd string) (string, error) {
	f.commands = append(f.commands, cmd)
	for _, reply := range f.replies {
		if strings.Contains(cmd, reply[0]) {
			if reply[1] == "ERROR" {
				return "", fmt.Errorf("refused")
			}
			return reply[1], nil
		}
	}
	return "", nil
}

func (f *fakeRouter) wrote(fragment string) bool {
	for _, cmd := range f.commands {
		if strings.Contains(cmd, fragment) {
			return true
		}
	}
	return false
}

// A router on ether1 uplink, with a modem there and a device behind the LAN bridge.
func lanRouter() *fakeRouter {
	return &fakeRouter{replies: [][2]string{
		{"print count-only where comment=", "0"},
		{"print count-only where dynamic=no", "3"},
		{"[/ip dhcp-client find]", "id: *1\ninterface: ether1\ngateway: 192.168.1.1\nadd-default-route: yes\ndisabled: false\n"},
		{"[/interface pppoe-client find]", ""},
		{"[/ip address find]", "id: *2\naddress: 192.168.1.5/24\ninterface: ether1\ncomment: \ndisabled: false\ndynamic: true\n"},
		{"[/ip route find]", "id: *3\ndst-address: 0.0.0.0/0\ngateway: 192.168.1.1\ndistance: 1\ncheck-gateway: \nscope: 30\ntarget-scope: 10\nrouting-table: main\ncomment: \ndisabled: false\ndynamic: true\n"},
		{"[/interface find]", "id: *4\nname: ether1\ntype: ether\nmac-address: " + testOwnMAC + "\nrunning: true\ndisabled: false\n"},
		{"[/ip arp find]", "id: *5\naddress: 192.168.1.1\nmac-address: " + testModemMAC + "\ninterface: ether1\nid: *6\naddress: 192.168.88.40\nmac-address: " + testDeviceMAC + "\ninterface: bridge1\n"},
		{"[/interface bridge host find]", "id: *7\nmac-address: " + testDeviceMAC + "\non-interface: ether2\nbridge: bridge1\nlocal: false\n"},
		{"[/ip neighbor find]", ""},
		{"/interface find where name=\"ether1\"", "*A\n"},
		{"/interface find where name=\"ether2\"", "*B\n"},
		{"/interface find where name=\"bridge1\"", "*C\n"},
		{"find where .id=*A", "id: *A\ncomment: \ndisabled: false\ntype: ether\n"},
		{"find where .id=*B", "id: *B\ncomment: Floor 2\ndisabled: false\ntype: ether\n"},
		{"find where .id=*C", "id: *C\ncomment: \ndisabled: false\ntype: bridge\n"},
		{"poe find where name=\"ether2\"", "*D\n"},
		{"poe find where name=\"ether1\"", "*E\n"},
		{"poe find where name=\"ether9\"", ""},
		{"find where .id=*D", "id: *D\npoe-out: auto-on\n"},
		{"find where .id=*E", "id: *E\npoe-out: auto-on\n"},
	}}
}

func plan(t *testing.T, r CommandRunner, kind string, values map[string]string) ([]networkStep, []string, error) {
	t.Helper()
	return buildNetworkPlan(context.Background(), r, NetworkRequest{Kind: kind, Values: values})
}

func TestDeviceBlockGoesAboveFastTrackAndOnlyPlansARule(t *testing.T) {
	r := lanRouter()
	steps, warnings, err := plan(t, r, "device_block", map[string]string{"mac": testDeviceMAC})
	if err != nil {
		t.Fatal(err)
	}
	s := steps[0]
	if len(steps) != 1 || !s.First || s.Key != "filters" || s.Values["chain"] != "forward" || s.Values["action"] != "drop" || s.Values["src-mac-address"] != testDeviceMAC || s.Values["comment"] != "mm-block-020000000040" {
		t.Fatalf("unexpected plan: %+v", steps)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "same switch") {
		t.Fatalf("the limit of a block must be stated: %v", warnings)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, " add ") || strings.Contains(cmd, " remove ") {
			t.Fatalf("planning wrote to the router: %s", cmd)
		}
	}
	if cmd := networkWriteCommand(s, false); !strings.Contains(cmd, "place-before=[:pick $mmStatic 0]") {
		t.Fatalf("rule must be placed first: %s", cmd)
	}
}

func TestDeviceBlockRefusals(t *testing.T) {
	cases := map[string]struct{ mac, want string }{
		"upstream modem": {testModemMAC, "uplink"},
		"router itself":  {testOwnMAC, "router itself"},
		"multicast":      {"01:00:5E:00:00:01", "valid address"},
		"lower case":     {"02:aa:00:00:00:01", "valid address"},
		"not a mac":      {`02:00:00:00:00:40" disabled=yes`, "valid address"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := plan(t, lanRouter(), "device_block", map[string]string{"mac": c.mac}); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("already blocked", func(t *testing.T) {
		r := lanRouter()
		r.replies = append([][2]string{{"print count-only where comment=", "1"}}, r.replies...)
		if _, _, err := plan(t, r, "device_block", map[string]string{"mac": testDeviceMAC}); err == nil || !strings.Contains(err.Error(), "already blocked") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unreadable ARP fails closed", func(t *testing.T) {
		r := lanRouter()
		r.replies = append([][2]string{{"[/ip arp find]", "ERROR"}}, r.replies...)
		if _, _, err := plan(t, r, "device_block", map[string]string{"mac": testDeviceMAC}); err == nil {
			t.Fatal("a block must not be offered when the uplink side cannot be checked")
		}
	})
	t.Run("extra values", func(t *testing.T) {
		if _, _, err := plan(t, lanRouter(), "device_block", map[string]string{"mac": testDeviceMAC, "chain": "input"}); err == nil {
			t.Fatal("the browser must not choose rule fields")
		}
	})
}

func TestDeviceUnblockOnlyRemovesOwnRule(t *testing.T) {
	r := lanRouter()
	if _, _, err := plan(t, r, "device_unblock", map[string]string{"mac": testDeviceMAC}); err == nil {
		t.Fatal("no rule means nothing to remove")
	}
	r = lanRouter()
	r.replies = append([][2]string{
		{`find where comment="mm-block-020000000040"]`, "*F\n"},
		{"find where .id=*F", "id: *F\naction: drop\nchain: forward\ncomment: mm-block-020000000040\nsrc-mac-address: " + testDeviceMAC + "\n"},
	}, r.replies...)
	steps, _, err := plan(t, r, "device_unblock", map[string]string{"mac": testDeviceMAC})
	if err != nil || len(steps) != 1 || !steps[0].Remove || steps[0].ID != "*F" {
		t.Fatalf("%+v %v", steps, err)
	}
}

func TestPortStateProtectsUplinkAndHoldsForConfirmation(t *testing.T) {
	if _, _, err := plan(t, lanRouter(), "port_state", map[string]string{"interface": "ether1", "disabled": "yes"}); err == nil || !strings.Contains(err.Error(), "uplink") {
		t.Fatalf("uplink must be refused: %v", err)
	}
	steps, warnings, err := plan(t, lanRouter(), "port_state", map[string]string{"interface": "ether2", "disabled": "yes"})
	if err != nil || !steps[0].Hold || steps[0].Values["disabled"] != "yes" || steps[0].Before["disabled"] != "no" || len(warnings) == 0 {
		t.Fatalf("turning a port off needs the restore timer: %+v %v %v", steps, warnings, err)
	}
	if _, _, err = plan(t, lanRouter(), "port_state", map[string]string{"interface": "ether2", "disabled": "no"}); err == nil {
		t.Fatal("a port already on must be reported, not rewritten")
	}
	if _, _, err = plan(t, lanRouter(), "port_state", map[string]string{"interface": "bridge1", "disabled": "yes"}); err == nil || !strings.Contains(err.Error(), "Ethernet") {
		t.Fatalf("a bridge carries the whole network: %v", err)
	}
	if _, _, err = plan(t, lanRouter(), "port_state", map[string]string{"interface": `ether2" disabled=no`, "disabled": "yes"}); err == nil {
		t.Fatal("a crafted port name must be refused")
	}
}

func TestPortLabel(t *testing.T) {
	steps, _, err := plan(t, lanRouter(), "port_label", map[string]string{"interface": "ether2", "comment": " Lobby AP "})
	if err != nil || steps[0].Values["comment"] != "Lobby AP" || steps[0].Before["comment"] != "Floor 2" || steps[0].Hold {
		t.Fatalf("%+v %v", steps, err)
	}
	if steps, _, err = plan(t, lanRouter(), "port_label", map[string]string{"interface": "ether2", "comment": ""}); err != nil || steps[0].Values["comment"] != "" {
		t.Fatalf("clearing a label must work: %+v %v", steps, err)
	}
	for _, bad := range []string{"ဧည့်ခန်း", strings.Repeat("x", 65), "two\nlines"} {
		if _, _, err = plan(t, lanRouter(), "port_label", map[string]string{"interface": "ether2", "comment": bad}); err == nil {
			t.Fatalf("label %q must be refused", bad)
		}
	}
	if _, _, err = plan(t, lanRouter(), "port_label", map[string]string{"interface": "ether2", "comment": "Floor 2"}); err == nil {
		t.Fatal("an unchanged label is not a change")
	}
}

func TestPoECyclePlanAndCommand(t *testing.T) {
	steps, _, err := plan(t, lanRouter(), "poe_cycle", map[string]string{"interface": "ether2"})
	if err != nil || steps[0].Action != poeCycleAction {
		t.Fatalf("%+v %v", steps, err)
	}
	for name, port := range map[string]string{"uplink": "ether1", "no PoE": "ether9"} {
		if _, _, err = plan(t, lanRouter(), "poe_cycle", map[string]string{"interface": port}); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	off := lanRouter()
	off.replies = append([][2]string{{"find where .id=*D", "id: *D\npoe-out: off\n"}}, off.replies...)
	if _, _, err = plan(t, off, "poe_cycle", map[string]string{"interface": "ether2"}); err == nil {
		t.Fatal("a port with no power output has nothing to restart")
	}

	r := lanRouter()
	result, err := runDeviceAction(context.Background(), r, steps[0])
	if err != nil || !result.Verified || !r.wrote("/interface ethernet poe power-cycle *D duration=5s") {
		t.Fatalf("%+v %v %v", result, err, r.commands)
	}
	changed := lanRouter()
	changed.replies = append([][2]string{{"find where .id=*D", "id: *D\npoe-out: off\n"}}, changed.replies...)
	if result, err = runDeviceAction(context.Background(), changed, steps[0]); err == nil || result.Verified {
		t.Fatal("a changed power setting must not be reported as verified")
	}
}

func TestObservationsExposeOnlyOwnBlockRules(t *testing.T) {
	objects := []NetworkObject{{Values: map[string]string{"comment": "mm-block-020000000040"}}, {Values: map[string]string{"comment": "customer rule"}}, {Values: map[string]string{"comment": ""}}}
	if own := ownDeviceBlocks(objects); len(own) != 1 || own[0].Values["comment"] != "mm-block-020000000040" {
		t.Fatalf("%+v", own)
	}
}

func TestParseEmptyLastValue(t *testing.T) {
	objects, err := parseNetworkObjects(networkMenu{"/x", []string{"comment"}}, "id: *1\ncomment:", "x")
	if err != nil || objects[0].Values["comment"] != "" {
		t.Fatalf("%+v %v", objects, err)
	}
	if _, err = parseNetworkObjects(networkMenu{"/x", []string{"comment"}}, "id: *1\nnonsense", "x"); err == nil {
		t.Fatal("unrecognized output must still fail")
	}
}

func TestObservationsKeepPortsWhenLabelsOrPoECannotBeRead(t *testing.T) {
	r := commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "get $id comment") && strings.Contains(cmd, "[/interface find"):
			return "", fmt.Errorf("no such property")
		case strings.Contains(cmd, "/interface ethernet poe"):
			return "", fmt.Errorf("bad command name")
		case strings.Contains(cmd, "[/interface find"):
			return "id: *1\nname: ether1\ntype: ether\nmac-address: 02:00:00:00:00:01\nrunning: true\ndisabled: false\n", nil
		}
		return "", nil
	})
	result, err := readObservations(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available["interfaces"] || len(result.Objects["interfaces"]) != 1 {
		t.Fatalf("a failed label read must not lose the port list: %+v", result.Available)
	}
	if result.Available["interface_labels"] || result.Available["poe_ports"] {
		t.Fatalf("the failed reads must be reported: %+v", result.Available)
	}
}
