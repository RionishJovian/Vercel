// Package vps builds the WireGuard peer-provisioning surface for a tenant's
// devices: the hub config a router must match to join the mesh, the per-device
// peer block (deterministically derived address in the management subnet), and
// reachability classification. All pure logic — no live-router I/O.
//
// Gate model: tunnel/peer provisioning is a Sovereign-plan benefit. Callers
// gate the handlers on an ACTIVE plan (quota.WebfigPassExpiresAt in the
// future); this package only shapes the payload once gated. It never touches a
// router — the live applier stays behind provisioner.applier.enabled=false.
package vps

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
)

// HubConfig is the WireGuard mesh config a device/role must match to reach the
// hub. Mirrors config.wireguard; surfaced per-tenant for operator visibility.
type HubConfig struct {
	Interface        string   `json:"interface"`
	ListenPort       int      `json:"listen_port"`
	EndpointHost     string   `json:"endpoint_host,omitempty"`
	AllowedIPs       []string `json:"allowed_ips"`
	PeerKeepaliveSec int      `json:"peer_keepalive_sec"`
	ManagementVLAN   int      `json:"management_vlan"`
	ManagementSubnet string   `json:"management_subnet"`
}

// Peer is one device's derived WireGuard client block: its tunnel address
// (stable per device), the peer it connects to (the hub endpoint), and the
// routes it can reach. Deterministic — a re-fetch yields the identical block.
type Peer struct {
	DeviceID            string   `json:"device_id"`
	HardwareName        string   `json:"hardware_name"`
	DeploymentLayer     string   `json:"deployment_layer"`
	Interface           string   `json:"interface"`
	Address             string   `json:"address"`
	AllowedIPs          []string `json:"allowed_ips"`
	Endpoint            string   `json:"endpoint,omitempty"`
	PersistentKeepalive int      `json:"persistent_keepalive_sec"`
}

const (
	// HubAddrHosts is the reserved first N host addresses of the management
	// subnet (hub + infra). Client peer addresses start after these.
	HubAddrHosts = 10
	// keepalive for CGNAT / NAT'd routers so the tunnel stays open.
	defaultKeepalive = 25
)

// BuildHubConfig shapes the hub WireGuard config for response. endpointHost may
// be empty when the operator hasn't pinned a public endpoint (no host in
// config) — endpoints are surfaced only when present.
func BuildHubConfig(iface string, listenPort int, endpointHost string, allowedIPs []string, mgmtVLAN int, mgmtSubnet string, keepaliveSec int) HubConfig {
	if keepaliveSec <= 0 {
		keepaliveSec = defaultKeepalive
	}
	return HubConfig{
		Interface:        iface,
		ListenPort:       listenPort,
		EndpointHost:     endpointHost,
		AllowedIPs:       append([]string{}, allowedIPs...),
		PeerKeepaliveSec: keepaliveSec,
		ManagementVLAN:   mgmtVLAN,
		ManagementSubnet: mgmtSubnet,
	}
}

// BuildPeer derives a deterministic per-device WireGuard client block.
//   - address: host = hash(device_id) mod usable-range, offset past the hub
//     reservations, inside managementSubnet (e.g. 192.168.99.x).
//   - allowed_ips: the hub control address plus the management subnet, so route
//     discovery works while keeping the client scoped.
//   - endpoint: hubHost:hubListenPort (omitted when hubHost is empty).
//
// Deterministic and collision-resistant: same device_id in the same subnet
// always yields the same address.
func BuildPeer(deviceID, hardwareName, deploymentLayer, ifaceID, mgmtSubnet, hubHost string, hubListenPort int, keepaliveSec int) (*Peer, error) {
	_, ipNet, err := net.ParseCIDR(mgmtSubnet)
	if err != nil {
		return nil, fmt.Errorf("parse management subnet %q: %w", mgmtSubnet, err)
	}
	if keepaliveSec <= 0 {
		keepaliveSec = defaultKeepalive
	}

	hostOff := stableHostOffset(deviceID, ipNet, HubAddrHosts)
	addr := ipNet.IP.To4()
	if addr == nil {
		// IPv6 management subnet not supported for peer addressing.
		return nil, fmt.Errorf("management subnet %q is not IPv4", mgmtSubnet)
	}
	// Client addresses start at network + 1 (skip the network itself), then
	// offset past the reserved hub hosts, then plus the device's hash offset.
	// e.g. 192.168.99.0/24 -> first client .11, stable per device.
	host := addTo4(addr, 1+HubAddrHosts+hostOff)

	ep := ""
	if hubHost != "" {
		ep = fmt.Sprintf("%s:%d", hubHost, hubListenPort)
	}
	return &Peer{
		DeviceID:            deviceID,
		HardwareName:        hardwareName,
		DeploymentLayer:     deploymentLayer,
		Interface:           ifaceID,
		Address:             host.String() + "/32",
		AllowedIPs:          []string{addr.String() + "/32", mgmtSubnet},
		Endpoint:            ep,
		PersistentKeepalive: keepaliveSec,
	}, nil
}

// addTo4 returns a copy of the IPv4 address advanced by delta host increments,
// carrying into higher bytes as needed.
func addTo4(ip net.IP, delta int) net.IP {
	b := make([]byte, 4)
	copy(b, ip.To4())
	for i := 3; i >= 0 && delta > 0; i-- {
		add := byte(delta)
		sum := int(b[i]) + int(add)
		b[i] = byte(sum)
		delta >>= 8
		delta += sum / 256 // carry into the next byte
	}
	return net.IP(b)
}

// stableHostOffset maps a device_id to a bounded host offset, deterministic
// (same input => same output) and hash-scaled to the subnet's usable host count
// so it never exceeds the range. counting = usable hosts (network + broadcast +
// reserved excluded); a degenerate /31 or /32 yields 0.
func stableHostOffset(deviceID string, ipNet *net.IPNet, reservedHosts int) int {
	sum := sha256.Sum256([]byte(deviceID))
	ones, bits := ipNet.Mask.Size()
	usableHosts := (1 << (bits - ones)) - 2 - reservedHosts
	if usableHosts < 1 {
		return 0
	}
	digest := binary.BigEndian.Uint32(sum[:4])
	return int(digest % uint32(usableHosts))
}

// Classify maps a monitoring reach state to the peer-health view.
func Classify(state string) string {
	switch state {
	case "up":
		return "reachable"
	case "stale":
		return "degraded"
	case "offline":
		return "unreachable"
	default:
		return "unknown"
	}
}
