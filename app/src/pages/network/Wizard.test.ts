import { describe, expect, it } from "vitest";
import { suggestPorts } from "./Wizard";

const ifaces = [{ name: "ether1", type: "ether" }, { name: "bridge", type: "bridge" }, { name: "wifi1", type: "wifi", running: true }, { name: "wifi2", type: "wifi" }];

describe("suggested ports for a new owner", () => {
  it("uses the port with an internet lease as uplink and a spare radio for guests", () => {
    expect(suggestPorts(ifaces, [{ values: { interface: "ether1", gateway: "" } }, { values: { interface: "wifi1", gateway: "192.168.100.1" } }])).toEqual({ wan: "wifi1", lan: "wifi2" });
  });
  it("suggests no uplink when no port has a gateway", () => {
    expect(suggestPorts(ifaces, [{ values: { interface: "ether1", gateway: "" } }]).wan).toBe("");
  });
  it("ignores a client on an interface the router did not report", () => {
    expect(suggestPorts(ifaces, [{ values: { interface: "ether9", gateway: "10.0.0.1" } }]).wan).toBe("");
  });
  it("suggests no guest port on a router without a radio", () => {
    expect(suggestPorts([{ name: "ether1", type: "ether" }], []).lan).toBe("");
  });
});
