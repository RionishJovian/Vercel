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

/** Marks every resource the optional Wi-Fi WAN setup creates or changes, so a re-paste can resume its own partial work. */
export const MARK = "Magic Hub Wi-Fi WAN";

// Only the new-router path uses this step. It is split in two so that nothing changes until
// everything has been checked:
//   1. the checks (router model, factory LAN and firewall, state of the uplink) go first, before
//      anything else in the block, so a wrong model or a router that is not new stops at once;
//   2. the changes go at the Magic Hub preflight/apply boundary, after every Magic Hub ownership
//      check and before the first Magic Hub write, so the network is up before the tunnel is made.
// Items it creates carry MARK, so a paste that was cut off part-way can be pasted again and
// continues where it stopped; anything that is neither factory state nor marked stops the script.
// Everything model-specific comes from the chosen profile (routerProfiles.ts).
export function withNewRouterNetwork(script: string, profile: RouterProfile, uplink: Uplink, ssid: string, passphrase: string) {
  if (!script.trimStart().startsWith("{")) throw new Error("Expected a RouterOS script block");
  const applyAt = script.indexOf("\n# APPLY:");
  if (applyAt < 0) throw new Error("Expected a Magic Hub preflight/apply boundary");
  const wifi = isWifiUplink(uplink);
  if (wifi && !validWifi(ssid, passphrase)) throw new Error("Invalid Wi-Fi credentials");
  const radio = wifi ? radioFor(profile, uplink) : undefined;
  if (wifi && !radio) throw new Error(`${profile.scriptName} has no ${uplink} radio`);
  const iface = radio ? radio.name : profile.wan;
  const { bridge, address, network: lanNet } = profile.lan;
  const menu = profile.wifiDriver === "wireless" ? "/interface wireless" : "/interface wifi";
  // Checks shared by both Wi-Fi drivers: each item is either still at factory settings or carries MARK.
  const wifiChecks = `:local mmMark "${MARK}"
:local mmRadio [${menu} find where default-name=$mmUplink and name=$mmUplink]
:local mmBridgePort [/interface bridge port find where interface=$mmUplink and bridge=${bridge}]
:local mmMember [/interface list member find where interface=$mmUplink]
:if ([:len $mmRadio] != 1 || [:len $mmBridgePort] != 1) do={ :error "STOP: Wi-Fi interface missing or already configured; nothing changed" }
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
:if ([:len $mmClient] = 0 && [:len [/ip route find where dst-address="0.0.0.0/0" and active=yes]] != 0) do={ :error "STOP: an internet route is already active" }
`;
  // Legacy `/interface wireless` (the ac models) has no per-radio security fields, so the passphrase
  // lives in its own security profile named mm-wifi-wan. The factory "default" profile is shared with
  // the radio that stays on the LAN and is never edited. A router without the legacy package has no
  // `/interface wireless` menu, so these commands cannot run at all and nothing is changed.
  const wirelessProfileCheck = `:local mmProfile [/interface wireless security-profiles find where name="mm-wifi-wan"]
:if ([:len $mmProfile] > 1) do={ :error "STOP: several Wi-Fi security profiles named mm-wifi-wan; nothing changed" }
:if ([:len $mmProfile] = 1) do={
  :if ([/interface wireless security-profiles get $mmProfile comment] != $mmMark) do={ :error "STOP: Wi-Fi security profile mm-wifi-wan was created outside this setup; nothing changed" }
}
`;
  const stationApply = profile.wifiDriver === "wireless"
    ? `:if ([:len $mmProfile] = 0) do={ /interface wireless security-profiles add name=mm-wifi-wan mode=dynamic-keys authentication-types=wpa2-psk wpa2-pre-shared-key=${quote(passphrase)} comment=$mmMark } else={ /interface wireless security-profiles set $mmProfile wpa2-pre-shared-key=${quote(passphrase)} }
/interface wireless set $mmRadio mode=station ssid=${quote(ssid)} security-profile=mm-wifi-wan disabled=no
`
    : `/interface wifi set $mmRadio configuration.mode=station configuration.ssid=${quote(ssid)} security.authentication-types=wpa2-psk security.passphrase=${quote(passphrase)} disabled=no
`;
  const uplinkChecks = wifi
    ? wifiChecks + (profile.wifiDriver === "wireless" ? wirelessProfileCheck : "")
    : `:if ([:len $mmClient] != 1 || [:len [/interface list member find where list=WAN and interface=$mmUplink]] != 1 || [:len [/interface bridge port find where interface=$mmUplink and disabled=no]] != 0) do={ :error "STOP: factory Ethernet WAN or DHCP client is missing" }
`;
  const uplinkApply = wifi
    ? `# APPLY (Wi-Fi WAN): resumes a partial earlier attempt; each step runs only if not already done.
:if (!$mmPortOurs) do={ /interface bridge port set $mmBridgePort disabled=yes comment=$mmMark }
:if ([:len $mmMember] = 0) do={ /interface list member add list=WAN interface=$mmUplink comment=$mmMark }
${stationApply}:if ([:len $mmClient] = 0) do={ /ip dhcp-client add interface=$mmUplink add-default-route=no use-peer-dns=no use-peer-ntp=no disabled=no comment=$mmMark }
:set mmClient [/ip dhcp-client find where interface=$mmUplink]
`
    : `/ip dhcp-client set $mmClient disabled=no
`;
  const checks = `
# New ${profile.scriptName} with factory defaults. Run in Safe Mode over a LAN Ethernet port.
:if ([/system routerboard get model] != ${quote(profile.boardModel)}) do={ :error ${quote("STOP: this network setup requires a factory-configured " + profile.scriptName)} }
:if ([:len [/ip address find where interface=${bridge} and address="${address}" and disabled=no]] != 1 || [:len [/ip dhcp-server find where interface=${bridge} and disabled=no]] != 1 || [:len [/interface list member find where list=LAN and interface=${bridge}]] != 1 || [:len [/ip hotspot find]] != 0) do={ :error "STOP: existing network settings detected; use Magic Hub enrollment only" }
# Only a Magic Hub tunnel from an earlier attempt of this setup may exist; the Hub checks verify it.
:foreach mmWg in=[/interface wireguard find] do={
  :if ([:find [/interface wireguard get $mmWg comment] "mega-magic:mmh-"] != 0) do={ :error "STOP: existing network settings detected; use Magic Hub enrollment only" }
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
${uplinkChecks}`;
  const changes = `
${uplinkApply}:for mmWait from=1 to=30 do={
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
  return script.slice(0, blockStart) + checks + script.slice(blockStart, applyAt) + changes + script.slice(applyAt);
}
