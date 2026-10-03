package routeros

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var networkRatePattern = regexp.MustCompile(`^[1-9][0-9]{0,4}[kMG]?/[1-9][0-9]{0,4}[kMG]?$`)

// RouterOS scalar time values use HH:MM:SS, optionally with week/day prefixes;
// input also accepts compact durations. Reject ignored garbage or overflow.
func networkDurationSeconds(s string) (int, bool) {
	s = strings.TrimSpace(s)
	clock := regexp.MustCompile(`^(?:([0-9]{1,2})w)?(?:([0-9]{1,2})d)?([0-9]{1,3}):([0-5][0-9]):([0-5][0-9])$`)
	if groups := clock.FindStringSubmatch(s); groups != nil {
		total := 0
		for i, factor := range []int{604800, 86400, 3600, 60, 1} {
			value, _ := strconv.Atoi(groups[i+1])
			total += value * factor
		}
		return total, total > 0 && total <= 604800
	}
	if regexp.MustCompile(`^[0-9]{1,6}$`).MatchString(s) {
		n, _ := strconv.Atoi(s)
		return n, n > 0 && n <= 604800
	}
	groups := regexp.MustCompile(`([0-9]{1,6})([wdhms])`).FindAllStringSubmatch(s, -1)
	full, total, prior := "", 0, -1
	for _, group := range groups {
		order := strings.Index("wdhms", group[2])
		if order <= prior {
			return 0, false
		}
		prior = order
		n, _ := strconv.Atoi(group[1])
		total += n * []int{604800, 86400, 3600, 60, 1}[order]
		full += group[0]
	}
	return total, full == s && total > 0 && total <= 604800
}

