import { project, type ObsInventory } from "./project";

const gw = { device_id: "g1", hardware_name: "hAP ax2 lab" };
const inv: ObsInventory = {
  observed_at: "2026-10-31T12:00:00Z", health: { version: "7.24" },
  available: { interfaces: true, wifi_interfaces: true, arp: true, wifi_clients: true, bridge_hosts: true, neighbors: false },
  objects: {
    interfaces: [{ values: { "mac-address": "AA:00:00:00:00:01" } }],
    wifi_interfaces: [{ values: { name: "wifi1", "configuration.mode": "station", "configuration.ssid": "Home" } }, { values: { name: "wifi2", "configuration.mode": "ap" } }],
    arp: [{ values: { "mac-address": "DC:A6:32:00:00:09", address: "192.168.100.1", interface: "wifi1" } }],
    wifi_clients: [{ values: { "mac-address": "B8:27:EB:11:22:33", interface: "wifi2", "host-name": "Phone", signal: "-50" } }],
    bridge_hosts: [{ values: { "mac-address": "B8:27:EB:11:22:44", "on-interface": "ether2" } }, { values: { "mac-address": "AA:00:00:00:00:01", local: "true", "on-interface": "bridge" } }],
  },
};

describe("topology projection", () => {
  const g = project(gw, inv, [{ mac: "B8:27:EB:11:22:44", name: "Cashier PC", kind: "computer" }]);
  it("never lists the router's own MAC", () => { expect(g.devices.some((d) => d.mac === "AA:00:00:00:00:01")).toBe(false); });
  it("treats an ARP neighbour on a station radio as the upstream uplink", () => { const u = g.devices.find((d) => d.mac === "DC:A6:32:00:00:09"); expect(u).toMatchObject({ upstream: true, kind: "upstream", connection: "Uplink neighbour (ARP)" }); });
  it("places a Wi-Fi client on its radio", () => { expect(g.devices.find((d) => d.name === "Phone")).toMatchObject({ ports: ["wifi2"], state: "Wi-Fi associated", direct: true }); });
  it("applies saved names and kinds only to non-upstream devices", () => { expect(g.devices.find((d) => d.mac === "B8:27:EB:11:22:44")).toMatchObject({ name: "Cashier PC", kind: "computer" }); });
  it("treats bridge-learned devices as downstream paths, not a direct physical cable", () => {
    const node = g.devices.find((d) => d.mac === "B8:27:EB:11:22:44");
    expect(node).toMatchObject({ ports: ["ether2"], direct: false, connection: "Learned via gateway port (hop unverified)" });
  });
  it("shows registered but unobserved switches and APs without claiming connectivity", () => {
    const topology = project(gw, inv, [], [
      { device_id: "sw1", deployment_layer: "DOWNSTREAM_SWITCH", hardware_name: "Managed switch" },
      { device_id: "ap1", deployment_layer: "ACCESS_POINT", hardware_name: "RG-RAP", management_ip: "192.168.88.50" },
    ]);
    expect(topology.devices.find(n => n.id === "record:sw1")).toMatchObject({ kind: "switch", state: "Not observed", direct: false, ports: [] });
    expect(topology.devices.find(n => n.id === "record:ap1")).toMatchObject({ kind: "ap", state: "Not observed", connection: "Attachment unverified", direct: false });
  });
  it("does not create a second node when registered AP MAC matches an observed device", () => {
    const topology = project(gw, inv, [], [{ device_id: "ap2", deployment_layer: "ACCESS_POINT", hardware_name: "Cashier AP", mac_address: "B8:27:EB:11:22:44" }]);
    expect(topology.devices.filter(n => n.mac === "B8:27:EB:11:22:44")).toHaveLength(1);
    expect(topology.devices.find(n => n.mac === "B8:27:EB:11:22:44")?.record?.device_id).toBe("ap2");
  });
  it("reports unreadable sources instead of guessing", () => { expect(g.unavailable).toEqual(["neighbors"]); });
  it("marks the router read only when the router actually answered", () => { expect(g.root.state).toBe("Router read"); expect(project(gw).root.state).toBe("Not read"); });
});
