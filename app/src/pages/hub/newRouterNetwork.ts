export type Uplink = "ethernet" | "wifi1" | "wifi2";

const validText = (value: string) => [...value].every(char => char.charCodeAt(0) >= 32 && char.charCodeAt(0) !== 127);
const quote = (value: string) => '"' + value.replace(/["\\$]/g, "\\$&") + '"';

/** Marks every resource the optional Wi-Fi WAN setup creates or changes, so a re-paste can resume its own partial work. */
export const MARK = "Magic Hub Wi-Fi WAN";

export function validWifi(ssid: string, passphrase: string) {
  const ssidBytes = new TextEncoder().encode(ssid).length;
  return ssidBytes > 0 && ssidBytes <= 32 && validText(ssid) &&
    validText(passphrase) && /^[ -~]{8,63}$/.test(passphrase);
}

// Only the new-router path uses this step. It shares the Hub block and is placed
// after every Magic Hub ownership check and before the first Magic Hub write, so a
// failed check anywhere stops the paste before anything changes, and the network
// is up before the tunnel is created.
export function withNewRouterNetwork(script: string, uplink: Uplink, ssid: string, passphrase: string) {
  if (!script.trimStart().startsWith("{")) throw new Error("Expected a RouterOS script block");
  const applyAt = script.indexOf("\n# APPLY:");
  if (applyAt < 0) throw new Error("Expected a Magic Hub preflight/apply boundary");
  const wifi = uplink !== "ethernet";
  if (wifi && !validWifi(ssid, passphrase)) throw new Error("Invalid Wi-Fi credentials");
  const iface = wifi ? uplink : "ether1";
  const network = `
# New hAP ax2 (RouterOS 7.13 or later: uses /interface wifi) with factory defaults. Run in Safe Mode over a LAN Ethernet port.
# Not for other models, RouterOS 7.12 (wifiwave2) or legacy wireless: use "Keep existing network settings" there.
:if ([/system routerboard get model] != "C52iG-5HaxD2HaxD") do={ :error "STOP: this network setup requires a factory-configured hAP ax2" }
:if ([:len [/ip address find where interface=bridge and address="192.168.88.1/24" and disabled=no]] != 1 || [:len [/ip dhcp-server find where interface=bridge and disabled=no]] != 1 || [:len [/interface list member find where list=LAN and interface=bridge]] != 1 || [:len [/ip hotspot find]] != 0) do={ :error "STOP: existing network settings detected; use Magic Hub enrollment only" }
# Only a Magic Hub tunnel from an earlier attempt of this setup may exist; the Hub checks below verify it.
:foreach mmWg in=[/interface wireguard find] do={
  :if ([:pick [/interface wireguard get $mmWg comment] 0 15] != "mega-magic:mmh-") do={ :error "STOP: existing network settings detected; use Magic Hub enrollment only" }
}
:local mmWanNat false
:foreach mmRule in=[/ip firewall nat find where chain=srcnat and action=masquerade and !disabled] do={
  :if ([:tostr [/ip firewall nat get $mmRule out-interface-list]] = "WAN") do={ :set mmWanNat true }
}
:local mmWanDrop false
:foreach mmRule in=[/ip firewall filter find where chain=input and action=drop and !disabled] do={
  :if ([:tostr [/ip firewall filter get $mmRule in-interface-list]] = "!LAN") do={ :set mmWanDrop true }
}
:if (!$mmWanNat || !$mmWanDrop) do={ :error "STOP: factory WAN firewall or NAT is missing" }
:local mmUplink "${iface}"
:local mmClient [/ip dhcp-client find where interface=$mmUplink]
${wifi ? `:local mmRadio [/interface wifi find where default-name=$mmUplink and name=$mmUplink]
:local mmMark "${MARK}"
:local mmBridgePort [/interface bridge port find where interface=$mmUplink and bridge=bridge]
:local mmMember [/interface list member find where interface=$mmUplink]
:if ([:len $mmRadio] != 1 || [:len $mmBridgePort] != 1) do={ :error "STOP: Wi-Fi interface missing or already configured; nothing changed" }
# Each item is either still at factory settings or carries this setup's marker from an earlier attempt.
:local mmPortOurs ([/interface bridge port get $mmBridgePort comment] = $mmMark)
:if (!$mmPortOurs && [/interface bridge port get $mmBridgePort disabled]) do={ :error "STOP: Wi-Fi bridge port was changed outside this setup; nothing changed" }
:if ([:len $mmMember] > 1) do={ :error "STOP: Wi-Fi interface is in several interface lists; nothing changed" }
:if ([:len $mmMember] = 1) do={
  :if ([/interface list member get $mmMember comment] != $mmMark || [/interface list member get $mmMember list] != "WAN") do={ :error "STOP: Wi-Fi interface list was changed outside this setup; nothing changed" }
}
:if ([:len $mmClient] > 1) do={ :error "STOP: several DHCP clients on the Wi-Fi interface; nothing changed" }
:if ([:len $mmClient] = 1) do={
  :if ([/ip dhcp-client get $mmClient comment] != $mmMark) do={ :error "STOP: Wi-Fi DHCP client was changed outside this setup; nothing changed" }
}
:if ([:len $mmClient] = 0 && [:len [/ip route find where dst-address="0.0.0.0/0" and active=yes]] != 0) do={ :error "STOP: an internet route is already active; use Keep existing network settings" }
# APPLY (Wi-Fi WAN): resumes a partial earlier attempt; each step runs only if not already done.
:if (!$mmPortOurs) do={ /interface bridge port set $mmBridgePort disabled=yes comment=$mmMark }
:if ([:len $mmMember] = 0) do={ /interface list member add list=WAN interface=$mmUplink comment=$mmMark }
/interface wifi set $mmRadio configuration.mode=station configuration.ssid=${quote(ssid)} security.authentication-types=wpa2-psk security.passphrase=${quote(passphrase)} disabled=no
:if ([:len $mmClient] = 0) do={ /ip dhcp-client add interface=$mmUplink add-default-route=no use-peer-dns=no use-peer-ntp=no disabled=no comment=$mmMark }
:set mmClient [/ip dhcp-client find where interface=$mmUplink]
` : `:if ([:len $mmClient] != 1 || [:len [/interface list member find where list=WAN and interface=$mmUplink]] != 1 || [:len [/interface bridge port find where interface=$mmUplink and disabled=no]] != 0) do={ :error "STOP: factory Ethernet WAN or DHCP client is missing" }
/ip dhcp-client set $mmClient disabled=no
`}
:for mmWait from=1 to=30 do={
  :if ([/ip dhcp-client get $mmClient status] != "bound") do={ :delay 1s }
}
:if ([/ip dhcp-client get $mmClient status] != "bound") do={ :error "STOP: no WAN DHCP lease; exit Safe Mode with Ctrl+D to undo the setup" }
:local mmLease [:tostr [/ip dhcp-client get $mmClient address]]
:local mmSlash [:find $mmLease "/"]
:local mmGateway [/ip dhcp-client get $mmClient gateway]
:if ([:typeof $mmSlash] != "num" || [:typeof [:toip $mmGateway]] != "ip") do={ :error "STOP: WAN lease has no usable IPv4 gateway; undo in Safe Mode with Ctrl+D" }
:if ([:tonum [:pick $mmLease ($mmSlash + 1) [:len $mmLease]]] < 24 || ([:toip [:pick $mmLease 0 $mmSlash]] in 192.168.88.0/24) || ([:toip $mmGateway] in 192.168.88.0/24)) do={ :error "STOP: WAN subnet may overlap the factory LAN; undo in Safe Mode with Ctrl+D" }
/ip dhcp-client set $mmClient add-default-route=yes default-route-distance=${wifi ? 2 : 1} use-peer-dns=yes
:delay 2s
:if ([:len [/ip route find where dst-address="0.0.0.0/0" and active=yes and gateway=$mmGateway]] = 0) do={ :error "STOP: no active default route through the WAN; undo in Safe Mode with Ctrl+D" }
:put "WAN DHCP and default route are ready. Continuing with Magic Hub."
`;
  return script.slice(0, applyAt) + "\n" + network + script.slice(applyAt);
}
