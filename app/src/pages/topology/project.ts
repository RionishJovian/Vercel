import { asArray } from "../../lib/types";
import { productForName } from "../../lib/mikrotikProducts";

// Read-only projection of what a gateway reported. A forwarding entry is not proof of a direct cable,
// so nothing here invents a switch, a hop or an online state.
export interface NetObject { id?: string; values?: Record<string, string> }
export interface ObsInventory { available?: Record<string, boolean>; objects?: Record<string, NetObject[]>; health?: Record<string, string>; observed_at?: string; device_controls_enabled?: boolean; pending_confirm?: { revert_at?: string } }
export interface GatewayRef { device_id: string; hardware_name?: string; device_model?: string }
export interface Label { mac: string; name?: string; kind?: string }
export interface DeviceRecord { device_id: string; hardware_name?: string; deployment_layer?: string; mac_address?: string; management_ip?: string }
export interface GuestSession { id: string; user: string; address: string; mac: string }
export interface PortInfo { name: string; running: boolean; disabled: boolean; label: string; poe: boolean }
export interface TNode { id: string; mac?: string; name: string; ip: string; model?: string; platform?: string; kind: string; sources: string[]; ports: string[]; state: string; connection: string; direct: boolean; upstream: boolean; signal?: string; image?: string; dhcpServer?: string; blocked?: boolean; guestBanned?: boolean; session?: GuestSession; record?: DeviceRecord }
export interface Graph { root: TNode; devices: TNode[]; observedAt?: string; unavailable: string[]; ports: PortInfo[]; controlsEnabled: boolean; pendingAt: string }

const macOf = (v: unknown) => (/^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(String(v)) ? String(v).toUpperCase() : "");
const yes = (v: unknown) => v === "true" || v === "yes";
const rows = (inv: ObsInventory, key: string) => (inv.available?.[key] === true ? asArray<NetObject>(inv.objects?.[key]).map((o) => o.values ?? {}) : []);
const KINDS: Record<string, string> = { ap: "ap", switch: "switch", computer: "computer", printer: "printer", controller: "device" };
const LAYER_KINDS: Record<string, string> = { ACCESS_POINT: "ap", DOWNSTREAM_SWITCH: "switch", CONTROLLER: "device" };
// Reading this menu fails on any router without PoE, which is not a missing source.
const NOT_A_SOURCE = new Set(["poe_ports", "interface_labels"]);

