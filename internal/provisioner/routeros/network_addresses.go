package routeros

// Free-form IP address and VLAN operations. They reuse the guarded network
// path (preview, backup, router-side restoration timer, per-step readback and
// rollback) and add protections that fail closed: the Hub tunnel, the internet
// uplink, ports that serve customers and dynamic addresses are never touched.
import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// The Magic Hub management range must never be reused or overlapped.
var networkHubRange = func() *net.IPNet { _, n, _ := net.ParseCIDR("10.77.0.0/24"); return n }()

var networkTypePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// networkLANTypes can carry an address a customer network is reached through.
var networkLANTypes = " ether bridge bond vlan "

// networkVLANParents can carry a tagged VLAN. VLANs on VLANs and on tunnels are refused.
var networkVLANParents = " ether bridge bond "

func networkInterfaceType(ctx context.Context, r CommandRunner, name string) (string, error) {
	if !gatewayInterfacePattern.MatchString(name) {
		return "", fmt.Errorf("choose an actual router interface")
	}
	out, err := r.Run(ctx, `:put [/interface get [find where name=`+routerOSQuoted(name)+`] type]`)
	kind := strings.TrimSpace(out)
	if err != nil || !networkTypePattern.MatchString(kind) {
		return "", fmt.Errorf("could not read the interface type safely")
	}
	return kind, nil
}

// networkUplinkInterfaces returns every interface that carries internet: DHCP
// and PPPoE clients and the interface a default route leaves through. It fails
// closed: if any source cannot be read, nothing is allowed.
func networkUplinkInterfaces(ctx context.Context, r CommandRunner) (map[string]bool, error) {
	up := map[string]bool{}
	clients, err := readNetworkObjects(ctx, r, "dhcp_clients")
	if err != nil {
		return nil, fmt.Errorf("could not tell which port is your internet uplink; nothing was changed")
	}
	for _, c := range clients {
		up[c.Values["interface"]] = true
	}
	ppp, err := readNetworkObjects(ctx, r, "pppoe_clients")
	if err != nil {
		return nil, fmt.Errorf("could not tell which port is your internet uplink; nothing was changed")
	}
	for _, c := range ppp {
		up[c.Values["name"]], up[c.Values["interface"]] = true, true
	}
	addresses, err := readNetworkObjects(ctx, r, "addresses")
	if err != nil {
		return nil, fmt.Errorf("could not tell which port is your internet uplink; nothing was changed")
	}
	routes, err := readNetworkObjects(ctx, r, "routes")
	if err != nil {
		return nil, fmt.Errorf("could not tell which port is your internet uplink; nothing was changed")
	}
	for _, route := range routes {
		if route.Values["dst-address"] != "0.0.0.0/0" {
			continue
		}
		for _, gateway := range strings.FieldsFunc(route.Values["gateway"], func(c rune) bool { return c == ',' || c == ';' }) {
			gateway = strings.TrimSpace(gateway)
			host, iface, scoped := strings.Cut(gateway, "%")
			if scoped && iface != "" {
				up[iface] = true
			}
			if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
				for _, a := range addresses {
					if _, n, e := net.ParseCIDR(a.Values["address"]); e == nil && n.Contains(ip) {
						up[a.Values["interface"]] = true
					}
				}
			} else if host != "" && !scoped {
				up[host] = true // a route that leaves directly through a named interface
			}
		}
	}
	return up, nil
}

