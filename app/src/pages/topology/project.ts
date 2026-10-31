import { asArray } from "../../lib/types";

// Read-only projection of what a gateway reported. A forwarding entry is not proof of a direct cable,
// so nothing here invents a switch, a hop or an online state.
export interface NetObject { values?: Record<string, string> }
export interface ObsInventory { available?: Record<string, boolean>; objects?: Record<string, NetObject[]>; health?: Record<string, string>; observed_at?: string }
export interface GatewayRef { device_id: string; hardware_name?: string; device_model?: string }
export interface Label { mac: string; name?: string; kind?: string }
export interface TNode { id: string; mac?: string; name: string; ip: string; model?: string; kind: string; sources: string[]; ports: string[]; state: string; connection: string; direct: boolean; upstream: boolean; signal?: string; image?: string }
export interface Graph { root: TNode; devices: TNode[]; observedAt?: string; unavailable: string[] }

const macOf = (v: unknown) => (/^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(String(v)) ? String(v).toUpperCase() : "");
const yes = (v: unknown) => v === "true" || v === "yes";
const rows = (inv: ObsInventory, key: string) => (inv.available?.[key] === true ? asArray<NetObject>(inv.objects?.[key]).map((o) => o.values ?? {}) : []);
const KINDS: Record<string, string> = { ap: "ap", switch: "switch", computer: "computer", printer: "printer", controller: "device" };

export function project(device: GatewayRef, inv: ObsInventory = {}, labels: Label[] = []): Graph {
  const own = new Set(rows(inv, "interfaces").map((v) => macOf(v["mac-address"])).filter(Boolean));
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
      if (v.board || v.platform) n.model = v.board || v.platform;
      if (key === "arp" && String(wireless.get(v.interface)?.["configuration.mode"]).startsWith("station")) {
        n.upstream = true; n.kind = "upstream"; n.ports.push(v.interface); n.connection = "Uplink neighbour (ARP)";
        if (n.name === "Unknown device") n.name = "Upstream neighbour";
      }
      if (key === "neighbors" && v.interface) { n.ports.push(v.interface); n.connection = "Neighbour reported"; n.direct = true; }
      if (key === "bridge_hosts" && v["on-interface"]) { n.ports.push(v["on-interface"]); n.connection = "Learned on port"; n.direct = true; }
      if (key === "wifi_clients") {
        const wifi = wireless.get(v.interface) ?? {};
        n.ports.push(v.interface); n.signal = v.signal; n.state = "Wi-Fi associated"; n.connection = "Wi-Fi"; n.direct = true;
        if (String(wifi["configuration.mode"]).startsWith("station")) { n.upstream = true; n.kind = "upstream"; n.name = wifi["configuration.ssid"] || n.name; n.connection = "Wi-Fi uplink"; }
      }
      if (key === "hotspot_sessions") n.state = "HotSpot session";
    }
  }
  for (const label of labels) {
    const n = devices.get(macOf(label.mac));
    if (!n || n.upstream) continue;
    if (label.name) n.name = label.name;
    n.kind = KINDS[label.kind ?? ""] ?? n.kind;
  }
  const all = [...devices.values()].map((n) => ({ ...n, ports: [...new Set(n.ports)] }));
  const board = inv.health?.["board-name"] || device.device_model || "";
  const read = !!inv.observed_at && !!(inv.health?.version || Object.values(inv.available ?? {}).some(Boolean));
  return {
    root: { id: device.device_id, name: device.hardware_name || "RouterBOARD", ip: "", model: board, kind: "router", image: /hAP\s*ax(?:\^?2|²)/i.test(board + " " + device.hardware_name) ? "hap-ax2.webp" : "switch.svg", state: read ? "Management reachable" : "Not read", sources: ["Pinned management connection"], ports: [], connection: "", direct: true, upstream: false },
    devices: all, observedAt: inv.observed_at,
    unavailable: Object.entries(inv.available ?? {}).filter(([, ok]) => !ok).map(([k]) => k),
  };
}
