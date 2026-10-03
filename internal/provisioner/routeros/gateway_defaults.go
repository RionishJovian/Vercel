package routeros

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
)

var gatewayRoutePattern = regexp.MustCompile(`(?m)^\s*dst-address:\s*(\S+)`)

// RouterOS 7 can render value-list addresses in columns and routes in detail
// key=value format. Scalar get output avoids both ambiguous presentations.
const gatewayAddressesCommand = `:foreach id in=[/ip address find] do={ :put ("address: " . [/ip address get $id address]) }`
const gatewayRoutesCommand = `:foreach id in=[/ip route find] do={ :put ("dst-address: " . [/ip route get $id dst-address]) }`

// Read both assigned and routed networks. Default routes do not reserve every
// private subnet. All other routes are conservatively reserved, even inactive ones.
func gatewayReservedNetworks(ctx context.Context, r CommandRunner) ([]*net.IPNet, error) {
	var networks []*net.IPNet
	for _, source := range []struct {
		command       string
		pattern       *regexp.Regexp
		ignoreDefault bool
	}{{gatewayAddressesCommand, gatewayAddressPattern, false}, {gatewayRoutesCommand, gatewayRoutePattern, true}} {
		out, err := r.Run(ctx, source.command)
		if err != nil {
			return nil, fmt.Errorf("could not read existing addresses and routes")
		}
		matches := source.pattern.FindAllStringSubmatch(out, -1)
		lines := strings.FieldsFunc(out, func(c rune) bool { return c == '\r' || c == '\n' })
		if len(lines) != len(matches) {
			return nil, fmt.Errorf("could not parse existing addresses and routes")
		}
		for _, match := range matches {
			_, network, err := net.ParseCIDR(match[1])
			if err != nil || network.IP.To4() == nil {
				return nil, fmt.Errorf("could not parse existing IPv4 network")
			}
			ones, _ := network.Mask.Size()
			if ones != 0 || !source.ignoreDefault {
				networks = append(networks, network)
			}
		}
	}
	return networks, nil
}

func gatewayNetworkOverlaps(network *net.IPNet, reserved []*net.IPNet) bool {
	for _, other := range reserved {
		if network.Contains(other.IP) || other.Contains(network.IP) {
			return true
		}
	}
	return false
}

// GatewayDefaults suggests addresses only; it never guesses physical port roles
// or writes to the router. Preview and apply recheck the live inventory.
func (a *DeviceApplier) GatewayDefaults(ctx context.Context, businessID, deviceID string) (GatewaySetup, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return GatewaySetup{}, err
	}
	reserved, err := gatewayReservedNetworks(ctx, r)
	if err != nil {
		return GatewaySetup{}, err
	}
	// Prefer familiar private addresses, with bounded alternatives. Never use
	// the Magic Hub's 10.77 management range.
	for i := 0; i < 456; i++ {
		prefix := fmt.Sprintf("192.168.%d", (50+i)%256)
		if i >= 256 {
			prefix = fmt.Sprintf("10.200.%d", i-256)
		}
		_, network, _ := net.ParseCIDR(prefix + ".0/24")
		if !gatewayNetworkOverlaps(network, reserved) {
			return GatewaySetup{Name: "guest-lan", Address: prefix + ".1/24", PoolStart: prefix + ".10", PoolEnd: prefix + ".250", DNS: []string{"1.1.1.1", "8.8.8.8"}, Hotspot: true}, nil
		}
	}
	return GatewaySetup{}, fmt.Errorf("no non-overlapping suggested subnet is available; ask your network administrator")
}
