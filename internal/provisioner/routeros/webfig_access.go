package routeros

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// WebFigRuleComment marks the one firewall rule cloud WebFig adds to a router.
// The Hub's disconnect script removes it by this exact comment.
const WebFigRuleComment = "mega-magic:webfig"

// ErrWebFigNoTunnel means the router has no Magic Hub interface to allow.
var ErrWebFigNoTunnel = errors.New("this router is not connected through Magic Hub")

var webFigTunnelName = regexp.MustCompile(`^mmh-[0-9a-f]{12}$`)

// WebFigAccess is what the router reports about reaching WebFig from the Hub.
// Plan lists, in plain words, every change Enable would make; it is empty when
// the router is ready. The console shows it for review before anything changes.
type WebFigAccess struct {
	Version string   `json:"version"`
	Scheme  string   `json:"-"`
	Port    int      `json:"-"`
	Ready   bool     `json:"ready"`
	Plan    []string `json:"plan"`
	Service string   `json:"service,omitempty"`

	addRule      bool
	enableWWW    bool
	allowList    []string
	writeAllowed bool
}

type webFigServiceRow struct {
	port     int
	disabled bool
	allowed  []string
}

// webFigReadServices prints each row in one fixed format. RouterOS names the
// source list "available-from" now and "address" on older versions; each name is
// parsed only when it runs, so one absent property does not fail the whole line.
const webFigReadServices = `:foreach s in=[/ip/service find where name="www" or name="www-ssl"] do={:local a ""; :do {:set a [[:parse (":return [/ip/service get " . $s . " available-from]")]]} on-error={:do {:set a [[:parse (":return [/ip/service get " . $s . " address]")]]} on-error={}}; :put ("mm|" . [/ip/service get $s name] . "|" . [/ip/service get $s port] . "|" . [/ip/service get $s disabled] . "|" . [:tostr $a])}`

func parseWebFigServices(out string) map[string]webFigServiceRow {
	rows := map[string]webFigServiceRow{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), "|")
		if len(f) != 5 || f[0] != "mm" {
			continue
		}
		// /ip/service also lists live connections; the service itself comes first.
		if _, seen := rows[f[1]]; seen {
			continue
		}
		port, err := strconv.Atoi(f[2])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		row := webFigServiceRow{port: port, disabled: f[3] == "true" || f[3] == "yes"}
		for _, entry := range strings.FieldsFunc(f[4], func(r rune) bool { return r == ';' || r == ',' }) {
			row.allowed = append(row.allowed, strings.TrimSpace(entry))
		}
		rows[f[1]] = row
	}
	return rows
}

func webFigAllows(list []string, sourceIP string) bool {
	if len(list) == 0 {
		return true
	}
	ip := net.ParseIP(sourceIP)
	for _, entry := range list {
		if _, network, err := net.ParseCIDR(entry); err == nil && ip != nil && network.Contains(ip) {
			return true
		}
		if entry == sourceIP {
			return true
		}
	}
	return false
}

func webFigSource(sourceCIDR string) (string, error) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(sourceCIDR))
	if err != nil || ip.To4() == nil {
		return "", fmt.Errorf("WebFig needs a Hub source address")
	}
	if ones, _ := network.Mask.Size(); ones != 32 {
		return "", fmt.Errorf("WebFig needs a single Hub source address")
	}
	return ip.String(), nil
}

func webFigRuleCount(ctx context.Context, runner CommandRunner, tunnel, sourceIP string, port int) (int, error) {
	return networkCount(ctx, runner, fmt.Sprintf(`/ip/firewall/filter print count-only where comment=%s and chain=input and action=accept and in-interface=%s and src-address=%s and protocol=tcp and dst-port=%s and disabled=no`,
		routerOSQuoted(WebFigRuleComment), routerOSQuoted(tunnel), routerOSQuoted(sourceIP+"/32"), routerOSQuoted(strconv.Itoa(port))))
}