// networkInterfaceServes reports whether customers depend on the interface.
func networkInterfaceServes(ctx context.Context, r CommandRunner, name string) (bool, error) {
	for _, menu := range []string{"/ip dhcp-server", "/ip hotspot", "/interface pppoe-server server"} {
		n, err := networkCount(ctx, r, menu+" print count-only where interface="+routerOSQuoted(name))
		if err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

func lastIPv4(n *net.IPNet) net.IP {
	ip, mask := n.IP.To4(), n.Mask
	out := make(net.IP, 4)
	for i := range out {
		out[i] = ip[i] | ^mask[i]
	}
	return out
}

// parseLANAddress accepts a private IPv4 host address with a /8 to /30 prefix.
func parseLANAddress(value string) (net.IP, *net.IPNet, error) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(value))
	if err != nil || ip.To4() == nil {
		return nil, nil, fmt.Errorf("enter an IPv4 address with its prefix, for example 192.168.50.1/24")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones < 8 || ones > 30 {
		return nil, nil, fmt.Errorf("the prefix must be between /8 and /30")
	}
	ip = ip.To4()
	if ip.Equal(network.IP) || ip.Equal(lastIPv4(network)) {
		return nil, nil, fmt.Errorf("that is the network or broadcast address; choose a usable host address")
	}
	if !network.IP.IsPrivate() || !lastIPv4(network).IsPrivate() || !ip.IsPrivate() {
		return nil, nil, fmt.Errorf("only private networks (10.x.x.x, 172.16-31.x.x, 192.168.x.x) can be set here")
	}
	if network.Contains(networkHubRange.IP) || networkHubRange.Contains(network.IP) {
		return nil, nil, fmt.Errorf("10.77.0.0/24 is reserved for Magic Hub management")
	}
	return ip, network, nil
}

// networkReservedForAddress lists every network already in use on the router
// except the record being edited. Dynamic connected routes are ignored because
// they only mirror the addresses already counted.
func networkReservedForAddress(ctx context.Context, r CommandRunner, excludeID string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	addresses, err := readNetworkObjects(ctx, r, "addresses")
	if err != nil {
		return nil, fmt.Errorf("could not read existing addresses; nothing was changed")
	}
	for _, a := range addresses {
		if a.ID == excludeID {
			continue
		}
		_, n, e := net.ParseCIDR(a.Values["address"])
		if e != nil || n.IP.To4() == nil {
			return nil, fmt.Errorf("could not parse an existing router address; nothing was changed")
		}
		out = append(out, n)
	}
	routes, err := readNetworkObjects(ctx, r, "routes")
	if err != nil {
		return nil, fmt.Errorf("could not read existing routes; nothing was changed")
	}
	for _, route := range routes {
		if networkBool(route.Values["dynamic"]) != "no" || route.Values["dst-address"] == "0.0.0.0/0" {
			continue
		}
		if _, n, e := net.ParseCIDR(route.Values["dst-address"]); e == nil && n.IP.To4() != nil {
			out = append(out, n)
		}
	}
	return out, nil
}

func networkAddStep(ctx context.Context, r CommandRunner, key, selector, value, label string, values map[string]string) (networkStep, error) {
	n, err := networkCount(ctx, r, networkMenus[key].path+" print count-only where "+selector+"="+routerOSQuoted(value))
	if err != nil {
		return networkStep{}, err
	}
	if n != 0 {
		return networkStep{}, fmt.Errorf("%s already exists; select its record to edit instead", value)
	}
	return networkStep{Key: key, SelectorField: selector, SelectorValue: value, Label: label, Values: values}, nil
}

// networkEditStep returns nil when the record already has the requested values.
func networkEditStep(ctx context.Context, r CommandRunner, key, id, label string, values map[string]string) (*networkStep, error) {
	before, err := networkReadOne(ctx, r, key, id, values)
	if err != nil {
		return nil, err
	}
	for field, value := range values {
		if !networkEqual(field, before[field], value) {
			return &networkStep{Key: key, ID: id, Label: label, Values: values, Before: before}, nil
		}
	}
	return nil, nil
}

func networkEnabledState(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", "no":
		return "no", nil
	case "yes":
		return "yes", nil
	}
	return "", fmt.Errorf("enabled state must be yes or no")
}