func networkText(v string, max int) bool {
	return len(v) > 0 && len(v) <= max && !strings.ContainsAny(v, "\r\n\x00")
}
func networkIPv4(v string) bool {
	ip := net.ParseIP(v)
	return ip.To4() != nil && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLoopback()
}
func networkCount(ctx context.Context, r CommandRunner, command string) (int, error) {
	out, err := r.Run(ctx, command)
	if err != nil {
		return 0, fmt.Errorf("could not validate router configuration")
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid router count")
	}
	return n, nil
}
func networkRequireInterface(ctx context.Context, r CommandRunner, name string, unused bool) error {
	if !gatewayInterfacePattern.MatchString(name) {
		return fmt.Errorf("choose an actual router interface")
	}
	n, err := networkCount(ctx, r, "/interface print count-only where name="+routerOSQuoted(name)+" disabled=no")
	if err != nil || n != 1 {
		return fmt.Errorf("interface is missing or disabled")
	}
	if unused {
		out, e := r.Run(ctx, `:put [/interface get [find where name=`+routerOSQuoted(name)+`] type]`)
		if e != nil || !strings.Contains(" ether bridge bond vlan ", " "+strings.TrimSpace(out)+" ") {
			return fmt.Errorf("choose a supported customer port, not a loopback or management tunnel")
		}
		for _, menu := range []string{"/interface bridge port", "/ip address", "/ip dhcp-server", "/ip hotspot", "/interface pppoe-server server"} {
			n, err = networkCount(ctx, r, menu+" print count-only where interface="+routerOSQuoted(name))
			if err != nil || n != 0 {
				return fmt.Errorf("customer port is already assigned; choose a dedicated unused interface")
			}
		}
	}
	return nil
}
func networkDNS(v string) error {
	parts := strings.Split(v, ",")
	if len(parts) < 1 || len(parts) > 4 {
		return fmt.Errorf("provide 1-4 IPv4 DNS servers")
	}
	for _, part := range parts {
		if !networkIPv4(strings.TrimSpace(part)) {
			return fmt.Errorf("invalid client DNS address")
		}
	}
	return nil
}
func buildNetworkPlan(ctx context.Context, r CommandRunner, request NetworkRequest) ([]networkStep, []string, error) {
	v := request.Values
	if v == nil {
		return nil, nil, fmt.Errorf("network settings are required")
	}
	allowed := map[string]string{
		"wan":             "mode wan1 wan2 gateway1 gateway2",
		"pppoe_client":    "name interface user password disabled distance",
		"pppoe_server":    "name interface gateway_cidr pool_start pool_end dns rate_limit",
		"pppoe_service":   "default-profile disabled",
		"subscriber":      "name password profile disabled",
		"ppp_profile":     "local-address remote-address dns-server rate-limit only-one",
		"dhcp":            "address-pool lease-time disabled",
		"pool":            "ranges",
		"dhcp_network":    "gateway dns-server",
		"hotspot":         "profile disabled",
		"hotspot_profile": "dns-name login-by use-radius",
		"lease":           "address mac-address server disabled",
		"queue":           "name target max-limit disabled",
		"wifi":            "ssid country password disabled",
		"wireless":        "ssid country password disabled",
	}
	fields, ok := allowed[request.Kind]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported network operation")
	}
	for field := range v {
		if !strings.Contains(" "+fields+" ", " "+field+" ") {
			return nil, nil, fmt.Errorf("unexpected network input")
		}
	}
	steps := []networkStep{}
	if len(v) == 0 {
		return nil, nil, fmt.Errorf("no network settings supplied")
	}
	warnings := []string{"Changes are applied to the selected router only. Test a real client after verified configuration readback."}
	add := func(key, selector, value, label string, values map[string]string) error {
		n, err := networkCount(ctx, r, networkMenus[key].path+" print count-only where "+selector+"="+routerOSQuoted(value))
		if err != nil {
			return err
		}
		if n != 0 {
			return fmt.Errorf("%s already exists; select its record to edit instead", value)
		}
		steps = append(steps, networkStep{Key: key, SelectorField: selector, SelectorValue: value, Label: label, Values: values})
		return nil
	}
	edit := func(key, id, label string, values map[string]string) error {
		if key == "leases" {
			flags, e := networkReadOne(ctx, r, key, id, map[string]string{"dynamic": ""})
			if e != nil || networkBool(flags["dynamic"]) != "no" {
				return fmt.Errorf("only static DHCP reservations can be edited")
			}
		}
		before, err := networkReadOne(ctx, r, key, id, values)
		if err != nil {
			return err
		}
		changed := false
		for field, value := range values {
			if !networkEqual(field, before[field], value) {
				changed = true
			}
		}
		if !changed {
			return nil
		}
		steps = append(steps, networkStep{Key: key, ID: id, Label: label, Values: values, Before: before})
		return nil
	}
	name := v["name"]
	if name != "" && !gatewayNamePattern.MatchString(name) {
		return nil, nil, fmt.Errorf("use a 1-24 character name with letters, numbers, dash or underscore")
	}
	boolValue := func(value string) bool { return value == "yes" || value == "no" }
	var err error
	switch request.Kind {
	case "wan":
		if request.ID != "" || (v["mode"] != "balance" && v["mode"] != "failover") || v["wan1"] == v["wan2"] || !networkIPv4(v["gateway1"]) || !networkIPv4(v["gateway2"]) {
			return nil, nil, fmt.Errorf("choose two distinct active uplinks, their IPv4 ISP gateways and balance or failover mode")
		}
		for _, iface := range []string{v["wan1"], v["wan2"]} {
			if err = networkRequireInterface(ctx, r, iface, false); err != nil {
				return nil, nil, err
			}
		}
		addresses, e := readNetworkObjects(ctx, r, "addresses")
		if e != nil {
			return nil, nil, e
		}
		var uplinkPrefixes []*net.IPNet
		for i := 1; i <= 2; i++ {
			iface, gw := v[fmt.Sprint("wan", i)], net.ParseIP(v[fmt.Sprint("gateway", i)])
			reachable := false
			for _, address := range addresses {
				ip, prefix, e := net.ParseCIDR(address.Values["address"])
				if e == nil && address.Values["interface"] == iface && prefix.Contains(gw) && !gw.Equal(ip) {
					reachable = true
					uplinkPrefixes = append(uplinkPrefixes, prefix)
					break
				}
			}
			if !reachable {
				return nil, nil, fmt.Errorf("ISP gateway must be reachable in the selected uplink's assigned subnet")
			}
		}
		if len(uplinkPrefixes) != 2 || uplinkPrefixes[0].Contains(uplinkPrefixes[1].IP) || uplinkPrefixes[1].Contains(uplinkPrefixes[0].IP) {
			return nil, nil, fmt.Errorf("ISP uplink subnets overlap; use distinct modem LAN subnets before two-WAN routing")
		}
		routes, e := readNetworkObjects(ctx, r, "routes")
		if e != nil {
			return nil, nil, e
		}
		upsert := func(key, comment, label string, values map[string]string) error {
			objects, e := readNetworkObjects(ctx, r, key)
			if e != nil {
				return e
			}
			var matches []NetworkObject
			for _, o := range objects {
				if o.Values["comment"] == comment {
					matches = append(matches, o)
				}
			}
			if len(matches) > 1 {
				return fmt.Errorf("managed WAN records are ambiguous")
			}
			if len(matches) == 1 {
				same := true
				for k, value := range values {
					if !networkEqual(k, matches[0].Values[k], value) {
						same = false
					}
				}
				if same {
					return nil
				}
				return edit(key, matches[0].ID, label, values)
			}
			return add(key, "comment", comment, label, values)
		}
		deferred := []networkStep{}
		clients, e := readNetworkObjects(ctx, r, "dhcp_clients")
		if e != nil {
			return nil, nil, e
		}
		for _, client := range clients {
			iface := client.Values["interface"]
			if iface != v["wan1"] && iface != v["wan2"] && networkBool(client.Values["disabled"]) != "yes" && networkBool(client.Values["add-default-route"]) != "no" {
				return nil, nil, fmt.Errorf("another DHCP uplink supplies a default route; review it before two-WAN routing")
			}
			if iface == v["wan1"] || iface == v["wan2"] {
				if networkBool(client.Values["add-default-route"]) != "no" {
					if err = edit("dhcp_clients", client.ID, "Stop automatic default route on "+iface, map[string]string{"add-default-route": "no"}); err != nil {
						return nil, nil, err
					}
				}
			}
		}
		pppClients, e := readNetworkObjects(ctx, r, "pppoe_clients")
		if e != nil {
			return nil, nil, e
		}
		for _, client := range pppClients {
			if networkBool(client.Values["disabled"]) != "yes" && networkBool(client.Values["add-default-route"]) != "no" {
				return nil, nil, fmt.Errorf("review automatic PPPoE default routes before static two-WAN routing")
			}
		}
		for _, route := range routes {
			if (route.Values["dst-address"] == "1.1.1.1/32" || route.Values["dst-address"] == "8.8.8.8/32") && !strings.HasPrefix(route.Values["comment"], "mm-wan-probe-") {
				return nil, nil, fmt.Errorf("existing health-probe destination route conflicts with this plan")
			}
			if route.Values["dst-address"] == "0.0.0.0/0" && route.Values["routing-table"] == "main" && networkBool(route.Values["dynamic"]) != "yes" && networkBool(route.Values["disabled"]) != "yes" && !strings.HasPrefix(route.Values["comment"], "mm-wan-default-") {
				if err = edit("routes", route.ID, "Disable previous static default route (retained for rollback)", map[string]string{"disabled": "yes"}); err != nil {
					return nil, nil, err
				}
			}
		}
		// Keep the old defaults until probe routes, sticky management routes and
		// the replacement defaults exist. DHCP changes can renew the address.
		deferred = append(deferred, steps...)
		steps = nil
		// Recursive independent ISP probes test upstream connectivity, not merely
		// a modem's local response. ECMP stays in main for Hotspot compatibility.
		for i, probe := range []string{"1.1.1.1", "8.8.8.8"} {
			suffix := strconv.Itoa(i + 1)
			comment := "mm-wan-probe-" + suffix
			if err = upsert("routes", comment, "Route ISP "+suffix+" health probe through its own uplink", map[string]string{"dst-address": probe + "/32", "gateway": v["gateway"+suffix] + "%" + v["wan"+suffix], "distance": "1", "scope": "10", "target-scope": "10", "routing-table": "main", "comment": comment, "disabled": "no"}); err != nil {
				return nil, nil, err
			}
			distance := "1"
			if i == 1 && v["mode"] == "failover" {
				distance = "2"
			}
			comment = "mm-wan-default-" + suffix
			if err = upsert("routes", comment, "Install "+v["mode"]+" default route for ISP "+suffix+" with recursive ping health check", map[string]string{"dst-address": "0.0.0.0/0", "gateway": probe, "distance": distance, "scope": "30", "target-scope": "11", "check-gateway": "ping", "routing-table": "main", "comment": comment, "disabled": "no"}); err != nil {
				return nil, nil, err
			}
			comment = "mm-wan-nat-" + suffix
			if err = upsert("nat", comment, "Masquerade customer internet traffic on "+v["wan"+suffix], map[string]string{"chain": "srcnat", "action": "masquerade", "out-interface": v["wan"+suffix], "comment": comment, "disabled": "no"}); err != nil {
				return nil, nil, err
			}
		}
		// Current SSH clients and active WireGuard endpoints must not enter ECMP
		// arbitrarily. Stay on primary while healthy, then use the backup probe.
		management := map[string]bool{}
		for _, key := range []string{"ssh_sessions", "wg_peers"} {
			objects, e := readNetworkObjects(ctx, r, key)
			if e != nil {
				return nil, nil, fmt.Errorf("could not discover management return paths safely")
			}
			for _, o := range objects {
				address := o.Values["current-endpoint-address"]
				if key == "ssh_sessions" {
					if o.Values["via"] != "ssh" {
						continue
					}
					address = o.Values["address"]
				}
				if networkIPv4(address) {
					management[address] = true
				}
			}
		}
		if len(management) > 16 {
			return nil, nil, fmt.Errorf("too many management endpoints for a bounded WAN change")
		}
		for _, address := range networkSortedKeys(func() map[string]string {
			m := map[string]string{}
			for ip := range management {
				m[ip] = ip
			}
			return m
		}()) {
			connected := false
			ip := net.ParseIP(address)
			for _, a := range addresses {
				_, prefix, e := net.ParseCIDR(a.Values["address"])
				if e == nil && prefix.Contains(ip) {
					connected = true
				}
			}
			if connected {
				continue
			}
			for i, probe := range []string{"1.1.1.1", "8.8.8.8"} {
				comment := "mm-wan-management-" + address + "-" + strconv.Itoa(i+1)
				if err = upsert("routes", comment, "Preserve management return path with primary/backup health checks", map[string]string{"dst-address": address + "/32", "gateway": probe, "distance": strconv.Itoa(i + 1), "scope": "30", "target-scope": "11", "check-gateway": "ping", "routing-table": "main", "comment": comment, "disabled": "no"}); err != nil {
					return nil, nil, err
				}
			}
		}
		steps = append(steps, deferred...)
		warnings = append(warnings, "This replaces main-table defaults, affecting internet access and router-originated traffic. ISP DHCP address/gateway changes require policy re-review. Existing sessions can reset when the public source IP changes. Probe ICMP must be reachable. A router-side timed restoration guard protects management loss.")
	case "pppoe_client":
		if name == "" || !networkText(v["user"], 128) || !networkText(v["password"], 128) || !boolValue(v["disabled"]) {
			return nil, nil, fmt.Errorf("provide uplink name, ISP username/password and enabled state")
		}
		distance, e := strconv.Atoi(v["distance"])
		if e != nil || distance < 1 || distance > 254 {
			return nil, nil, fmt.Errorf("route distance must be 1-254")
		}
		if err = networkRequireInterface(ctx, r, v["interface"], false); err != nil {
			return nil, nil, err
		}
		values := map[string]string{"name": "mm-" + name, "interface": v["interface"], "user": v["user"], "password": v["password"], "disabled": v["disabled"], "add-default-route": "yes", "default-route-distance": strconv.Itoa(distance), "use-peer-dns": "no"}
		if request.ID == "" {
			err = add("pppoe_clients", "name", values["name"], "Create PPPoE ISP uplink (credentials hidden)", values)
		} else {
			delete(values, "name")
			err = edit("pppoe_clients", request.ID, "Update selected PPPoE ISP uplink (credentials hidden)", values)
		}
		warnings = append(warnings, "Configuration verification does not prove the ISP accepted the account; check PPPoE link status and actual internet traffic.")
	case "pppoe_server":
		if name == "" || request.ID != "" {
			return nil, nil, fmt.Errorf("provide a new PPPoE service name")
		}
		if err = networkRequireInterface(ctx, r, v["interface"], true); err != nil {
			return nil, nil, err
		}
		g, network, e := normalizeGatewaySetup(GatewaySetup{Name: name, WAN: "validation-uplink", LAN: v["interface"], Address: v["gateway_cidr"], PoolStart: v["pool_start"], PoolEnd: v["pool_end"], DNS: strings.Split(v["dns"], ",")})
		if e != nil {
			return nil, nil, e
		}
		reserved, e := gatewayReservedNetworks(ctx, r)
		if e != nil {
			return nil, nil, e
		}
		if gatewayNetworkOverlaps(network, reserved) {
			return nil, nil, fmt.Errorf("subscriber subnet overlaps existing addresses or routes")
		}
		if v["rate_limit"] != "" && !networkRatePattern.MatchString(v["rate_limit"]) {
			return nil, nil, fmt.Errorf("use upload/download rate such as 10M/20M")
		}
		owned := "mm-" + name
		if err = add("pools", "name", owned, "Create PPPoE subscriber address pool", map[string]string{"name": owned, "ranges": g.PoolStart + "-" + g.PoolEnd}); err != nil {
			return nil, nil, err
		}
		if err = add("ppp_profiles", "name", owned, "Create subscriber profile with addresses, DNS and bandwidth", map[string]string{"name": owned, "local-address": strings.Split(g.Address, "/")[0], "remote-address": owned, "dns-server": strings.Join(g.DNS, ","), "rate-limit": v["rate_limit"], "only-one": "yes"}); err != nil {
			return nil, nil, err
		}
		err = add("pppoe_servers", "service-name", owned, "Enable authenticated PPPoE service on dedicated customer interface", map[string]string{"service-name": owned, "interface": v["interface"], "default-profile": owned, "authentication": "chap,mschap2", "one-session-per-host": "yes", "disabled": "no"})
		warnings = append(warnings, "The physical PPPoE customer interface deliberately receives no IP address or DHCP server. Create subscribers next. Upstream NAT/firewall policy must permit subscriber internet traffic.")
	case "subscriber":
		if name == "" || !networkText(v["password"], 128) || !gatewayInterfacePattern.MatchString(v["profile"]) || !boolValue(v["disabled"]) {
			return nil, nil, fmt.Errorf("provide subscriber name, password, PPP profile and enabled state")
		}
		n, e := networkCount(ctx, r, "/ppp profile print count-only where name="+routerOSQuoted(v["profile"]))
		if e != nil || n != 1 {
			return nil, nil, fmt.Errorf("choose an existing PPP profile")
		}
		values := map[string]string{"name": name, "password": v["password"], "service": "pppoe", "profile": v["profile"], "disabled": v["disabled"]}
		if request.ID == "" {
			err = add("subscribers", "name", name, "Create authenticated PPPoE subscriber (password hidden)", values)
		} else {
			delete(values, "name")
			err = edit("subscribers", request.ID, "Update PPPoE subscriber profile, password and state", values)
		}
	case "wifi", "wireless":
		if request.ID == "" || !networkText(v["ssid"], 32) || !networkText(v["country"], 64) || !networkText(v["password"], 63) || len(v["password"]) < 8 || !boolValue(v["disabled"]) {
			return nil, nil, fmt.Errorf("select a radio, provide 1-32 byte SSID, actual installation country and 8-63 character Wi-Fi password")
		}
		if request.Kind == "wifi" {
			err = edit("wifi", request.ID, "Update selected Wi-Fi radio SSID, country and WPA2 security (password hidden)", map[string]string{"configuration.ssid": v["ssid"], "configuration.country": v["country"], "configuration.mode": "ap", "security.authentication-types": "wpa2-psk", "security.passphrase": v["password"], "disabled": v["disabled"]})
		} else {
			profile := "mm-wifi-" + strings.TrimPrefix(request.ID, "*")
			profiles, e := readNetworkObjects(ctx, r, "wireless_security")
			if e != nil {
				return nil, nil, e
			}
			var matched []NetworkObject
			for _, p := range profiles {
				if p.Values["name"] == profile {
					matched = append(matched, p)
				}
			}
			security := map[string]string{"mode": "dynamic-keys", "authentication-types": "wpa2-psk", "wpa2-pre-shared-key": v["password"]}
			if len(matched) > 1 {
				return nil, nil, fmt.Errorf("dedicated Wi-Fi security profile is ambiguous")
			}
			if len(matched) == 1 {
				uses, e := networkCount(ctx, r, "/interface wireless print count-only where security-profile="+routerOSQuoted(profile))
				if e != nil || uses > 1 {
					return nil, nil, fmt.Errorf("Wi-Fi security profile is shared; separate the radios before editing")
				}
				err = edit("wireless_security", matched[0].ID, "Update dedicated WPA2 security profile (password hidden)", security)
			} else {
				security["name"] = profile
				err = add("wireless_security", "name", profile, "Create dedicated WPA2 security profile (password hidden)", security)
			}
			if err != nil {
				return nil, nil, err
			}
			err = edit("wireless", request.ID, "Update selected legacy radio SSID, country and dedicated security profile", map[string]string{"ssid": v["ssid"], "country": v["country"], "mode": "ap-bridge", "security-profile": profile, "disabled": v["disabled"]})
		}
		warnings = append(warnings, "Changing the SSID/password disconnects wireless clients. Country must match the physical installation. Remote APs must be managed as their own registered devices; a radio-free CHR cannot broadcast Wi-Fi.")
	default:
		key := map[string]string{"pppoe_service": "pppoe_servers", "ppp_profile": "ppp_profiles", "dhcp": "dhcp", "pool": "pools", "dhcp_network": "dhcp_networks", "hotspot": "hotspots", "hotspot_profile": "hotspot_profiles", "lease": "leases", "queue": "queues"}[request.Kind]
		values := map[string]string{}
		for field, value := range v {
			values[field] = strings.TrimSpace(value)
			if !networkText(value, 128) {
				return nil, nil, fmt.Errorf("provide valid network values")
			}
		}
		if value, ok := values["disabled"]; ok && !boolValue(value) {
			return nil, nil, fmt.Errorf("enabled state must be yes or no")
		}
		if value, ok := values["only-one"]; ok && !boolValue(value) {
			return nil, nil, fmt.Errorf("choose one-session state")
		}
		for _, field := range []string{"gateway", "local-address", "address"} {
			if value, ok := values[field]; ok && !networkIPv4(value) {
				return nil, nil, fmt.Errorf("invalid IPv4 address")
			}
		}
		for _, field := range []string{"dns-server"} {
			if value, ok := values[field]; ok {
				if e := networkDNS(value); e != nil {
					return nil, nil, e
				}
			}
		}
		for _, field := range []string{"rate-limit", "max-limit"} {
			if value, ok := values[field]; ok && !networkRatePattern.MatchString(value) {
				return nil, nil, fmt.Errorf("use upload/download rates such as 10M/20M")
			}
		}
		if value, ok := values["lease-time"]; ok {
			seconds, valid := networkDurationSeconds(value)
			if !valid || seconds < 60 {
				return nil, nil, fmt.Errorf("DHCP lease time must be 1 minute through 7 days")
			}
		}
		if value, ok := values["ranges"]; ok {
			ends := strings.Split(value, "-")
			if len(ends) != 2 || !networkIPv4(ends[0]) || !networkIPv4(ends[1]) || gatewayIPv4Number(net.ParseIP(ends[0])) > gatewayIPv4Number(net.ParseIP(ends[1])) {
				return nil, nil, fmt.Errorf("provide an ordered IPv4 pool range")
			}
			old, e := networkReadOne(ctx, r, "pools", request.ID, map[string]string{"ranges": ""})
			if e != nil {
				return nil, nil, e
			}
			prior := strings.Split(old["ranges"], "-")
			if len(prior) != 2 || !networkIPv4(prior[0]) || !networkIPv4(prior[1]) || gatewayIPv4Number(net.ParseIP(ends[0])) < gatewayIPv4Number(net.ParseIP(prior[0])) || gatewayIPv4Number(net.ParseIP(ends[1])) > gatewayIPv4Number(net.ParseIP(prior[1])) {
				return nil, nil, fmt.Errorf("pool edits must stay within the current allocation; create a separately reviewed network to expand or move it")
			}
		}
		if value, ok := values["mac-address"]; ok {
			mac, e := net.ParseMAC(value)
			if e != nil || len(mac) != 6 {
				return nil, nil, fmt.Errorf("provide a valid six-byte MAC address")
			}
			values["mac-address"] = strings.ToUpper(mac.String())
		}
		if value, ok := values["target"]; ok {
			_, prefix, e := net.ParseCIDR(value)
			if e != nil || prefix.IP.To4() == nil {
				return nil, nil, fmt.Errorf("bandwidth target must be an IPv4 prefix")
			}
		}
		for field, menu := range map[string]string{"address-pool": "/ip pool", "remote-address": "/ip pool", "profile": "/ip hotspot profile", "default-profile": "/ppp profile", "server": "/ip dhcp-server"} {
			if value, ok := values[field]; ok {
				n, e := networkCount(ctx, r, menu+" print count-only where name="+routerOSQuoted(value))
				if e != nil || n != 1 {
					return nil, nil, fmt.Errorf("select an existing %s record", field)
				}
			}
		}
		if request.Kind == "queue" {
			for _, f := range []string{"name", "target", "max-limit", "disabled"} {
				if values[f] == "" {
					return nil, nil, fmt.Errorf("provide bandwidth name, target, speeds and state")
				}
			}
		}
		if request.Kind == "lease" {
			for _, f := range []string{"address", "mac-address", "server", "disabled"} {
				if values[f] == "" {
					return nil, nil, fmt.Errorf("provide reservation address, MAC, DHCP service and state")
				}
			}
			servers, e := readNetworkObjects(ctx, r, "dhcp")
			if e != nil {
				return nil, nil, e
			}
			iface := ""
			for _, server := range servers {
				if server.Values["name"] == values["server"] {
					iface = server.Values["interface"]
				}
			}
			addresses, e := readNetworkObjects(ctx, r, "addresses")
			if e != nil {
				return nil, nil, e
			}
			valid := false
			for _, address := range addresses {
				ip, prefix, e := net.ParseCIDR(address.Values["address"])
				client := net.ParseIP(values["address"])
				if e == nil && address.Values["interface"] == iface && prefix.Contains(client) && !ip.Equal(client) && !prefix.IP.Equal(client) {
					ones, bits := prefix.Mask.Size()
					if bits == 32 && ones <= 30 {
						broadcast := gatewayIPv4Number(prefix.IP) | (^uint32(0) >> ones)
						valid = gatewayIPv4Number(client) != broadcast
					}
				}
			}
			if !valid {
				return nil, nil, fmt.Errorf("reservation must be a usable client address in the selected DHCP interface subnet")
			}
		}
		if request.Kind == "dhcp_network" && values["gateway"] != "" {
			old, e := networkReadOne(ctx, r, key, request.ID, map[string]string{"address": ""})
			if e != nil {
				return nil, nil, e
			}
			_, prefix, e := net.ParseCIDR(old["address"])
			if e != nil || !prefix.Contains(net.ParseIP(values["gateway"])) {
				return nil, nil, fmt.Errorf("client gateway must belong to its DHCP subnet")
			}
		}
		if request.Kind == "hotspot_profile" && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,126}$`).MatchString(values["dns-name"]) {
			return nil, nil, fmt.Errorf("provide a valid portal DNS name")
		}
		if request.Kind == "hotspot_profile" {
			if values["login-by"] != "http-chap,mac" && values["login-by"] != "http-chap" {
				return nil, nil, fmt.Errorf("use CHAP login with optional MAC grace; plaintext HTTP PAP is not offered")
			}
			if !boolValue(values["use-radius"]) {
				return nil, nil, fmt.Errorf("choose RADIUS state")
			}
		}
		if request.ID == "" {
			if request.Kind == "queue" && name != "" {
				values["name"] = "mm-" + name
				err = add(key, "name", values["name"], "Create customer bandwidth limit", values)
			} else if request.Kind == "lease" {
				err = add(key, "mac-address", values["mac-address"], "Create static customer DHCP reservation", values)
			} else {
				return nil, nil, fmt.Errorf("select an existing record to edit")
			}
		} else {
			if len(values) == 0 {
				return nil, nil, fmt.Errorf("no settings supplied")
			}
			err = edit(key, request.ID, "Update selected "+request.Kind+" settings", values)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if len(steps) == 0 {
		return nil, nil, fmt.Errorf("selected settings are already applied")
	}
	if len(steps) > 40 {
		return nil, nil, fmt.Errorf("network change is too large for one guarded operation")
	}
	return steps, warnings, nil
}
