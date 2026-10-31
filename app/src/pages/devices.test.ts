import { discoveryRows } from "./Devices";

describe("discoveryRows", () => {
  const inv = { available: { discovery_leases: true, arp: true, neighbors: false }, objects: {
    discovery_leases: [{ values: { "mac-address": "aa:bb:cc:00:00:01", address: "10.0.0.5", "host-name": "Phone", status: "bound" } }],
    arp: [{ values: { "mac-address": "AA:BB:CC:00:00:01", address: "10.0.0.5" } }, { values: { "mac-address": "AA:BB:CC:00:00:02", address: "10.0.0.6" } }],
    neighbors: [{ values: { "mac-address": "AA:BB:CC:00:00:03" } }],
  } };
  const rows = discoveryRows(inv);
  it("merges the same device seen in two tables", () => { expect(rows.find((r) => r.mac === "AA:BB:CC:00:00:01")).toMatchObject({ name: "Phone", state: "bound", sources: ["DHCP lease", "ARP table"] }); });
  it("ignores tables the router could not read", () => { expect(rows.some((r) => r.mac === "AA:BB:CC:00:00:03")).toBe(false); });
  it("never claims a device is online from a table entry", () => { expect(rows.find((r) => r.mac === "AA:BB:CC:00:00:02")?.state).toBe("unverified"); });
});