func buildIPAddressPlan(ctx context.Context, r CommandRunner, request NetworkRequest) ([]networkStep, []string, error) {
	v := request.Values
	warnings := []string{"Changes apply to the selected router only. If the router restarts or the connection drops, a router-side timer restores the previous settings within three minutes.",
		"An IP address alone does not give customers DHCP or internet. Use Create customer network for a complete LAN."}
	disabled := ""
	var err error
	if _, given := v["disabled"]; given || request.ID == "" {
		if disabled, err = networkEnabledState(v["disabled"]); err != nil {
			return nil, nil, err
		}
	}
	up, err := networkUplinkInterfaces(ctx, r)
	if err != nil {
		return nil, nil, err
	}

	if request.ID == "" {
		iface := v["interface"]
		kind, err := networkInterfaceType(ctx, r, iface)
		if err != nil {
			return nil, nil, err
		}
		if err = networkRequireInterface(ctx, r, iface, false); err != nil {
			return nil, nil, err
		}
		if !strings.Contains(networkLANTypes, " "+kind+" ") || strings.HasPrefix(iface, "mmh-") {
			return nil, nil, fmt.Errorf("choose a customer port or bridge, not a tunnel, loopback or internet connection")
		}
		if up[iface] {
			return nil, nil, fmt.Errorf("%s carries your internet uplink; changing its addresses could disconnect the router", iface)
		}
		if kind != "bridge" {
			if n, e := networkCount(ctx, r, "/interface bridge port print count-only where interface="+routerOSQuoted(iface)); e != nil || n != 0 {
				return nil, nil, fmt.Errorf("%s belongs to a bridge; give the address to the bridge itself", iface)
			}
		}
		_, network, err := parseLANAddress(v["address"])
		if err != nil {
			return nil, nil, err
		}
		reserved, err := networkReservedForAddress(ctx, r, "")
		if err != nil {
			return nil, nil, err
		}
		if gatewayNetworkOverlaps(network, reserved) {
			return nil, nil, fmt.Errorf("that network overlaps an address or route already on the router")
		}
		serves, err := networkInterfaceServes(ctx, r, iface)
		if err != nil {
			return nil, nil, err
		}
		if serves {
			warnings = append(warnings, iface+" already serves customers (DHCP, Hotspot or PPPoE). This extra address does not change that service.")
		}
		cidr := strings.TrimSpace(v["address"])
		step, err := networkAddStep(ctx, r, "addresses", "address", cidr, "Add IP address "+cidr+" to "+iface,
			map[string]string{"address": cidr, "interface": iface, "comment": networkOwnedMark, "disabled": disabled})
		if err != nil {
			return nil, nil, err
		}
		return []networkStep{step}, warnings, nil
	}

	before, err := networkReadOne(ctx, r, "addresses", request.ID, map[string]string{"address": "", "interface": "", "dynamic": ""})
	if err != nil {
		return nil, nil, err
	}
	iface := before["interface"]
	if networkBool(before["dynamic"]) != "no" {
		return nil, nil, fmt.Errorf("this address is assigned automatically (DHCP or a service) and cannot be edited here")
	}
	if v["interface"] != "" && v["interface"] != iface {
		return nil, nil, fmt.Errorf("an address cannot be moved to another port here; add a new address instead")
	}
	kind, err := networkInterfaceType(ctx, r, iface)
	if err != nil {
		return nil, nil, err
	}
	if !strings.Contains(networkLANTypes, " "+kind+" ") || strings.HasPrefix(iface, "mmh-") {
		return nil, nil, fmt.Errorf("addresses on %s (%s) are protected and cannot be edited here", iface, kind)
	}
	if up[iface] {
		return nil, nil, fmt.Errorf("%s carries your internet uplink; its addresses are protected", iface)
	}
	serves, err := networkInterfaceServes(ctx, r, iface)
	if err != nil {
		return nil, nil, err
	}
	if serves {
		return nil, nil, fmt.Errorf("%s serves customers (DHCP, Hotspot or PPPoE); changing or disabling its address would strand them. Create a new customer network instead", iface)
	}
	values := map[string]string{}
	if cidr := strings.TrimSpace(v["address"]); cidr != "" {
		_, network, err := parseLANAddress(cidr)
		if err != nil {
			return nil, nil, err
		}
		reserved, err := networkReservedForAddress(ctx, r, request.ID)
		if err != nil {
			return nil, nil, err
		}
		if gatewayNetworkOverlaps(network, reserved) {
			return nil, nil, fmt.Errorf("that network overlaps an address or route already on the router")
		}
		values["address"] = cidr
		warnings = append(warnings, "Devices that use the old address as their gateway lose connection until they are updated. If you reach this router from your LAN at the old address, use the new one afterwards.")
	}
	if disabled != "" {
		values["disabled"] = disabled
	}
	if len(values) == 0 {
		return nil, nil, fmt.Errorf("no settings supplied")
	}
	step, err := networkEditStep(ctx, r, "addresses", request.ID, "Update IP address on "+iface, values)
	if err != nil {
		return nil, nil, err
	}
	if step == nil {
		return nil, nil, fmt.Errorf("selected settings are already applied")
	}
	return []networkStep{*step}, warnings, nil
}

