package routeros

// Typed controls for a device behind the router. They change the router only: the
// app holds no login for the device itself. Every plan runs through the shared
// preview/apply path, so review, backup, restore timer and readback all apply.
import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const (
	deviceBlockPrefix = "mm-block-"
	poeCycleAction    = "poe_cycle"
	poeCycleDuration  = "5s"
)

var (
	deviceMACPattern = regexp.MustCompile(`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`)
	// RouterOS comments are not reliably stored outside printable ASCII.
	portLabelPattern = regexp.MustCompile(`^[ -~]{0,64}$`)
)

// IsDeviceControlKind reports the operations held by MM_DEVICE_CONTROLS_ENABLED.
func IsDeviceControlKind(kind string) bool {
	switch kind {
	case "device_block", "device_unblock", "port_label", "port_state", "poe_cycle":
		return true
	}
	return false
}

func deviceBlockComment(mac string) string {
	return deviceBlockPrefix + strings.ReplaceAll(mac, ":", "")
}

func exactValues(values map[string]string, names ...string) bool {
	if len(values) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := values[name]; !ok {
			return false
		}
	}
	return true
}

func buildDeviceControlPlan(ctx context.Context, r CommandRunner, request NetworkRequest) ([]networkStep, []string, error) {
	if request.ID != "" {
		return nil, nil, fmt.Errorf("unexpected network input")
	}
	switch request.Kind {
	case "device_block":
		return buildDeviceBlockPlan(ctx, r, request.Values)
	case "device_unblock":
		return buildDeviceUnblockPlan(ctx, r, request.Values)
	case "port_label":
		return buildPortLabelPlan(ctx, r, request.Values)
	case "port_state":
		return buildPortStatePlan(ctx, r, request.Values)
	case "poe_cycle":
		return buildPoECyclePlan(ctx, r, request.Values)
	}
	return nil, nil, fmt.Errorf("unsupported network operation")
}

func readObservedRows(ctx context.Context, r CommandRunner, key string) ([]map[string]string, error) {
	menu := observationMenus[key]
	out, err := r.Run(ctx, networkReadCommand(menu))
	if err != nil {
		return nil, err
	}
	objects, err := parseNetworkObjects(menu, out, key)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]string, 0, len(objects))
	for _, object := range objects {
		rows = append(rows, object.Values)
	}
	return rows, nil
}

// Blocking the upstream modem drops the return traffic of every client, so a MAC
// seen on an uplink, or one of the router's own, is refused. Unreadable sources refuse too.
func refuseProtectedMAC(ctx context.Context, r CommandRunner, mac string) error {
	own, err := readObservedRows(ctx, r, "interfaces")
	if err != nil {
		return fmt.Errorf("could not check this device safely; nothing was changed")
	}
	for _, row := range own {
		if strings.EqualFold(row["mac-address"], mac) {
			return fmt.Errorf("this address belongs to the router itself and cannot be blocked")
		}
	}
	uplinks, err := networkUplinkInterfaces(ctx, r)
	if err != nil {
		return err
	}
	seen := func(key, field string, required bool) error {
		rows, e := readObservedRows(ctx, r, key)
		if e != nil {
			if required {
				return fmt.Errorf("could not check this device safely; nothing was changed")
			}
			return nil
		}
		for _, row := range rows {
			if strings.EqualFold(row["mac-address"], mac) && uplinks[row[field]] {
				return fmt.Errorf("this device is on your internet uplink; blocking it would cut everyone off")
			}
		}
		return nil
	}
	if err = seen("arp", "interface", true); err != nil {
		return err
	}
	if err = seen("bridge_hosts", "on-interface", false); err != nil {
		return err
	}
	return seen("neighbors", "interface", false)
}

func buildDeviceBlockPlan(ctx context.Context, r CommandRunner, v map[string]string) ([]networkStep, []string, error) {
	if !exactValues(v, "mac") || !deviceMACPattern.MatchString(v["mac"]) || isOddHex(v["mac"][1]) {
		return nil, nil, fmt.Errorf("choose a device with a valid address")
	}
	mac, comment := v["mac"], deviceBlockComment(v["mac"])
	n, err := networkCount(ctx, r, networkMenus["filters"].path+" print count-only where comment="+routerOSQuoted(comment))
	if err != nil {
		return nil, nil, err
	}
	if n != 0 {
		return nil, nil, fmt.Errorf("this device is already blocked")
	}
	if err = refuseProtectedMAC(ctx, r, mac); err != nil {
		return nil, nil, err
	}
	static, err := networkCount(ctx, r, networkMenus["filters"].path+" print count-only where dynamic=no")
	if err != nil {
		return nil, nil, err
	}
	return []networkStep{deviceBlockStep(mac, static == 0)}, []string{
		"Blocks traffic this device sends through the router, including internet. It can still reach devices on the same switch or Wi-Fi network.",
		"A person who changes the device's MAC address gets past this block.",
	}, nil
}