func readWebFigAccess(ctx context.Context, runner CommandRunner, tunnel, sourceCIDR string) (WebFigAccess, error) {
	access := WebFigAccess{Plan: []string{}}
	if !webFigTunnelName.MatchString(tunnel) {
		return access, ErrWebFigNoTunnel
	}
	sourceIP, err := webFigSource(sourceCIDR)
	if err != nil {
		return access, err
	}
	n, err := networkCount(ctx, runner, "/interface/wireguard print count-only where name="+routerOSQuoted(tunnel))
	if err != nil {
		return access, err
	}
	if n != 1 {
		return access, ErrWebFigNoTunnel
	}
	versionOut, err := runner.Run(ctx, ":put [/system/resource/get version]")
	if err != nil {
		return access, fmt.Errorf("could not read the RouterOS version")
	}
	if fields := strings.Fields(versionOut); len(fields) > 0 {
		access.Version = fields[0]
	}
	out, err := runner.Run(ctx, webFigReadServices)
	if err != nil {
		return access, fmt.Errorf("could not read the router's web services")
	}
	rows := parseWebFigServices(out)
	www, hasWWW := rows["www"]
	secure, hasSecure := rows["www-ssl"]
	var row webFigServiceRow
	switch {
	case hasWWW && !www.disabled:
		access.Service, access.Scheme, row = "www", "http", www
	case hasSecure && !secure.disabled:
		access.Service, access.Scheme, row = "www-ssl", "https", secure
	case hasWWW:
		access.Service, access.Scheme, row = "www", "http", www
		access.enableWWW = true
		access.Plan = append(access.Plan, fmt.Sprintf("Turn on the router's web service (www, port %d) for the Hub address %s only. WebFig is off on this router today.", www.port, sourceIP))
	default:
		return access, fmt.Errorf("could not read the router's web services")
	}
	access.Port = row.port
	access.allowList = row.allowed
	if !access.enableWWW && !webFigAllows(row.allowed, sourceIP) {
		access.writeAllowed = true
		access.Plan = append(access.Plan, fmt.Sprintf("Add the Hub address %s to the addresses allowed to use the %s service. Addresses already allowed stay allowed.", sourceIP, access.Service))
	}
	count, err := webFigRuleCount(ctx, runner, tunnel, sourceIP, row.port)
	if err != nil {
		return access, err
	}
	if count == 0 {
		access.addRule = true
		access.Plan = append(access.Plan, fmt.Sprintf("Add one firewall rule: accept TCP port %d only from the Hub address %s, only on the Magic Hub tunnel. WAN and LAN rules do not change.", row.port, sourceIP))
	}
	access.Ready = len(access.Plan) == 0
	return access, nil
}

// webFigSetAllowed writes the source list under whichever name this RouterOS uses.
func webFigSetAllowed(service, list string) string {
	return fmt.Sprintf(`:local s [:pick [/ip/service find where name=%[1]s] 0]; :do {[[:parse (":return [/ip/service set " . $s . " available-from=%[2]s]")]]} on-error={[[:parse (":return [/ip/service set " . $s . " address=%[2]s]")]]}`, routerOSQuoted(service), list)
}

func webFigAddRule(tunnel, sourceIP string, port int) string {
	return fmt.Sprintf(`:local f [/ip/firewall/filter find where dynamic=no]; :if ([:len $f] = 0) do={/ip/firewall/filter add chain=input action=accept in-interface=%[1]s src-address=%[2]s protocol=tcp dst-port=%[3]d comment=%[4]s} else={/ip/firewall/filter add chain=input action=accept in-interface=%[1]s src-address=%[2]s protocol=tcp dst-port=%[3]d comment=%[4]s place-before=[:pick $f 0]}`,
		routerOSQuoted(tunnel), sourceIP+"/32", port, routerOSQuoted(WebFigRuleComment))
}

// WebFigAccessState reads, and changes nothing.
func (a *DeviceApplier) WebFigAccessState(ctx context.Context, businessID, deviceID, tunnel, sourceCIDR string) (WebFigAccess, error) {
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return WebFigAccess{}, err
	}
	return readWebFigAccess(ctx, runner, tunnel, sourceCIDR)
}