// networkVLANUsedBy reports what depends on a VLAN interface; an empty result
// means it is unused and may be reshaped freely.
func networkVLANUsedBy(ctx context.Context, r CommandRunner, name string) ([]string, error) {
	var used []string
	for _, probe := range []struct{ label, menu string }{
		{"an IP address", "/ip address"}, {"a DHCP server", "/ip dhcp-server"}, {"a Hotspot", "/ip hotspot"},
		{"a bridge", "/interface bridge port"}, {"a PPPoE service", "/interface pppoe-server server"},
		{"an internet DHCP client", "/ip dhcp-client"}, {"a PPPoE internet uplink", "/interface pppoe-client"},
	} {
		n, err := networkCount(ctx, r, probe.menu+" print count-only where interface="+routerOSQuoted(name))
		if err != nil {
			return nil, err
		}
		if n > 0 {
			used = append(used, probe.label)
		}
	}
	return used, nil
}

func buildVLANPlan(ctx context.Context, r CommandRunner, request NetworkRequest) ([]networkStep, []string, error) {
	v := request.Values
	warnings := []string{"Changes apply to the selected router only. If the connection drops, a router-side timer restores the previous settings within three minutes.",
		"A VLAN only creates a tagged interface. Your switch or access point must carry the same VLAN ID on the connecting port, and the VLAN needs an address or a customer network to be useful."}
	var id int
	var err error
	parseID := func(value string) (int, error) {
		n, e := strconv.Atoi(strings.TrimSpace(value))
		if e != nil || n < 1 || n > 4094 {
			return 0, fmt.Errorf("the VLAN ID must be a number from 1 to 4094")
		}
		return n, nil
	}
	vlans, err := readNetworkObjects(ctx, r, "vlans")
	if err != nil {
		return nil, nil, fmt.Errorf("could not read existing VLANs; nothing was changed")
	}
	duplicate := func(parent string, vid int, exceptID string) bool {
		for _, o := range vlans {
			if o.ID != exceptID && o.Values["interface"] == parent && o.Values["vlan-id"] == strconv.Itoa(vid) {
				return true
			}
		}
		return false
	}
	checkParent := func(parent string) error {
		kind, e := networkInterfaceType(ctx, r, parent)
		if e != nil {
			return e
		}
		if e = networkRequireInterface(ctx, r, parent, false); e != nil {
			return e
		}
		if !strings.Contains(networkVLANParents, " "+kind+" ") || strings.HasPrefix(parent, "mmh-") {
			return fmt.Errorf("a VLAN can be created on a port, bridge or bond only, not on %s (%s)", parent, kind)
		}
		return nil
	}

	if request.ID == "" {
		name, parent := v["name"], v["interface"]
		if !gatewayNamePattern.MatchString(name) {
			return nil, nil, fmt.Errorf("use a 1-24 character VLAN name with letters, numbers, dash or underscore")
		}
		if id, err = parseID(v["vlan-id"]); err != nil {
			return nil, nil, err
		}
		disabled, err := networkEnabledState(v["disabled"])
		if err != nil {
			return nil, nil, err
		}
		if err = checkParent(parent); err != nil {
			return nil, nil, err
		}
		if n, e := networkCount(ctx, r, "/interface print count-only where name="+routerOSQuoted(name)); e != nil || n != 0 {
			return nil, nil, fmt.Errorf("an interface named %s already exists; choose another name", name)
		}
		if duplicate(parent, id, "") {
			return nil, nil, fmt.Errorf("VLAN %d already exists on %s", id, parent)
		}
		step, err := networkAddStep(ctx, r, "vlans", "name", name, "Create VLAN "+strconv.Itoa(id)+" ("+name+") on "+parent,
			map[string]string{"name": name, "vlan-id": strconv.Itoa(id), "interface": parent, "comment": networkOwnedMark, "disabled": disabled})
		if err != nil {
			return nil, nil, err
		}
		return []networkStep{step}, warnings, nil
	}

	before, err := networkReadOne(ctx, r, "vlans", request.ID, map[string]string{"name": "", "interface": "", "vlan-id": ""})
	if err != nil {
		return nil, nil, err
	}
	name := before["name"]
	if v["name"] != "" && v["name"] != name {
		return nil, nil, fmt.Errorf("a VLAN cannot be renamed here; create a new one instead")
	}
	up, err := networkUplinkInterfaces(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	if up[name] {
		return nil, nil, fmt.Errorf("%s carries your internet uplink and is protected", name)
	}
	used, err := networkVLANUsedBy(ctx, r, name)
	if err != nil {
		return nil, nil, err
	}
	values := map[string]string{}
	if state, given := v["disabled"]; given {
		disabled, e := networkEnabledState(state)
		if e != nil {
			return nil, nil, e
		}
		values["disabled"] = disabled
		if disabled == "yes" && len(used) > 0 {
			warnings = append(warnings, name+" is in use by "+strings.Join(used, ", ")+". Disabling it stops those services.")
		}
	}
	reshaped := (v["vlan-id"] != "" && v["vlan-id"] != before["vlan-id"]) || (v["interface"] != "" && v["interface"] != before["interface"])
	hold := false
	if reshaped {
		if len(used) > 0 {
			// Its addresses and services follow the VLAN, so nothing is removed, but
			// everything on it is cut off unless the switch or access point carries the
			// new ID and port. A person must test a real client and keep the change.
			hold = true
			warnings = append(warnings, name+" is in use by "+strings.Join(used, ", ")+". Changing its ID or port cuts off everything on it unless your switch or access point carries the new VLAN ID on the connecting port. After applying, test a real client: the change reverts by itself in 5 minutes unless you press Keep.")
		}
		parent, vid := before["interface"], before["vlan-id"]
		if v["interface"] != "" {
			parent = v["interface"]
		}
		newID, e := parseID(firstNonEmpty(v["vlan-id"], vid))
		if e != nil {
			return nil, nil, e
		}
		if e = checkParent(parent); e != nil {
			return nil, nil, e
		}
		if duplicate(parent, newID, request.ID) {
			return nil, nil, fmt.Errorf("VLAN %d already exists on %s", newID, parent)
		}
		values["interface"], values["vlan-id"] = parent, strconv.Itoa(newID)
	}
	if len(values) == 0 {
		return nil, nil, fmt.Errorf("no settings supplied")
	}
	step, err := networkEditStep(ctx, r, "vlans", request.ID, "Update VLAN "+name, values)
	if err != nil {
		return nil, nil, err
	}
	if step == nil {
		return nil, nil, fmt.Errorf("selected settings are already applied")
	}
	step.Hold = hold
	return []networkStep{*step}, warnings, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// networkOwnedMark is the comment the app puts on addresses and VLANs it creates.
// Only records carrying it can be deleted here: anything else was made by a person
// or another tool and is removed where it was made.
const networkOwnedMark = "mega-magic"

// buildRemovePlan deletes one address or VLAN that this app created. It applies
// the same protections as editing, and a VLAN must also be unused. The router
// keeps a restore timer that puts the record back, and a full copy of its fields
// is taken first.
func buildRemovePlan(ctx context.Context, r CommandRunner, request NetworkRequest) ([]networkStep, []string, error) {
	if len(request.Values) != 0 || !networkIDPattern.MatchString(request.ID) {
		return nil, nil, fmt.Errorf("choose the record to delete")
	}
	warnings := []string{"Deleting cannot be undone from the app after you confirm, but the full record is copied first and the router puts it back by itself within three minutes if anything goes wrong."}
	up, err := networkUplinkInterfaces(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	if request.Kind == "remove_ip_address" {
		before, err := networkReadOne(ctx, r, "addresses", request.ID, map[string]string{"address": "", "interface": "", "comment": "", "disabled": "", "dynamic": ""})
		if err != nil {
			return nil, nil, err
		}
		if before["comment"] != networkOwnedMark {
			return nil, nil, fmt.Errorf("this address was not created by Mega Magic, so it is not deleted here; remove it where it was created")
		}
		if networkBool(before["dynamic"]) != "no" {
			return nil, nil, fmt.Errorf("this address is assigned automatically and cannot be deleted here")
		}
		iface := before["interface"]
		kind, err := networkInterfaceType(ctx, r, iface)
		if err != nil {
			return nil, nil, err
		}
		if !strings.Contains(networkLANTypes, " "+kind+" ") || strings.HasPrefix(iface, "mmh-") {
			return nil, nil, fmt.Errorf("addresses on %s (%s) are protected and cannot be deleted here", iface, kind)
		}
		if up[iface] {
			return nil, nil, fmt.Errorf("%s carries your internet uplink; its addresses are protected", iface)
		}
		serves, err := networkInterfaceServes(ctx, r, iface)
		if err != nil {
			return nil, nil, err
		}
		if serves {
			return nil, nil, fmt.Errorf("%s serves customers (DHCP, Hotspot or PPPoE); deleting its address would strand them", iface)
		}
		keep := map[string]string{"address": before["address"], "interface": iface, "comment": networkOwnedMark, "disabled": before["disabled"]}
		return []networkStep{{Key: "addresses", ID: request.ID, Remove: true, SelectorField: "address", SelectorValue: before["address"], Label: "Delete IP address " + before["address"] + " on " + iface, Before: keep}}, warnings, nil
	}
	before, err := networkReadOne(ctx, r, "vlans", request.ID, map[string]string{"name": "", "interface": "", "vlan-id": "", "comment": "", "disabled": ""})
	if err != nil {
		return nil, nil, err
	}
	if before["comment"] != networkOwnedMark {
		return nil, nil, fmt.Errorf("this VLAN was not created by Mega Magic, so it is not deleted here; remove it where it was created")
	}
	name := before["name"]
	if up[name] {
		return nil, nil, fmt.Errorf("%s carries your internet uplink and is protected", name)
	}
	used, err := networkVLANUsedBy(ctx, r, name)
	if err != nil {
		return nil, nil, err
	}
	if len(used) > 0 {
		return nil, nil, fmt.Errorf("%s is in use by %s; remove those first", name, strings.Join(used, ", "))
	}
	keep := map[string]string{"name": name, "interface": before["interface"], "vlan-id": before["vlan-id"], "comment": networkOwnedMark, "disabled": before["disabled"]}
	return []networkStep{{Key: "vlans", ID: request.ID, Remove: true, SelectorField: "name", SelectorValue: name, Label: "Delete VLAN " + name + " (ID " + before["vlan-id"] + ")", Before: keep}}, warnings, nil
}
