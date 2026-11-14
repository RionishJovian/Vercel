import type { RouterProfile, WifiBand } from "./routerProfiles";
import { radioFor } from "./routerProfiles";

export type Uplink = "ethernet" | WifiBand;
export const isWifiUplink = (uplink: Uplink): uplink is WifiBand => uplink !== "ethernet";

const validText = (value: string) => [...value].every(char => char.charCodeAt(0) >= 32 && char.charCodeAt(0) !== 127);
const quote = (value: string) => '"' + value.replace(/["\\$]/g, "\\$&") + '"';

export function validWifi(ssid: string, passphrase: string) {
  const ssidBytes = new TextEncoder().encode(ssid).length;
  return ssidBytes > 0 && ssidBytes <= 32 && validText(ssid) &&
    validText(passphrase) && /^[ -~]{8,63}$/.test(passphrase);
}

// Only the new-router path uses this prelude. It shares the Hub block so
// a failed network check prevents the enrollment commands from running.
// Everything model-specific comes from the chosen profile (routerProfiles.ts). The first check
// compares the profile's board string with the router's own, so a wrong choice stops the
// script before anything is changed.
export function withNewRouterNetwork(script: string, profile: RouterProfile, uplink: Uplink, ssid: string, passphrase: string) {
  if (!script.trimStart().startsWith("{")) throw new Error("Expected a RouterOS script block");
  const wifi = isWifiUplink(uplink);
  if (wifi && !validWifi(ssid, passphrase)) throw new Error("Invalid Wi-Fi credentials");
  const radio = wifi ? radioFor(profile, uplink) : undefined;
  if (wifi && !radio) throw new Error(`${profile.scriptName} has no ${uplink} radio`);
  const iface = radio ? radio.name : profile.wan;
  const { bridge, address, network: lanNet } = profile.lan;
  const network = `
# New ${profile.scriptName} with factory defaults. Run in Safe Mode over a LAN Ethernet port.
:if ([/system routerboard get model] != ${quote(profile.boardModel)}) do={ :error ${quote("STOP: this network setup requires a factory-configured " + profile.scriptName)} }
:if ([:len [/ip address find where interface=${bridge} and address="${address}" and disabled=no]] != 1 || [:len [/ip dhcp-server find where interface=${bridge} and disabled=no]] != 1 || [:len [/interface list member find where list=LAN and interface=${bridge}]] != 1 || [:len [/ip hotspot find]] != 0 || [:len [/interface wireguard find]] != 0) do={ :error "STOP: existing network settings detected; use Magic Hub enrollment only" }
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
:local mmBridgePort [/interface bridge port find where interface=$mmUplink and bridge=${bridge} and disabled=no]
:if ([:len $mmRadio] != 1 || [:len $mmBridgePort] != 1 || [:len $mmClient] != 0 || [:len [/interface list member find where interface=$mmUplink]] != 0) do={ :error "STOP: Wi-Fi interface already configured or missing" }
:if ([:len [/ip route find where dst-address="0.0.0.0/0" and active=yes]] != 0) do={ :error "STOP: an internet route is already active" }
/interface bridge port disable $mmBridgePort
/interface list member add list=WAN interface=$mmUplink comment="Magic Hub Wi-Fi WAN"
/interface wifi set $mmRadio configuration.mode=station configuration.ssid=${quote(ssid)} security.authentication-types=wpa2-psk security.passphrase=${quote(passphrase)} disabled=no
/ip dhcp-client add interface=$mmUplink add-default-route=no use-peer-dns=no use-peer-ntp=no disabled=no comment="Magic Hub Wi-Fi WAN"
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
:if ([:tonum [:pick $mmLease ($mmSlash + 1) [:len $mmLease]]] < 24 || ([:toip [:pick $mmLease 0 $mmSlash]] in ${lanNet}) || ([:toip $mmGateway] in ${lanNet})) do={ :error "STOP: WAN subnet may overlap the factory LAN; undo in Safe Mode with Ctrl+D" }
/ip dhcp-client set $mmClient add-default-route=yes default-route-distance=${wifi ? 2 : 1} use-peer-dns=yes
:delay 2s
:if ([:len [/ip route find where dst-address="0.0.0.0/0" and active=yes and gateway=$mmGateway]] = 0) do={ :error "STOP: no active default route through the WAN; undo in Safe Mode with Ctrl+D" }
:put "WAN DHCP and default route are ready. Continuing with Magic Hub."
`;
  const blockStart = script.indexOf("{") + 1;
  return script.slice(0, blockStart) + network + script.slice(blockStart);
}
