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
// LAN, the owner's users or existing firewall policy.
type RouterScripts struct {
	Bootstrap  string `json:"bootstrap_script"`
	Setup      string `json:"router_script"`
	Disconnect string `json:"disconnect_script"`
}

// GenerateRouterKeypair creates a WireGuard keypair for the one-paste setup.
// The private key is returned only to the authenticated operator as part of a
// no-store script response. When durable setup is configured, the server keeps
// it encrypted in the pending-enrollment store until the router connects (at
// most PendingEnrollmentLifetime) and deletes it then; it is never sent to Hub.
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

// TunnelInterfaceName is the Magic Hub WireGuard interface on a router.
func TunnelInterfaceName(deviceID string) (string, error) {
	name, _, err := scriptNames(deviceID)
	return name, err
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
`, name, marker, peer.RouterPublicKey, name, marker, peer.HubPublicKey, host, port, strings.Join(peer.AllowedIPs, ";"), name, marker, peer.TunnelAddress)
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
	setup += "\n# APPLY: add only the missing tunnel resources.\n"
	setup += fmt.Sprintf(`:if ([:len $peer] = 0) do={ /interface/wireguard/peers add interface="%s" public-key="%s" endpoint-address="%s" endpoint-port=%d allowed-address=%s persistent-keepalive=25s comment="%s" }
:if ([:len $addr] = 0) do={ /ip/address add address=%s interface="%s" comment="%s" }
`, name, peer.HubPublicKey, host, port, strings.Join(peer.AllowedIPs, ","), marker, peer.TunnelAddress, name, marker)
	for _, raw := range peer.AllowedIPs {
		setup += fmt.Sprintf(":if ([:len [/ip/route find where dst-address=\"%s\"]] = 0) do={ /ip/route add dst-address=%s gateway=\"%s\" comment=\"%s\" }\n", raw, raw, name, marker)
	}
	setup += fmt.Sprintf(":put \"Tunnel configured. Return to the app and Check Magic Hub. A handshake is not management authentication.\"\n/interface/wireguard/peers print where interface=\"%s\"\n}", name)
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

// BuildQuickConnectScript builds the one script a person pastes in WinBox. The
// private key appears only in this authenticated, no-store response; until the
// router connects, the application keeps it encrypted in the pending-enrollment
// store (at most PendingEnrollmentLifetime) so a late paste still works.
//
// It may create missing resources or re-enable exact matches. Existing
// tunnels, peers, routes and addresses are never removed or reassigned.
// A router that holds a key from an earlier attempt stops before the first
// write; recovery is either BuildResumeScript (keep that key) or the explicit
// BuildStartOverScript.
func BuildQuickConnectScript(deviceID string, peer Peer, privateKey string) (string, error) {
	return buildQuickConnect(deviceID, peer, privateKey, false)
}

// BuildStartOverScript is the explicit, owner-confirmed recovery for a router
// left with this gateway's tunnel from an earlier, abandoned attempt. It removes
// only resources carrying this gateway's exact ownership marker (its own
// interface, peers, address and routes), installs the new key on the same
// interface and continues as a normal setup. Anything else still stops it.
func BuildStartOverScript(deviceID string, peer Peer, privateKey string) (string, error) {
	return buildQuickConnect(deviceID, peer, privateKey, true)
}

// BuildResumeScript continues an enrollment whose setup key is no longer held
// by the application: it uses the key already on the router, never a new one.
// It stops before any write unless the router's own Magic Hub interface exists
// and its public key equals the Hub allocation.
func BuildResumeScript(deviceID string, peer Peer) (string, error) {
	scripts, err := BuildRouterScripts(deviceID, peer)
	if err != nil {
		return "", err
	}
	s := scripts.Setup
	for _, r := range [][2]string{
		{"# Magic Hub step 2. Review, then paste the entire block in WinBox New Terminal.\n# Scoped tunnel only. Does not enable SSH/REST or change WAN/LAN/firewall/users.\n",
			"# Magic Hub resume. Uses the tunnel key this router already holds from the earlier setup.\n"},
		{`:error "STOP: run step 1 first"`, `:error "STOP: this router does not hold the key from the earlier setup. In the app, choose Start over; nothing changed"`},
		{`:error "STOP: router key differs from enrollment; no replacement performed"`, `:error "STOP: this router holds a different Magic Hub key. In the app, choose Start over; nothing changed"`},
	} {
		if strings.Count(s, r[0]) != 1 {
			return "", fmt.Errorf("resume script template changed")
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	return s, nil
}

func buildQuickConnect(deviceID string, peer Peer, privateKey string, startOver bool) (string, error) {
	if !scriptKey(privateKey) {
		return "", fmt.Errorf("invalid router private key")
	}
	private, _ := base64.StdEncoding.DecodeString(privateKey)
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil || base64.StdEncoding.EncodeToString(public) != peer.RouterPublicKey {
		return "", fmt.Errorf("router private key does not match allocation")
	}
	// Validates every value placed in the script.
	if _, err := BuildRouterScripts(deviceID, peer); err != nil {
		return "", err
	}
	name, marker, err := scriptNames(deviceID)
	if err != nil {
		return "", err
	}
	host, portRaw, err := net.SplitHostPort(peer.Endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid Hub endpoint")
	}
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	// In start-over mode a difference is tolerated only for a resource that
	// carries this gateway's exact marker; it is removed during APPLY.
	differs := func(stop string) string {
		if startOver {
			return `:if ($mmOwned != true) do={ :error "` + stop + `" }`
		}
		return `:error "` + stop + `"`
	}

	if startOver {
		w(`{
# Magic Hub START OVER for this gateway. Paste the entire block in WinBox New Terminal.
# Replaces only this gateway's own tunnel (%[1]s): its key, peer, address and routes marked
# "%[2]s". Other tunnels, WAN/LAN and users are not changed. Run it from the LAN, not over the tunnel.
`, name, marker)
	} else {
		w(`{
# Magic Hub one-paste setup. Paste the entire block in WinBox New Terminal.
# Creates missing tunnel resources only. Does not remove prior tunnels or change WAN/LAN/users.
# A different enrollment requires an explicit Start over, NOT an automatic cleanup.
`)
	}
	w(`
# PREFLIGHT: inspect all existing Magic Hub identities and destinations before any writes.
:local mmOwned true
:foreach mmItem in=[/interface/wireguard find where name~"^mmh-"] do={
  :local mmName [/interface/wireguard get $mmItem name]
  :local mmComment [/interface/wireguard get $mmItem comment]
  :if ($mmName != "%[1]s" && [:pick $mmComment 0 15] = "mega-magic:mmh-") do={
    :error "STOP: older Magic Hub tunnel found. Migrate separately; nothing changed"
  }
}
:local mmIface [/interface/wireguard find where name="%[1]s"]
:if ([:len $mmIface] > 1) do={ :error "STOP: duplicate tunnel interface; nothing changed" }
:if ([:len $mmIface] = 1) do={
  :if ([/interface/wireguard get $mmIface comment] != "%[2]s") do={ :error "STOP: interface belongs to another setup; nothing changed" }
  :if ([/interface/wireguard get $mmIface public-key] != "%[5]s") do={ %[10]s }
}
:local mmPeer [/interface/wireguard/peers find where interface="%[1]s"]
%[11]s:foreach mmItem in=$mmPeer do={
  :if ([/interface/wireguard/peers get $mmItem comment] != "%[2]s") do={ :error "STOP: a peer on the Magic Hub interface belongs to another setup; nothing changed" }
  :if ([/interface/wireguard/peers get $mmItem public-key] != "%[6]s" ||
       [/interface/wireguard/peers get $mmItem endpoint-address] != "%[7]s" ||
       [:tostr [/interface/wireguard/peers get $mmItem endpoint-port]] != "%[8]s" ||
       [:tostr [/interface/wireguard/peers get $mmItem allowed-address]] != "%[9]s") do={
    %[12]s
  }
}
:foreach mmItem in=[/ip/address find where interface="%[1]s"] do={
  :if ([/ip/address get $mmItem comment] != "%[2]s") do={ :error "STOP: an address on the Magic Hub interface belongs to another setup; nothing changed" }
  :if ([/ip/address get $mmItem address] != "%[3]s") do={ %[13]s }
}
:foreach mmItem in=[/ip/address find where address="%[3]s"] do={
  :if ([/ip/address get $mmItem interface] != "%[1]s" ||
       [/ip/address get $mmItem comment] != "%[2]s") do={
    :error "STOP: tunnel address already assigned elsewhere; nothing changed"
  }
}
:foreach mmItem in=[/ip/route find where comment~"^mega-magic:mmh-"] do={
  :if ([/ip/route get $mmItem comment] != "%[2]s") do={
    :error "STOP: older Magic Hub route found. Migrate separately; nothing changed"
  }
}
`, name, marker, peer.TunnelAddress, privateKey, peer.RouterPublicKey, peer.HubPublicKey, host, portRaw, strings.Join(peer.AllowedIPs, ";"),
		differs("STOP: this router holds a key from an earlier setup. In the app, choose Start over; nothing changed"),
		map[bool]string{false: ":if ([:len $mmPeer] > 1) do={ :error \"STOP: multiple existing peers; nothing changed\" }\n", true: ""}[startOver],
		differs("STOP: existing Hub peer differs; migration required; nothing changed"),
		differs("STOP: existing interface address differs; nothing changed"))
	for _, raw := range peer.AllowedIPs {
		w(`:local mmRoute [/ip/route find where dst-address="%[1]s"]
:if ([:len $mmRoute] > 1) do={ :error "STOP: duplicate Hub routes; nothing changed" }
:if ([:len $mmRoute] = 1) do={
  :if ([/ip/route get $mmRoute comment] != "%[2]s") do={ :error "STOP: Hub destination route belongs to another setup; nothing changed" }
  :if ([/ip/route get $mmRoute gateway] != "%[3]s") do={ %[4]s }
}
`, raw, marker, name, differs("STOP: Hub destination route differs; nothing changed"))
	}
	w(`
# APPLY: only add absent resources or enable the matching enrollment.
`)
	if startOver {
		w(`:if ([:len $mmIface] = 1) do={
  /interface/wireguard/peers remove [find where interface="%[1]s" and comment="%[2]s"]
  /ip/address remove [find where interface="%[1]s" and comment="%[2]s"]
  /ip/route remove [find where comment="%[2]s"]
  /interface/wireguard set $mmIface private-key="%[3]s"
  :if ([/interface/wireguard get $mmIface public-key] != "%[4]s") do={ :error "STOP: new key could not be verified; Safe Mode Ctrl+D undoes this paste" }
  :set mmPeer [/interface/wireguard/peers find where interface="%[1]s"]
}
`, name, marker, privateKey, peer.RouterPublicKey)
	}
	w(`:if ([:len $mmIface] = 0) do={
  /interface/wireguard add name="%[1]s" mtu=1280 private-key="%[3]s" comment="%[2]s"
  :set mmIface [/interface/wireguard find where name="%[1]s"]
}
:if ([:len $mmIface] != 1) do={ :error "STOP: tunnel creation could not be verified" }
/interface/wireguard set $mmIface disabled=no
:if ([:len $mmPeer] = 0) do={
  /interface/wireguard/peers add interface="%[1]s" public-key="%[4]s" endpoint-address="%[5]s" endpoint-port=%[6]s allowed-address=%[7]s persistent-keepalive=25s comment="%[2]s"
} else={ /interface/wireguard/peers set $mmPeer disabled=no }
:local mmAddr [/ip/address find where interface="%[1]s" and address="%[8]s"]
:if ([:len $mmAddr] = 0) do={
  /ip/address add address=%[8]s interface="%[1]s" comment="%[2]s"
} else={ /ip/address set $mmAddr disabled=no }
`, name, marker, privateKey, peer.HubPublicKey, host, portRaw, strings.Join(peer.AllowedIPs, ","), peer.TunnelAddress)
	for _, raw := range peer.AllowedIPs {
		w(`:if ([:len [/ip/route find where dst-address="%[1]s"]] = 0) do={
  /ip/route add dst-address=%[1]s gateway="%[2]s" comment="%[3]s"
} else={ /ip/route set [find where dst-address="%[1]s"] disabled=no }
`, raw, name, marker)
	}
	w(":put \"Tunnel configured.\"\n}")
	return b.String(), nil
}

// ManagementUserName is the dedicated RouterOS account (and group) that one-paste
// setup creates for a router. Setup and disconnect must derive it identically.
func ManagementUserName(businessID, deviceID string) string {
	hash := sha256.Sum256([]byte(businessID + "/" + deviceID))
	return fmt.Sprintf("mm-%x", hash[:6])
}

var managementUserName = regexp.MustCompile(`^mm-[0-9a-f]{12}$`)

// BuildDisconnectScript removes everything one-paste setup put on a router: the
// management firewall rule, SSH key, user and group (only when their ownership
// marker matches), then the tunnel and its key. SSH service settings are not
// touched because the router's original state is unknown. Safe to paste again.
func BuildDisconnectScript(deviceID string, peer Peer, managementUser string) (string, error) {
	if _, err := BuildRouterScripts(deviceID, peer); err != nil || !managementUserName.MatchString(managementUser) {
		return "", fmt.Errorf("invalid disconnect request")
	}
	marker := "mega-magic:" + managementUser
	name, tunnelMarker, err := scriptNames(deviceID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{
# Removes this router's Magic Hub setup: its management user, group, SSH key and firewall
# rules, then its tunnel. SSH service settings are left as they are. Safe to paste again.
/ip/firewall/filter remove [find where comment="%[1]s"]
/ip/firewall/filter remove [find where comment="mega-magic:webfig"]
/ip/firewall/nat remove [find where comment="%[1]s"]
:local mmAccount [/user find where name="%[2]s"]
:if ([:len $mmAccount] = 1) do={
  :if ([:pick [/user get $mmAccount comment] 0 %[3]d] = "%[1]s:") do={
    /user/ssh-keys remove [find where user="%[2]s"]
    /user remove $mmAccount
    :put "Management user and SSH key removed."
  } else={ :put "A user named %[2]s exists but is not from Magic Hub; left untouched." }
}
:local mmGroup [/user/group find where name="%[2]s"]
:if ([:len $mmGroup] = 1) do={ :do { /user/group remove $mmGroup } on-error={ :put "Management group is still in use; left in place." } }
/ip/route remove [find where comment="%[5]s"]
/ip/address remove [find where comment="%[5]s"]
/interface/wireguard/peers remove [find where interface="%[4]s" and comment="%[5]s"]
/interface/wireguard remove [find where name="%[4]s" and comment="%[5]s"]
:put "Magic Hub setup removed from this router."
}`, marker, managementUser, len(marker)+1, name, tunnelMarker), nil
}
