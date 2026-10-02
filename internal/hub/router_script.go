package hub

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// Scripts keep the WireGuard private key on the router. They never reset WAN,
// LAN, users, management services or existing firewall policy.
type RouterScripts struct {
	Bootstrap  string `json:"bootstrap_script"`
	Setup      string `json:"router_script"`
	Disconnect string `json:"disconnect_script"`
}

func scriptNames(deviceID string) (string, string, error) {
	if !identifier.MatchString(deviceID) {
		return "", "", fmt.Errorf("invalid gateway identity")
	}
	hash := sha256.Sum256([]byte(deviceID))
	name := fmt.Sprintf("mmh-%x", hash[:6])
	return name, "mega-magic:" + name, nil
}

func BootstrapScript(deviceID string) (string, error) {
	name, marker, err := scriptNames(deviceID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{
# Magic Hub step 1. RouterOS 7. Paste the entire block in WinBox New Terminal.
# Creates only a dedicated WireGuard interface. Never prints or uploads its private key.
:local iface [/interface/wireguard find where name="%s"]
:if ([:len $iface] = 0) do={
  /interface/wireguard add name="%s" mtu=1280 comment="%s"
  :set iface [/interface/wireguard find where name="%s"]
}
:if ([:len $iface] != 1) do={ :error "STOP: ambiguous Magic Hub interface" }
:if ([/interface/wireguard get $iface comment] != "%s") do={ :error "STOP: interface belongs to another setup" }
:put "Copy only this PUBLIC key into the app:"
:put [/interface/wireguard get $iface public-key]
}`, name, name, marker, name, marker), nil
}

var endpointName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

func scriptKey(value string) bool {
	b, err := base64.StdEncoding.DecodeString(value)
	return err == nil && len(b) == 32 && base64.StdEncoding.EncodeToString(b) == value && strings.Trim(value, "A=") != ""
}

// BuildRouterScripts accepts only the authenticated Hub allocation. Every value
// interpolated into RouterOS is tightly validated rather than shell-escaped.
func BuildRouterScripts(deviceID string, peer Peer) (RouterScripts, error) {
	name, marker, err := scriptNames(deviceID)
	if err != nil || peer.RouterID != deviceID || !scriptKey(peer.RouterPublicKey) || !scriptKey(peer.HubPublicKey) || peer.EnrollmentVersion != 1 || peer.State != "active" {
		return RouterScripts{}, fmt.Errorf("invalid Hub script allocation")
	}
	address, err := netip.ParsePrefix(peer.TunnelAddress)
	if err != nil || !address.Addr().Is4() || !address.Addr().IsPrivate() || address.Bits() != 32 {
		return RouterScripts{}, fmt.Errorf("invalid tunnel address")
	}
	host, portRaw, err := net.SplitHostPort(peer.Endpoint)
	port, portErr := strconv.Atoi(portRaw)
	if err != nil || portErr != nil || port < 1 || port > 65535 || !endpointName.MatchString(host) || strings.Contains(host, "..") || len(peer.AllowedIPs) == 0 || len(peer.AllowedIPs) > 8 {
		return RouterScripts{}, fmt.Errorf("invalid Hub endpoint or routes")
	}
	if endpointIP, ipErr := netip.ParseAddr(host); ipErr == nil && (!endpointIP.IsGlobalUnicast() || endpointIP.IsLoopback() || endpointIP.IsLinkLocalUnicast()) {
		return RouterScripts{}, fmt.Errorf("invalid Hub endpoint address")
	}
	seen := map[string]bool{}
	for _, raw := range peer.AllowedIPs {
		p, e := netip.ParsePrefix(raw)
		if e != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() || p.Bits() != 32 || p.Addr() == address.Addr() || seen[raw] {
			return RouterScripts{}, fmt.Errorf("invalid scoped Hub route")
		}
		seen[raw] = true
	}
	bootstrap, _ := BootstrapScript(deviceID)
	setup := fmt.Sprintf(`{
# Magic Hub step 2. Review, then paste the entire block in WinBox New Terminal.
# Scoped tunnel only. Does not enable SSH/REST or change WAN/LAN/firewall/users.
:local iface [/interface/wireguard find where name="%s"]
:if ([:len $iface] != 1) do={ :error "STOP: run step 1 first" }
:if ([/interface/wireguard get $iface comment] != "%s") do={ :error "STOP: interface ownership mismatch" }
:if ([/interface/wireguard get $iface disabled]) do={ :error "STOP: interface disabled; inspect in WinBox" }
:if ([/interface/wireguard get $iface public-key] != "%s") do={ :error "STOP: router key differs from enrollment; no replacement performed" }
:local peer [/interface/wireguard/peers find where interface="%s"]
:if ([:len $peer] > 1) do={ :error "STOP: unexpected peers on dedicated interface" }
:if ([:len $peer] = 1) do={
  :if ([/interface/wireguard/peers get $peer comment] != "%s" || [/interface/wireguard/peers get $peer public-key] != "%s") do={ :error "STOP: existing peer mismatch" }
  :if ([/interface/wireguard/peers get $peer endpoint-address] != "%s" || [/interface/wireguard/peers get $peer endpoint-port] != %d) do={ :error "STOP: existing endpoint differs; inspect configuration" }
  :if ([/interface/wireguard/peers get $peer disabled]) do={ :error "STOP: existing Hub peer disabled" }
  :if ([:tostr [/interface/wireguard/peers get $peer allowed-address]] != "%s") do={ :error "STOP: existing allowed addresses differ" }
}
:local addr [/ip/address find where interface="%s"]
:if ([:len $addr] > 1) do={ :error "STOP: unexpected interface addresses" }
:if ([:len $addr] = 1) do={
  :if ([/ip/address get $addr comment] != "%s" || [/ip/address get $addr address] != "%s") do={ :error "STOP: existing address mismatch" }
  :if ([/ip/address get $addr disabled]) do={ :error "STOP: existing tunnel address disabled" }
}
`, name, marker, peer.RouterPublicKey, name, marker, peer.HubPublicKey, host, port, strings.Join(peer.AllowedIPs, ","), name, marker, peer.TunnelAddress)
	setup += fmt.Sprintf(`:local assigned [/ip/address find where address="%s"]
:if ([:len $assigned] > 1) do={ :error "STOP: duplicate tunnel address" }
:if ([:len $assigned] = 1) do={
  :if ([/ip/address get $assigned interface] != "%s") do={ :error "STOP: tunnel address already used by another interface" }
}
`, peer.TunnelAddress, name)
	// Validate every existing route before any writes. Re-paste recovers partial
	// installs without removing or replacing resources.
	for index, raw := range peer.AllowedIPs {
		routeVar := fmt.Sprintf("route%d", index)
		setup += fmt.Sprintf(`:local %s [/ip/route find where dst-address="%s"]
:if ([:len $%s] > 1) do={ :error "STOP: ambiguous Hub route" }
:if ([:len $%s] = 1) do={
  :if ([/ip/route get $%s comment] != "%s" || [/ip/route get $%s gateway] != "%s") do={ :error "STOP: conflicting Hub route" }
  :if ([/ip/route get $%s disabled]) do={ :error "STOP: existing Hub route disabled" }
}
`, routeVar, raw, routeVar, routeVar, routeVar, marker, routeVar, name, routeVar)
	}
	setup += fmt.Sprintf(`:if ([:len $peer] = 0) do={ /interface/wireguard/peers add interface="%s" public-key="%s" endpoint-address="%s" endpoint-port=%d allowed-address=%s persistent-keepalive=25s comment="%s" }
:if ([:len $addr] = 0) do={ /ip/address add address=%s interface="%s" comment="%s" }
`, name, peer.HubPublicKey, host, port, strings.Join(peer.AllowedIPs, ","), marker, peer.TunnelAddress, name, marker)
	for _, raw := range peer.AllowedIPs {
		setup += fmt.Sprintf(":if ([:len [/ip/route find where dst-address=\"%s\"]] = 0) do={ /ip/route add dst-address=%s gateway=\"%s\" comment=\"%s\" }\n", raw, raw, name, marker)
	}
	setup += fmt.Sprintf(":put \"Tunnel configured. Return to the app and Check Magic Hub. A handshake is not management authentication.\"\n/interface/wireguard/peers print where interface=\"%s\"\n}", name)
	disconnect := fmt.Sprintf(`{
# Disconnect only this enrollment. Keeps the router's interface/private key for reconnection.
:local iface [/interface/wireguard find where name="%s"]
:if ([:len $iface] != 1) do={ :error "STOP: interface missing or ambiguous" }
:if ([/interface/wireguard get $iface comment] != "%s" || [/interface/wireguard get $iface public-key] != "%s") do={ :error "STOP: enrollment ownership mismatch" }
/interface/wireguard/peers remove [find where interface="%s" and comment="%s" and public-key="%s"]
/ip/route remove [find where gateway="%s" and comment="%s"]
/ip/address remove [find where interface="%s" and comment="%s" and address="%s"]
:put "Tunnel disconnected locally. Hub allocation and router key preserved."
}`, name, marker, peer.RouterPublicKey, name, marker, peer.HubPublicKey, name, marker, name, marker, peer.TunnelAddress)
	return RouterScripts{Bootstrap: bootstrap, Setup: setup, Disconnect: disconnect}, nil
}