func webFigLock(businessID, deviceID string) *sync.Mutex {
	lock, _ := webFigLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// EnableWebFigAccess applies the reviewed plan and reads the router back. A
// failed readback removes the rule and restores the service it changed.
func (a *DeviceApplier) EnableWebFigAccess(ctx context.Context, businessID, deviceID, tunnel, sourceCIDR string) (WebFigAccess, error) {
	mu := webFigLock(businessID, deviceID)
	mu.Lock()
	defer mu.Unlock()
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return WebFigAccess{}, err
	}
	before, err := readWebFigAccess(ctx, runner, tunnel, sourceCIDR)
	if err != nil || before.Ready {
		return before, err
	}
	sourceIP, err := webFigSource(sourceCIDR)
	if err != nil {
		return before, err
	}
	originalList := strings.Join(before.allowList, ",")
	undo := func() {
		if before.addRule {
			_, _ = runner.Run(context.Background(), "/ip/firewall/filter remove [find where comment="+routerOSQuoted(WebFigRuleComment)+"]")
		}
		if before.writeAllowed || before.enableWWW {
			_, _ = runner.Run(context.Background(), webFigSetAllowed(before.Service, originalList))
		}
		if before.enableWWW {
			_, _ = runner.Run(context.Background(), "/ip/service set [:pick [/ip/service find where name=\"www\"] 0] disabled=yes")
		}
	}
	if before.writeAllowed {
		next := strings.Join(append(append([]string{}, before.allowList...), sourceIP+"/32"), ",")
		if _, err := runner.Run(ctx, webFigSetAllowed(before.Service, next)); err != nil {
			undo()
			return before, fmt.Errorf("could not allow the Hub on the router's web service")
		}
	}
	if before.enableWWW {
		if _, err := runner.Run(ctx, webFigSetAllowed("www", sourceIP+"/32")); err != nil {
			undo()
			return before, fmt.Errorf("could not limit the router's web service to the Hub")
		}
		if _, err := runner.Run(ctx, "/ip/service set [:pick [/ip/service find where name=\"www\"] 0] disabled=no"); err != nil {
			undo()
			return before, fmt.Errorf("could not turn on the router's web service")
		}
	}
	if before.addRule {
		if _, err := runner.Run(ctx, webFigAddRule(tunnel, sourceIP, before.Port)); err != nil {
			undo()
			return before, fmt.Errorf("could not add the WebFig firewall rule; nothing was left changed")
		}
	}
	after, err := readWebFigAccess(ctx, runner, tunnel, sourceCIDR)
	if err != nil || !after.Ready {
		undo()
		return before, fmt.Errorf("the router did not confirm the WebFig change, so it was undone")
	}
	return after, nil
}

// DisableWebFigAccess removes the rule and the Hub address. A web service whose
// only allowed address was the Hub must have been turned on for this feature,
// and leaving it on with an empty list would open it to every address, so it is
// turned off instead.
func (a *DeviceApplier) DisableWebFigAccess(ctx context.Context, businessID, deviceID, tunnel, sourceCIDR string) (WebFigAccess, error) {
	mu := webFigLock(businessID, deviceID)
	mu.Lock()
	defer mu.Unlock()
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return WebFigAccess{}, err
	}
	sourceIP, err := webFigSource(sourceCIDR)
	if err != nil {
		return WebFigAccess{}, err
	}
	if _, err := runner.Run(ctx, "/ip/firewall/filter remove [find where comment="+routerOSQuoted(WebFigRuleComment)+"]"); err != nil {
		return WebFigAccess{}, fmt.Errorf("could not remove the WebFig firewall rule")
	}
	out, err := runner.Run(ctx, webFigReadServices)
	if err != nil {
		return WebFigAccess{}, fmt.Errorf("could not read the router's web services")
	}
	for name, row := range parseWebFigServices(out) {
		keep := make([]string, 0, len(row.allowed))
		for _, entry := range row.allowed {
			if entry != sourceIP && entry != sourceIP+"/32" {
				keep = append(keep, entry)
			}
		}
		if len(keep) == len(row.allowed) {
			continue
		}
		if len(keep) == 0 {
			if _, err := runner.Run(ctx, fmt.Sprintf("/ip/service set [:pick [/ip/service find where name=%s] 0] disabled=yes", routerOSQuoted(name))); err != nil {
				return WebFigAccess{}, fmt.Errorf("could not turn off the router's web service")
			}
		}
		if _, err := runner.Run(ctx, webFigSetAllowed(name, strings.Join(keep, ","))); err != nil {
			return WebFigAccess{}, fmt.Errorf("could not remove the Hub from the router's web service")
		}
	}
	return readWebFigAccess(ctx, runner, tunnel, sourceCIDR)
}
