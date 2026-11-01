package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// Scripts keep the WireGuard private key on the router. They never reset WAN,
// LAN, users, management services or existing firewall policy.
type RouterScripts struct {
	Bootstrap  string `json:"bootstrap_script"`
	Setup      string `json:"router_script"`
	Disconnect string `json:"disconnect_script"`
}

// GenerateRouterKeypair creates a WireGuard keypair for the one-paste setup.
// The private key is returned only to the authenticated operator as part of a
// no-store script response; it is never persisted by the application.
func GenerateRouterKeypair() (privateKey, publicKey string, err error) {
	private := make([]byte, 32)
	if _, err := rand.Read(private); err != nil {
		return "", "", err
	}
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(private), base64.StdEncoding.EncodeToString(public), nil
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

// Header lines other code recognises when it rewrites a tunnel script's claims.
const (
	TunnelScopeNote = "# Scoped tunnel only. Does not enable SSH/REST or change WAN/LAN/firewall/users."
	QuickScopeNote  = "# Checks this router's internet path first (changes nothing if it fails), turns on the NTP clock client\n# if it is off, then creates the Magic Hub tunnel. WAN, LAN, firewall and users are not changed.\n# Do not save this into System > Scripts: it contains this router's private key."
)

// preflightBlock checks, before anything is changed, that the router has an
// internet path. Only definitive configuration errors stop the paste; a failed
// ping only warns because some networks drop ICMP while WireGuard still works.
const preflightBlock = `# Pre-flight (changes nothing): the tunnel needs an internet path from this router.
:local problems 0
:if ([:len [/ip/route find where dst-address="0.0.0.0/0"]] = 0) do={ :put "STOP: this router has no default route, so it has no internet path."; :set problems ($problems + 1) }
:local dup ""
:foreach a in=[/ip/address find where !disabled] do={
  :local addr [/ip/address get $a address]
  :if ([:len [/ip/address find where address=$addr and !disabled]] > 1 && $addr != $dup) do={ :set dup $addr; :put ("STOP: the same address is on more than one interface: " . $addr); :set problems ($problems + 1) }
}
:local replies 1
:do { :set replies [/ping 8.8.8.8 count=3] } on-error={}
:if ($replies = 0) do={ :put "WARNING: this router got no reply from 8.8.8.8. Check its internet access; the tunnel may not connect." }
:if ($problems > 0) do={ :error "Magic Hub NOT installed. Fix the STOP lines above, then paste again." }
`

// clockPrepBlock turns on the NTP client only when it is off: voucher first-use
// and expiry stamps need a correct router clock. The time zone is not changed.
const clockPrepBlock = `# Clock: voucher first-use and expiry need a correct router clock.
:if (![/system/ntp/client get enabled]) do={
  :put "NTP clock client was off; turning it on."
  :do { /system/ntp/client set enabled=yes servers=time.cloudflare.com,time.google.com } on-error={
    :do { /system/ntp/client set enabled=yes } on-error={}
    :do { /system/ntp/client/servers add address=time.cloudflare.com } on-error={}
    :do { /system/ntp/client/servers add address=time.google.com } on-error={}
  }
}
`

// clockReportBlock runs after the handshake wait, giving NTP time to correct a
// clock that started wrong. 1735689600 is 2025-01-01, before this feature existed.
const clockReportBlock = `:local clockNow ([:tonsec [:timestamp]] / 1000000000)
:put ("Router clock: " . [/system/clock get date] . " " . [/system/clock get time])
:if ($clockNow < 1735689600) do={ :put "WARNING: the router clock still looks wrong. Vouchers need a correct clock; NTP needs internet and a few minutes to correct it." }
`

// handshakeWaitBlock tells the operator whether the Hub answered, instead of
// printing a peer that cannot have a handshake yet.
func handshakeWaitBlock(name, host string, port int) string {
	return fmt.Sprintf(`:put "Waiting up to 12 seconds for the Hub to answer..."
:local waited 0
:local shaken false
:while ($waited < 12 && !$shaken) do={
  :delay 1s
  :set waited ($waited + 1)
  :do { :if ([:typeof [/interface/wireguard/peers get [find where interface="%s"] last-handshake]] = "time") do={ :set shaken true } } on-error={}
}
:if ($shaken) do={ :put "Hub handshake OK: the tunnel is up." } else={ :put "No Hub handshake yet. Check this router's internet access and that UDP to %s:%d is not blocked, then use Check Magic Hub in the app." }
`, name, host, port)
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
%s
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
`, TunnelScopeNote, name, marker, peer.RouterPublicKey, name, marker, peer.HubPublicKey, host, port, strings.Join(peer.AllowedIPs, ";"), name, marker, peer.TunnelAddress)
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
	setup += ":put \"Tunnel configured. Return to the app and Check Magic Hub. A handshake is not management authentication.\"\n"
	setup += handshakeWaitBlock(name, host, port)
	setup += fmt.Sprintf("/interface/wireguard/peers print where interface=\"%s\"\n}", name)
	disconnect := fmt.Sprintf(`{
# Disconnect the tunnel of this enrollment. Keeps the router's interface/private key for reconnection.
# A management user (mm-...), its group, SSH key and firewall rule from the one-paste setup are NOT removed;
# remove them in WinBox if you are retiring this router.
:local iface [/interface/wireguard find where name="%s"]
:if ([:len $iface] != 1) do={ :error "STOP: interface missing or ambiguous" }
:if ([/interface/wireguard get $iface comment] != "%s" || [/interface/wireguard get $iface public-key] != "%s") do={ :error "STOP: enrollment ownership mismatch" }
/interface/wireguard/peers remove [find where interface="%s" and comment="%s" and public-key="%s"]
/ip/route remove [find where gateway="%s" and comment="%s"]
/ip/address remove [find where interface="%s" and comment="%s" and address="%s"]
:put "Tunnel disconnected locally. Hub allocation and router key preserved. Management user, SSH key and firewall rule were not removed."
}`, name, marker, peer.RouterPublicKey, name, marker, peer.HubPublicKey, name, marker, name, marker, peer.TunnelAddress)
	return RouterScripts{Bootstrap: bootstrap, Setup: setup, Disconnect: disconnect}, nil
}

// BuildQuickConnectScript preserves the original one-paste operator workflow
// while keeping the private key out of the Hub and database. The key is shown
// only in this authenticated, no-store response and is installed locally by
// the router. The normal ownership and route checks remain in the setup block.
func BuildQuickConnectScript(deviceID string, peer Peer, privateKey string) (string, error) {
	if !scriptKey(privateKey) {
		return "", fmt.Errorf("invalid router private key")
	}
	private, _ := base64.StdEncoding.DecodeString(privateKey)
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil || base64.StdEncoding.EncodeToString(public) != peer.RouterPublicKey {
		return "", fmt.Errorf("router private key does not match allocation")
	}
	scripts, err := BuildRouterScripts(deviceID, peer)
	if err != nil {
		return "", err
	}
	name, marker, err := scriptNames(deviceID)
	if err != nil {
		return "", err
	}
	insert := fmt.Sprintf(`
# One-paste Magic Hub setup. The private key is installed only on this router.
:local existing [/interface/wireguard find where name="%s"]
:if ([:len $existing] = 0) do={
  /interface/wireguard add name="%s" mtu=1280 private-key="%s" comment="%s"
}
`, name, name, privateKey, marker)
	idx := strings.Index(scripts.Setup, ":local iface")
	if idx < 0 {
		return "", fmt.Errorf("invalid generated setup script")
	}
	setup := scripts.Setup[:idx] + preflightBlock + clockPrepBlock + insert + scripts.Setup[idx:]
	setup = strings.Replace(setup, "# Magic Hub step 2. Review, then paste the entire block in WinBox New Terminal.", "# Magic Hub one-paste setup. Paste the entire block in WinBox New Terminal.", 1)
	setup = strings.Replace(setup, "STOP: run step 1 first", "STOP: Magic Hub interface creation failed", 1)
	setup = strings.Replace(setup, TunnelScopeNote, QuickScopeNote, 1)
	// Report the clock after the handshake wait, when NTP has had time to correct it.
	finalPrint := "/interface/wireguard/peers print where interface=\"" + name + "\"\n}"
	if !strings.HasSuffix(setup, finalPrint) {
		return "", fmt.Errorf("invalid generated setup script")
	}
	setup = strings.TrimSuffix(setup, finalPrint) + clockReportBlock + finalPrint
	return setup, nil
}

// ManagementUserName is the dedicated RouterOS account (and group) that one-paste
// setup creates for a router. Setup and disconnect must derive it identically.
func ManagementUserName(businessID, deviceID string) string {
	hash := sha256.Sum256([]byte(businessID + "/" + deviceID))
	return fmt.Sprintf("mm-%x", hash[:6])
}

var managementUserName = regexp.MustCompile(`^mm-[0-9a-f]{12}$`)

// BuildDisconnectScript retires an enrollment completely: it removes the
// management firewall rule, SSH key, user and group created by one-paste setup
// (only when their ownership marker matches), then the tunnel. The router's
// WireGuard interface and private key are kept for reconnection. SSH service
// settings are not touched because the router's original state is unknown.
func BuildDisconnectScript(deviceID string, peer Peer, managementUser string) (string, error) {
	if _, err := BuildRouterScripts(deviceID, peer); err != nil || !managementUserName.MatchString(managementUser) {
		return "", fmt.Errorf("invalid disconnect request")
	}
	marker := "mega-magic:" + managementUser
	cleanup := fmt.Sprintf(`# Retire this Magic Hub enrollment: removes its management user, group, SSH key and
# firewall rule, then its tunnel. The router's WireGuard interface and private key are kept,
# and SSH service settings are left as they are. Safe to paste more than once.
:local rule [/ip/firewall/filter find where comment="%s" and chain="input"]
:if ([:len $rule] > 0) do={ /ip/firewall/filter remove $rule; :put "Management firewall rule removed." }
:local account [/user find where name="%s"]
:if ([:len $account] > 1) do={ :error "STOP: ambiguous management account" }
:if ([:len $account] = 1) do={
  :if ([:pick [/user get $account comment] 0 %d] = "%s:") do={
    :local keys [/user/ssh-keys find where user="%s"]
    :if ([:len $keys] > 0) do={ /user/ssh-keys remove $keys }
    /user remove $account
    :put "Management user and SSH key removed."
  } else={ :put "A user named %s exists but is not owned by Magic Hub; left untouched." }
}
:local grp [/user/group find where name="%s"]
:if ([:len $grp] = 1) do={ :do { /user/group remove $grp } on-error={ :put "Management group is still in use; left in place." } }
`, marker, managementUser, len(marker)+1, marker, managementUser, managementUser, managementUser)
	// Guard the tunnel removals so a repeat paste on a partly cleaned router succeeds.
	name, tunnelMarker, err := scriptNames(deviceID)
	if err != nil {
		return "", err
	}
	tunnel := fmt.Sprintf(`:local iface [/interface/wireguard find where name="%s"]
:if ([:len $iface] != 1) do={ :error "STOP: interface missing or ambiguous" }
:if ([/interface/wireguard get $iface comment] != "%s" || [/interface/wireguard get $iface public-key] != "%s") do={ :error "STOP: enrollment ownership mismatch" }
:local peers [/interface/wireguard/peers find where interface="%s" and comment="%s" and public-key="%s"]
:if ([:len $peers] > 0) do={ /interface/wireguard/peers remove $peers }
:local routes [/ip/route find where gateway="%s" and comment="%s"]
:if ([:len $routes] > 0) do={ /ip/route remove $routes }
:local addrs [/ip/address find where interface="%s" and comment="%s" and address="%s"]
:if ([:len $addrs] > 0) do={ /ip/address remove $addrs }
:put "Magic Hub tunnel and management access removed. The router's WireGuard key was kept."
}`, name, tunnelMarker, peer.RouterPublicKey, name, tunnelMarker, peer.HubPublicKey, name, tunnelMarker, name, tunnelMarker, peer.TunnelAddress)
	return "{\n" + cleanup + tunnel, nil
}