// deviceBlockStep is the first static rule, above FastTrack: fast-tracked traffic skips later rules.
func deviceBlockStep(mac string, emptyTable bool) networkStep {
	comment := deviceBlockComment(mac)
	return networkStep{Key: "filters", SelectorField: "comment", SelectorValue: comment, First: true, EmptyTable: emptyTable, Label: "Block internet for " + mac,
		Values: map[string]string{"chain": "forward", "action": "drop", "src-mac-address": mac, "comment": comment}}
}

// isOddHex reports a multicast first octet, whose second hex digit is odd.
func isOddHex(c byte) bool {
	return strings.IndexByte("13579BDF", c) >= 0
}

func buildDeviceUnblockPlan(ctx context.Context, r CommandRunner, v map[string]string) ([]networkStep, []string, error) {
	if !exactValues(v, "mac") || !deviceMACPattern.MatchString(v["mac"]) {
		return nil, nil, fmt.Errorf("choose a device with a valid address")
	}
	comment := deviceBlockComment(v["mac"])
	out, err := r.Run(ctx, ":put ["+networkMenus["filters"].path+" find where comment="+routerOSQuoted(comment)+"]")
	id := strings.TrimSpace(out)
	if err != nil || !networkIDPattern.MatchString(id) {
		return nil, nil, fmt.Errorf("this device is not blocked by Mega Magic")
	}
	before, err := networkReadOne(ctx, r, "filters", id, map[string]string{"chain": "", "action": "", "src-mac-address": "", "comment": ""})
	if err != nil {
		return nil, nil, err
	}
	return []networkStep{{Key: "filters", ID: id, SelectorField: "comment", SelectorValue: comment, Remove: true, Label: "Remove the block for " + v["mac"], Before: before}}, nil, nil
}

// ethernetPort reads one physical port. Only Ethernet ports are offered: a bridge or
// tunnel can carry the router's whole network.
func ethernetPort(ctx context.Context, r CommandRunner, name string) (string, map[string]string, error) {
	if !gatewayInterfacePattern.MatchString(name) {
		return "", nil, fmt.Errorf("choose an actual router port")
	}
	out, err := r.Run(ctx, ":put [/interface find where name="+routerOSQuoted(name)+"]")
	id := strings.TrimSpace(out)
	if err != nil || !networkIDPattern.MatchString(id) {
		return "", nil, fmt.Errorf("this port was not found on the router")
	}
	values, err := networkReadOne(ctx, r, "iface_ports", id, map[string]string{"comment": "", "disabled": "", "type": ""})
	if err != nil {
		return "", nil, err
	}
	if values["type"] != "ether" {
		return "", nil, fmt.Errorf("only Ethernet ports can be changed here")
	}
	return id, values, nil
}

func buildPortLabelPlan(ctx context.Context, r CommandRunner, v map[string]string) ([]networkStep, []string, error) {
	if !exactValues(v, "interface", "comment") {
		return nil, nil, fmt.Errorf("choose a port and a label")
	}
	comment := strings.TrimSpace(v["comment"])
	if !portLabelPattern.MatchString(comment) {
		return nil, nil, fmt.Errorf("use up to 64 English letters, numbers and common symbols for a port label")
	}
	id, port, err := ethernetPort(ctx, r, v["interface"])
	if err != nil {
		return nil, nil, err
	}
	if port["comment"] == comment {
		return nil, nil, fmt.Errorf("this port already has that label")
	}
	return []networkStep{{Key: "iface_ports", ID: id, Label: "Label port " + v["interface"], Values: map[string]string{"comment": comment}, Before: map[string]string{"comment": port["comment"]}}}, nil, nil
}

