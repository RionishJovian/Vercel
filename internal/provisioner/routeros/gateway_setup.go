package routeros

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var gatewayNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,23}$`)
var gatewayInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
var gatewayAddressPattern = regexp.MustCompile(`(?m)^\s*address:\s*(\S+)`)
var gatewayLocks sync.Map

// GatewaySetup adds a separate customer network. It never rewrites an existing
// default route, management address, bridge membership, or WAN configuration.
type GatewaySetup struct {
	Name          string   `json:"name"`
	WAN           string   `json:"wan_interface"`
	LAN           string   `json:"lan_interface"`
	VLAN          int      `json:"vlan_id"`
	Address       string   `json:"gateway_cidr"`
	PoolStart     string   `json:"pool_start"`
	PoolEnd       string   `json:"pool_end"`
	DNS           []string `json:"dns_servers"`
	Hotspot       bool     `json:"hotspot"`
	RadiusAddress string   `json:"radius_address"`
	RadiusSecret  string   `json:"radius_secret"`
	Confirmation  string   `json:"confirmation"`
}

type GatewayPlan struct {
	Steps        []string `json:"steps"`
	Confirmation string   `json:"confirmation"`
	Network      string   `json:"network"`
	Interface    string   `json:"interface"`
}

type GatewayResult struct {
	Backup     string      `json:"backup"`
	Verified   bool        `json:"verified"`
	RolledBack bool        `json:"rolled_back"`
	Plan       GatewayPlan `json:"plan"`
}

type GatewayInterface struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Running bool   `json:"running"`
}

func (a *DeviceApplier) GatewayInterfaces(ctx context.Context, businessID, deviceID string) ([]GatewayInterface, error) {
	r, e := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if e != nil {
		return nil, e
	}
	out, e := r.Run(ctx, "/interface print value-list detail")
	if e != nil {
		return nil, e
	}
	interfaces := []GatewayInterface{}
	fields := map[string]string{}
	flush := func() {
		if gatewayInterfacePattern.MatchString(fields["name"]) && fields["disabled"] != "true" && fields["disabled"] != "yes" {
			interfaces = append(interfaces, GatewayInterface{Name: fields["name"], Type: fields["type"], Running: fields["running"] == "yes" || fields["running"] == "true"})
		}
		fields = map[string]string{}
	}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "name" && fields["name"] != "" {
			flush()
		}
		fields[key] = strings.TrimSpace(value)
	}
	if len(fields) > 0 {
		flush()
	}
	return interfaces, nil
}

type gatewayStep struct {
	menu, field, key string
	values           map[string]string
}

func gatewayIPv4Number(ip net.IP) uint32 {
	b := ip.To4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func normalizeGatewaySetup(g GatewaySetup) (GatewaySetup, *net.IPNet, error) {
	if !gatewayNamePattern.MatchString(g.Name) || !gatewayInterfacePattern.MatchString(g.WAN) || !gatewayInterfacePattern.MatchString(g.LAN) || g.WAN == g.LAN || g.VLAN < 0 || g.VLAN > 4094 {
		return g, nil, fmt.Errorf("choose a network name, distinct uplink/LAN interfaces and VLAN 0-4094")
	}
	ip, network, err := net.ParseCIDR(g.Address)
	if err != nil || ip.To4() == nil || !ip.IsPrivate() {
		return g, nil, fmt.Errorf("gateway must be a private IPv4 address with a prefix")
	}
	prefix, _ := network.Mask.Size()
	if prefix < 16 || prefix > 29 {
		return g, nil, fmt.Errorf("customer network prefix must be /16 through /29")
	}
	start, end := net.ParseIP(g.PoolStart), net.ParseIP(g.PoolEnd)
	if start.To4() == nil || end.To4() == nil || !network.Contains(start) || !network.Contains(end) || gatewayIPv4Number(start) > gatewayIPv4Number(end) {
		return g, nil, fmt.Errorf("DHCP range must be ordered IPv4 addresses inside the customer network")
	}
	n, host, lo, hi := gatewayIPv4Number(network.IP), gatewayIPv4Number(ip), gatewayIPv4Number(start), gatewayIPv4Number(end)
	broadcast := n | ^gatewayIPv4Number(net.IP(network.Mask))
	if host == n || host == broadcast || lo <= n || hi >= broadcast || (host >= lo && host <= hi) {
		return g, nil, fmt.Errorf("DHCP range must exclude the gateway, network and broadcast addresses")
	}
	if len(g.DNS) < 1 || len(g.DNS) > 4 {
		return g, nil, fmt.Errorf("provide 1-4 IPv4 DNS servers")
	}
	for i, raw := range g.DNS {
		dns := net.ParseIP(raw)
		if dns.To4() == nil || dns.IsUnspecified() || dns.IsMulticast() {
			return g, nil, fmt.Errorf("invalid IPv4 DNS server")
		}
		g.DNS[i] = dns.To4().String()
	}
	if g.RadiusAddress != "" {
		r := net.ParseIP(g.RadiusAddress)
		if !g.Hotspot || r.To4() == nil || r.IsUnspecified() || r.IsMulticast() || len(g.RadiusSecret) < 8 || len(g.RadiusSecret) > 128 || strings.ContainsAny(g.RadiusSecret, "\r\n\x00") {
			return g, nil, fmt.Errorf("RADIUS needs Hotspot, an IPv4 server and an 8-128 character secret")
		}
		g.RadiusAddress = r.To4().String()
	} else if g.RadiusSecret != "" {
		return g, nil, fmt.Errorf("provide the RADIUS server address")
	}
	g.Address = ip.To4().String() + "/" + strconv.Itoa(prefix)
	g.PoolStart = start.To4().String()
	g.PoolEnd = end.To4().String()
	return g, network, nil
}

func gatewaySteps(g GatewaySetup, network *net.IPNet) (GatewayPlan, []gatewayStep) {
	name := "mm-" + g.Name
	iface := g.LAN
	steps := []gatewayStep{}
	labels := []string{}
	add := func(menu, field, key, label string, values map[string]string) {
		steps = append(steps, gatewayStep{menu, field, key, values})
		labels = append(labels, label)
	}
	if g.VLAN > 0 {
		iface = name + "-vlan"
		add("/interface vlan", "name", iface, "Create tagged VLAN "+strconv.Itoa(g.VLAN), map[string]string{"name": iface, "interface": g.LAN, "vlan-id": strconv.Itoa(g.VLAN), "disabled": "no"})
	}
	add("/ip firewall filter", "comment", name+"-setup", "Block customer forwarding until setup verifies", map[string]string{"chain": "forward", "action": "drop", "in-interface": iface, "comment": name + "-setup"})
	add("/ip address", "comment", name, "Assign gateway "+g.Address+" on "+iface, map[string]string{"address": g.Address, "interface": iface, "comment": name})
	add("/ip pool", "name", name, "Create DHCP pool "+g.PoolStart+"–"+g.PoolEnd, map[string]string{"name": name, "ranges": g.PoolStart + "-" + g.PoolEnd})
	gw := strings.Split(g.Address, "/")[0]
	add("/ip dhcp-server network", "comment", name, "Advertise gateway and DNS to clients", map[string]string{"address": network.String(), "gateway": gw, "dns-server": strings.Join(g.DNS, ","), "comment": name})
	add("/ip dhcp-server", "name", name, "Enable DHCP on "+iface, map[string]string{"name": name, "interface": iface, "address-pool": name, "lease-time": "1h", "disabled": "no"})
	add("/ip firewall nat", "comment", name, "Masquerade only "+network.String()+" via "+g.WAN, map[string]string{"chain": "srcnat", "action": "masquerade", "src-address": network.String(), "out-interface": g.WAN, "comment": name})
	if g.RadiusAddress != "" {
		add("/radius", "comment", name, "Connect Hotspot to RADIUS "+g.RadiusAddress+" (secret hidden)", map[string]string{"address": g.RadiusAddress, "service": "hotspot", "secret": g.RadiusSecret, "comment": name})
	}
	if g.Hotspot {
		radius := "no"
		if g.RadiusAddress != "" {
			radius = "yes"
		}
		add("/ip hotspot profile", "name", name, "Create Hotspot login profile with MAC grace access", map[string]string{"name": name, "hotspot-address": gw, "login-by": "http-chap,mac", "use-radius": radius, "html-directory": "hotspot"})
		add("/ip hotspot", "name", name, "Enable captive Hotspot on "+iface, map[string]string{"name": name, "interface": iface, "profile": name, "address-pool": name, "disabled": "no"})
	}
	g.Confirmation = ""
	body, _ := json.Marshal(g)
	hash := sha256.Sum256(body)
	return GatewayPlan{Steps: labels, Confirmation: hex.EncodeToString(hash[:]), Network: network.String(), Interface: iface}, steps
}

func gatewaySelector(s gatewayStep) string { return s.field + "=" + routerOSQuoted(s.key) }

func gatewayPreflight(ctx context.Context, r CommandRunner, g GatewaySetup, network *net.IPNet, steps []gatewayStep) error {
	count := func(command string) (int, error) {
		out, e := r.Run(ctx, command)
		if e != nil {
			return 0, e
		}
		n, e := strconv.Atoi(strings.TrimSpace(out))
		return n, e
	}
	for _, iface := range []string{g.LAN, g.WAN} {
		n, e := count("/interface print count-only where name=" + routerOSQuoted(iface) + " disabled=no")
		if e != nil || n != 1 {
			return fmt.Errorf("interface %s is missing or disabled", iface)
		}
	}
	if g.VLAN == 0 {
		n, e := count("/interface bridge port print count-only where interface=" + routerOSQuoted(g.LAN))
		if e != nil || n > 0 {
			return fmt.Errorf("use the LAN bridge itself, not a bridge member port")
		}
	}
	for _, menu := range []string{"/ip address", "/ip dhcp-server", "/ip hotspot"} {
		n, e := count(menu + " print count-only where interface=" + routerOSQuoted(g.LAN))
		if e != nil || (g.VLAN == 0 && n > 0) {
			return fmt.Errorf("LAN interface is already configured; choose an unused interface or a new VLAN")
		}
	}
	reserved, e := gatewayReservedNetworks(ctx, r)
	if e != nil {
		return e
	}
	if gatewayNetworkOverlaps(network, reserved) {
		return fmt.Errorf("customer subnet overlaps an existing router address or route")
	}
	if g.RadiusAddress != "" {
		n, e := count("/ip hotspot print count-only")
		if e != nil || n > 0 {
			return fmt.Errorf("existing Hotspots must keep their RADIUS policy; this setup cannot add a global RADIUS server to them")
		}
		n, e = count("/radius print count-only")
		if e != nil || n > 0 {
			return fmt.Errorf("existing RADIUS configuration must not be overwritten")
		}
	}
	for _, s := range steps {
		n, e := count(s.menu + " print count-only where " + gatewaySelector(s))
		if e != nil || n > 0 {
			return fmt.Errorf("network name is already in use or menu unavailable: %s", s.menu)
		}
	}
	return nil
}

func (a *DeviceApplier) PreviewGateway(ctx context.Context, businessID, deviceID string, request GatewaySetup) (GatewayPlan, error) {
	g, network, e := normalizeGatewaySetup(request)
	if e != nil {
		return GatewayPlan{}, e
	}
	r, e := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if e != nil {
		return GatewayPlan{}, e
	}
	plan, steps := gatewaySteps(g, network)
	e = gatewayPreflight(ctx, r, g, network, steps)
	return plan, e
}

func (a *DeviceApplier) ApplyGateway(ctx context.Context, businessID, deviceID string, request GatewaySetup) (GatewayResult, error) {
	g, network, e := normalizeGatewaySetup(request)
	if e != nil {
		return GatewayResult{}, e
	}
	plan, steps := gatewaySteps(g, network)
	result := GatewayResult{Plan: plan}
	if g.Confirmation != plan.Confirmation {
		return result, fmt.Errorf("preview the current settings before applying")
	}
	lock, _ := gatewayLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	r, e := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if e != nil {
		return result, e
	}
	if e = gatewayPreflight(ctx, r, g, network, steps); e != nil {
		return result, e
	}
	result.Backup = snapshotName(deviceID+"-gateway", time.Now().UTC())
	for _, cmd := range []string{"/export file=" + result.Backup, "/system backup save name=" + result.Backup} {
		if _, e = r.Run(ctx, cmd); e != nil {
			return result, fmt.Errorf("gateway backup failed; nothing applied")
		}
	}
	created := []gatewayStep{}
	rollback := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ok := true
		for i := len(created) - 1; i >= 0; i-- {
			s := created[i]
			if _, e := r.Run(cleanup, s.menu+" remove [find "+gatewaySelector(s)+"]"); e != nil {
				ok = false
			}
			out, e := r.Run(cleanup, s.menu+" print count-only where "+gatewaySelector(s))
			if e != nil || strings.TrimSpace(out) != "0" {
				ok = false
			}
		}
		result.RolledBack = ok
	}
	for _, s := range steps {
		keys := make([]string, 0, len(s.values))
		for k := range s.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		cmd := s.menu + " add"
		for _, k := range keys {
			cmd += " " + k + "=" + routerOSQuoted(s.values[k])
		}
		created = append(created, s)
		if _, e = r.Run(ctx, cmd); e != nil {
			rollback()
			return result, fmt.Errorf("gateway write failed at %s; check rollback status", s.menu)
		}
		if s.menu == "/ip firewall filter" {
			if _, e = r.Run(ctx, s.menu+" move [find "+gatewaySelector(s)+"] 0"); e != nil {
				rollback()
				return result, fmt.Errorf("could not install temporary setup traffic guard")
			}
		}
		out, e := r.Run(ctx, s.menu+" print value-list detail where "+gatewaySelector(s))
		if e != nil {
			rollback()
			return result, fmt.Errorf("gateway readback failed at %s", s.menu)
		}
		actual := parseValueList(out)
		if actual["invalid"] == "yes" || actual["invalid"] == "true" {
			rollback()
			return result, fmt.Errorf("RouterOS marked %s invalid", s.menu)
		}
		for key, want := range s.values {
			if key == "secret" {
				continue
			}
			got := actual[key]
			if key == "disabled" || key == "use-radius" {
				got = strings.ReplaceAll(strings.ReplaceAll(got, "false", "no"), "true", "yes")
			}
			if key == "lease-time" {
				if parseUptimeSeconds(got) == 3600 {
					got = "1h"
				}
			}
			if key == "login-by" || key == "dns-server" {
				a, b := strings.Split(got, ","), strings.Split(want, ",")
				sort.Strings(a)
				sort.Strings(b)
				got = strings.Join(a, ",")
				want = strings.Join(b, ",")
			}
			if got != want {
				rollback()
				return result, fmt.Errorf("gateway verification failed at %s field %s", s.menu, key)
			}
		}
	}
	guard := gatewayStep{menu: "/ip firewall filter", field: "comment", key: "mm-" + g.Name + "-setup"}
	if _, e = r.Run(ctx, guard.menu+" remove [find "+gatewaySelector(guard)+"]"); e != nil {
		rollback()
		return result, fmt.Errorf("could not release setup traffic guard")
	}
	out, e := r.Run(ctx, guard.menu+" print count-only where "+gatewaySelector(guard))
	if e != nil || strings.TrimSpace(out) != "0" {
		rollback()
		return result, fmt.Errorf("setup traffic guard removal did not verify")
	}
	result.Verified = true
	return result, nil
}