export function project(device: GatewayRef, inv: ObsInventory = {}, labels: Label[] = [], records: DeviceRecord[] = []): Graph {
  const interfaces = rows(inv, "interfaces");
  const own = new Set(interfaces.map((v) => macOf(v["mac-address"])).filter(Boolean));
  const types = new Map(interfaces.map((v) => [v.name, v.type]));
  // The router can report a port together with its bridge ("ether2,bridge", or
  // "ether2;bridge" on RouterOS 7.25). The port is the useful part.
  const portsOf = (raw?: string) => {
    const names = String(raw ?? "").split(/[,;]/).map((s) => s.trim()).filter(Boolean);
    const specific = names.filter((n) => types.get(n) !== "bridge");
    return specific.length ? specific : names;
  };
  const wireless = new Map(rows(inv, "wifi_interfaces").map((v) => [v.name, v]));
  const devices = new Map<string, TNode>();
  const get = (v: Record<string, string>) => {
    const address = macOf(v["mac-address"]);
    if (!address || own.has(address) || parseInt(address.slice(0, 2), 16) & 1) return null;
    let n = devices.get(address);
    if (!n) { n = { id: address, mac: address, name: "Unknown device", ip: "", kind: "device", sources: [], ports: [], state: "Reported", connection: "Attachment unknown", direct: false, upstream: false }; devices.set(address, n); }
    return n;
  };
  for (const key of ["discovery_leases", "arp", "neighbors", "bridge_hosts", "wifi_clients", "hotspot_sessions"]) {
    for (const v of rows(inv, key)) {
      if (key === "bridge_hosts" && yes(v.local)) continue;
      const n = get(v); if (!n) continue;
      if (!n.sources.includes(key)) n.sources.push(key);
      if (v.address) n.ip = v.address;
      if (v["host-name"] || v.identity) n.name = v.identity || v["host-name"];
      if (v.board) n.model = v.board;
      if (v.platform) n.platform = v.platform;
      if (key === "discovery_leases" && v.server) n.dhcpServer = v.server;
      if (key === "arp" && String(wireless.get(v.interface)?.["configuration.mode"]).startsWith("station")) {
        n.upstream = true; n.kind = "upstream"; n.ports.push(v.interface); n.connection = "Uplink neighbour (ARP)";
        if (n.name === "Unknown device") n.name = "Upstream neighbour";
      }
      if (key === "neighbors" && v.interface) { n.ports.push(...portsOf(v.interface)); n.connection = "Neighbour reported"; n.direct = true; }
      if (key === "bridge_hosts" && v["on-interface"]) { n.ports.push(v["on-interface"]); n.connection = "Learned on port"; n.direct = true; }
      if (key === "wifi_clients") {
        const wifi = wireless.get(v.interface) ?? {};
        n.ports.push(v.interface); n.signal = v.signal; n.state = "Wi-Fi associated"; n.connection = "Wi-Fi"; n.direct = true;
        if (String(wifi["configuration.mode"]).startsWith("station")) { n.upstream = true; n.kind = "upstream"; n.name = wifi["configuration.ssid"] || n.name; n.connection = "Wi-Fi uplink"; }
      }
      if (key === "hotspot_sessions") n.state = "HotSpot session";
    }
  }
  // Sessions keep the record id, which the router needs to end exactly this one.
  if (inv.available?.hotspot_sessions === true) {
    for (const o of asArray<NetObject>(inv.objects?.hotspot_sessions)) {
      const v = o.values ?? {}, n = devices.get(macOf(v["mac-address"]));
      if (n && o.id) n.session = { id: o.id, user: v.user ?? "", address: v.address ?? "", mac: v["mac-address"] ?? "" };
    }
  }
  for (const v of rows(inv, "device_blocks")) { const n = devices.get(macOf(v["src-mac-address"])); if (n) n.blocked = true; }
  // The guest actions' own ban, an IP binding: a different thing from the firewall block above.
  for (const v of rows(inv, "hotspot_bindings")) { if (v.comment !== "mega-magic:ban" || v.type !== "blocked" || yes(v.disabled)) continue; const n = devices.get(macOf(v["mac-address"])); if (n) n.guestBanned = true; }
  // A saved record only names what the router reported. It adds no node and no proof of access.
  for (const record of records) {
    const mac = macOf(record.mac_address);
    const n = (mac && devices.get(mac)) || [...devices.values()].find((d) => record.management_ip && d.ip === record.management_ip);
    if (!n || n.upstream) continue;
    n.record = record; n.kind = LAYER_KINDS[record.deployment_layer ?? ""] ?? n.kind;
  }
  for (const label of labels) {
    const n = devices.get(macOf(label.mac));
    if (!n || n.upstream) continue;
    if (label.name) n.name = label.name;
    n.kind = KINDS[label.kind ?? ""] ?? n.kind;
  }
  const all = [...devices.values()].map((n) => ({ ...n, ports: [...new Set(n.ports)] }));
  const poe = new Set(rows(inv, "poe_ports").map((v) => v.name));
  const portLabels = new Map(rows(inv, "interface_labels").map((v) => [v.name, v.comment ?? ""]));
  const ports = interfaces.filter((v) => v.type === "ether").map((v): PortInfo => ({ name: v.name, running: yes(v.running), disabled: yes(v.disabled), label: portLabels.get(v.name) ?? "", poe: poe.has(v.name) }));
  const board = inv.health?.["board-name"] || device.device_model || "";
  const read = !!inv.observed_at && !!(inv.health?.version || Object.values(inv.available ?? {}).some(Boolean));
  return {
    root: { id: device.device_id, name: device.hardware_name || "RouterBOARD", ip: "", model: board, kind: "router", image: productForName(board, device.hardware_name)?.image ?? "switch.svg", state: read ? "Router read" : "Not read", sources: ["Pinned management connection"], ports: [], connection: "", direct: true, upstream: false },
    devices: all, observedAt: inv.observed_at,
    unavailable: Object.entries(inv.available ?? {}).filter(([k, ok]) => !ok && !NOT_A_SOURCE.has(k)).map(([k]) => k),
    ports, controlsEnabled: inv.device_controls_enabled === true, pendingAt: inv.pending_confirm?.revert_at ?? "",
  };
}