func buildPortStatePlan(ctx context.Context, r CommandRunner, v map[string]string) ([]networkStep, []string, error) {
	if !exactValues(v, "interface", "disabled") || (v["disabled"] != "yes" && v["disabled"] != "no") {
		return nil, nil, fmt.Errorf("choose a port and whether to turn it on or off")
	}
	id, port, err := ethernetPort(ctx, r, v["interface"])
	if err != nil {
		return nil, nil, err
	}
	if networkBool(port["disabled"]) == v["disabled"] {
		return nil, nil, fmt.Errorf("this port is already in that state")
	}
	before := map[string]string{"disabled": networkBool(port["disabled"])}
	if v["disabled"] == "no" {
		return []networkStep{{Key: "iface_ports", ID: id, Label: "Turn on port " + v["interface"], Values: map[string]string{"disabled": "no"}, Before: before}}, nil, nil
	}
	uplinks, err := networkUplinkInterfaces(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	if uplinks[v["interface"]] {
		return nil, nil, fmt.Errorf("this port is your internet uplink; turning it off would disconnect the router from Mega Magic")
	}
	return []networkStep{{Key: "iface_ports", ID: id, Label: "Turn off port " + v["interface"], Values: map[string]string{"disabled": "yes"}, Before: before, Hold: true}},
		[]string{"Everything on this port loses its connection, including any switch or access point and the devices behind it. The router turns the port back on after 5 minutes unless you keep the change."}, nil
}

func buildPoECyclePlan(ctx context.Context, r CommandRunner, v map[string]string) ([]networkStep, []string, error) {
	if !exactValues(v, "interface") || !gatewayInterfacePattern.MatchString(v["interface"]) {
		return nil, nil, fmt.Errorf("choose a port")
	}
	name := v["interface"]
	out, err := r.Run(ctx, ":put ["+networkMenus["poe_ports"].path+" find where name="+routerOSQuoted(name)+"]")
	id := strings.TrimSpace(out)
	if err != nil || !networkIDPattern.MatchString(id) {
		return nil, nil, fmt.Errorf("this port cannot power a device")
	}
	poe, err := networkReadOne(ctx, r, "poe_ports", id, map[string]string{"poe-out": ""})
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasPrefix(poe["poe-out"], "auto-on") && !strings.HasPrefix(poe["poe-out"], "forced-on") {
		return nil, nil, fmt.Errorf("power output is off on this port")
	}
	uplinks, err := networkUplinkInterfaces(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	if uplinks[name] {
		return nil, nil, fmt.Errorf("this port is your internet uplink; restarting its power could disconnect the router from Mega Magic")
	}
	return []networkStep{{Key: "poe_ports", ID: id, Action: poeCycleAction, Label: "Restart power on port " + name + " for " + poeCycleDuration, Values: map[string]string{"poe-out": poe["poe-out"]}}},
		[]string{"The device on this port restarts, and anything connected through it loses its connection until it comes back."}, nil
}

// runDeviceAction performs a one-shot command that changes no saved setting, so it
// has nothing to back up or restore. Verified means the router took the command and
// kept the port's power setting. It does not mean the device has come back.
func runDeviceAction(ctx context.Context, r CommandRunner, step networkStep) (NetworkResult, error) {
	if step.Action != poeCycleAction {
		return NetworkResult{}, fmt.Errorf("unsupported network operation")
	}
	if _, err := r.Run(ctx, networkMenus[step.Key].path+" power-cycle "+step.ID+" duration="+poeCycleDuration); err != nil {
		return NetworkResult{}, fmt.Errorf("the router did not accept the power restart")
	}
	after, err := networkReadOne(ctx, r, step.Key, step.ID, map[string]string{"poe-out": ""})
	if err != nil || after["poe-out"] != step.Values["poe-out"] {
		return NetworkResult{}, fmt.Errorf("the port's power setting could not be confirmed after the restart")
	}
	return NetworkResult{Verified: true}, nil
}

// clearDeviceConnections ends a newly blocked device's tracked connections: fast-tracked
// packets skip the firewall until their entry is gone. A failure here leaves the block
// itself in force, so it is best effort.
func clearDeviceConnections(ctx context.Context, r CommandRunner, mac string) {
	addresses := map[string]bool{}
	for _, key := range []string{"arp", "discovery_leases"} {
		rows, err := readObservedRows(ctx, r, key)
		if err != nil {
			continue
		}
		for _, row := range rows {
			if strings.EqualFold(row["mac-address"], mac) && networkIPv4(row["address"]) {
				addresses[row["address"]] = true
			}
		}
	}
	for _, address := range networkSortedKeys(boolKeys(addresses)) {
		_, _ = r.Run(ctx, "/ip firewall connection remove [find where src-address~"+routerOSQuoted("^"+address+":")+"]")
	}
}

func boolKeys(m map[string]bool) map[string]string {
	out := make(map[string]string, len(m))
	for key := range m {
		out[key] = ""
	}
	return out
}
