import { existsSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { mikrotikProducts } from "../../lib/mikrotikProducts";
import { findModel, findModelByProfile, modelCatalogue, modelGroups, setupBlockFor, uplinksFor } from "./modelCatalogue";
import { withNewRouterNetwork } from "./newRouterNetwork";
import { routerProfiles, type RouterProfile } from "./routerProfiles";

describe("model picker catalogue", () => {
  it("lists every model in the product catalogue, once, with its photo on disk", () => {
    expect(modelCatalogue.map(m => m.product.id)).toEqual(mikrotikProducts.map(p => p.id));
    for (const m of modelCatalogue) expect(existsSync(join(__dirname, "../../../public", m.product.image)), m.product.id).toBe(true);
    expect(modelGroups().flatMap(g => g.models)).toHaveLength(modelCatalogue.length);
  });
  it("has exactly one of a setup profile or a reason for every model", () => {
    for (const m of modelCatalogue) expect(Boolean(m.profile) !== Boolean(m.blocked), m.product.id).toBe(true);
  });
  it("makes every profile reachable from the picker", () => {
    for (const p of routerProfiles) expect(findModelByProfile(p.id)?.product.id).toBe(p.productId);
  });
  it("offers Ethernet and each radio band for a Wi-Fi profile, in a fixed order", () => {
    // Order is by band (5 GHz first), never by interface name: wAP ax has wifi1 = 2.4 GHz.
    for (const id of ["hap-ax2", "hap-ax3", "hap-ax-s", "chateau-pro-ax", "cap-ax", "wap-ax", "hap-ac2", "hap-ac3", "cap-ac", "wap-ac"]) expect(uplinksFor(findModel(id)), id).toEqual(["ethernet", "5ghz", "2ghz"]);
  });
  it("offers 2.4 GHz only for the single-radio hAP ax lite, and Ethernet only for wired models", () => {
    expect(uplinksFor(findModel("hap-ax-lite"))).toEqual(["ethernet", "2ghz"]);
    for (const id of ["rb5009", "rb4011", "l009", "hex-s"]) expect(uplinksFor(findModel(id)), id).toEqual(["ethernet"]);
  });
  it("offers nothing but keep-existing for a model without a profile", () => {
    for (const id of ["ccr2004", "ccr2116", "ccr2216", "rb1100", "crs310", "css610", "cap-xl-ac"]) expect(uplinksFor(findModel(id)), id).toEqual([]);
    expect(uplinksFor(undefined)).toEqual([]);
  });
  it("gives each blocked model the reason MikroTik's defaults imply", () => {
    for (const id of ["crs310", "crs326", "crs328", "crs305", "crs309", "crs112", "css610"]) expect(findModel(id)?.blocked, id).toBe("switch");
    for (const id of ["ccr2004", "ccr2116", "ccr2216", "rb1100"]) expect(findModel(id)?.blocked, id).toBe("ip-only");
    // The only router or AP left without a row: its printed board string is not documented apart from the cAP ac's.
    expect(findModel("cap-xl-ac")?.blocked).toBe("board-unverified");
  });
  it("leaves only switches, IP-only core routers and the cAP XL ac without a setup profile", () => {
    const without = modelCatalogue.filter(m => !m.profile).map(m => m.product.id).sort();
    expect(without).toEqual(["cap-xl-ac", "ccr2004", "ccr2116", "ccr2216", "crs112", "crs305", "crs309", "crs310", "crs326", "crs328", "css610", "rb1100"]);
    expect(modelCatalogue.filter(m => m.profile)).toHaveLength(15);
  });
  it("shows the back-panel diagram where one exists and never borrows one", () => {
    expect(findModel("hap-ax3")?.panel?.id).toBe("hap-ax3");
    expect(findModel("hap-ax2")?.panel?.id).toBe("hap-ax2");
    expect(modelCatalogue.filter(m => m.panel).map(m => m.product.id).sort()).toEqual(["hap-ax2", "hap-ax3"]);
  });
  it("groups by category with models that can be set up first", () => {
    const groups = modelGroups();
    expect(groups.map(g => g.category)).toEqual(["router", "ap", "switch"]);
    const router = groups[0].models;
    expect(router.slice(0, 11).every(m => m.profile)).toBe(true);
    expect(router.slice(11).every(m => !m.profile)).toBe(true);
    expect(router.slice(0, 11).map(m => m.product.id).sort()).toEqual(["chateau-pro-ax", "hap-ac2", "hap-ac3", "hap-ax-lite", "hap-ax-s", "hap-ax2", "hap-ax3", "hex-s", "l009", "rb4011", "rb5009"]);
    const ap = groups[1].models;
    expect(ap.slice(0, 4).map(m => m.product.id).sort()).toEqual(["cap-ac", "cap-ax", "wap-ac", "wap-ax"]);
    expect(ap.slice(4).map(m => m.product.id)).toEqual(["cap-xl-ac"]);
  });
  it("lets a profile override a block, so adding a row is the only step to enable a model", () => {
    const rb5009 = mikrotikProducts.find(p => p.id === "rb5009")!;
    expect(setupBlockFor(rb5009, false)).toBe("board-unverified");
    expect(setupBlockFor(rb5009, true)).toBeUndefined();
  });
});

describe("wired-only profile", () => {
  // A synthetic row (not a real board code): proves a model with no radios gets Ethernet only.
  const wired: RouterProfile = {
    id: "fixture-wired", productId: "rb5009", scriptName: "Fixture wired", boardModel: "FIXTURE-WIRED", evidence: "documented",
    ports: [{ name: "ether1", kind: "ether", speed: "1G" }, { name: "ether2", kind: "ether", speed: "1G" }],
    wan: "ether1", radios: [], lan: { bridge: "bridge", address: "192.168.88.1/24", network: "192.168.88.0/24" }, wifiDriver: "wifi", docs: ["https://help.mikrotik.com/docs/"],
  };
  const BLOCK = "{\n:put \"probe\"\n}\n";
  it("generates the Ethernet script with its own board guard", () => {
    const script = withNewRouterNetwork(BLOCK, wired, "ethernet", "", "");
    expect(script).toContain('!= "FIXTURE-WIRED"');
    expect(script).toContain(':local mmUplink "ether1"');
    expect(script).not.toContain("/interface wifi");
  });
  it.each(["5ghz", "2ghz"] as const)("refuses %s Wi-Fi WAN because the model has no radio", band => {
    expect(() => withNewRouterNetwork(BLOCK, wired, band, "Net", "password1")).toThrow(`has no ${band} radio`);
  });
});
