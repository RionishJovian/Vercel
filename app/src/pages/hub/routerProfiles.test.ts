import { describe, expect, it } from "vitest";
import { portPanels } from "../../lib/portPanels";
import { findProductPhoto } from "../../lib/mikrotikProducts";
import { assetsFor, findProfile, profileFor, profileName, radioFor, routerProfiles } from "./routerProfiles";

describe("router profile table", () => {
  it("lists the hAP ax2 and the hAP ax3 first, then every other documented router and AP", () => {
    expect(routerProfiles.slice(0, 2).map(p => p.id)).toEqual(["hap-ax2", "hap-ax3"]);
    expect(routerProfiles).toHaveLength(15);
  });
  it("keeps the hAP ax2 board string and layout that the lab router verified", () => {
    expect(findProfile("hap-ax2")).toMatchObject({ boardModel: "C52iG-5HaxD2HaxD", wan: "ether1", radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }] });
  });
  it("uses MikroTik's product code for the hAP ax3", () => {
    expect(findProfile("hap-ax3")).toMatchObject({ boardModel: "C53UiG+5HPaxD2HPaxD", wan: "ether1" });
  });
  it("records the printed board string of every model, not the retail product code", () => {
    const boards = Object.fromEntries(routerProfiles.map(p => [p.id, p.boardModel]));
    expect(boards).toEqual({
      "hap-ax2": "C52iG-5HaxD2HaxD", "hap-ax3": "C53UiG+5HPaxD2HPaxD", "hap-ax-lite": "L41G-2axD", "hap-ax-s": "E62iUGS-2axD5axT",
      "chateau-pro-ax": "H53UiG-5HaxQ2HaxQ", "hap-ac3": "RBD53iG-5HacD2HnD", "hap-ac2": "RBD52G-5HacD2HnD",
      // Printouts on forum.mikrotik.com drop the "IN" / "RM" retail suffix on these three.
      "rb5009": "RB5009UG+S+", "rb4011": "RB4011iGS+", "l009": "L009UiGS",
      "hex-s": "RB760iGS", "cap-ax": "cAPGi-5HaxD2HaxD", "wap-ax": "wAPG-5HaxD2HaxD", "cap-ac": "RBcAPGi-5acD2nD", "wap-ac": "RBwAPG-5HacD2HnD",
    });
  });
  it("only the hAP ax2 is marked as run on hardware", () => {
    expect(routerProfiles.filter(p => p.evidence === "hardware").map(p => p.id)).toEqual(["hap-ax2"]);
  });
  it("keeps each model's radio order, which differs between models", () => {
    const order = (id: string) => findProfile(id)!.radios.map(r => `${r.name}:${r.band}`).join(",");
    for (const id of ["hap-ax2", "hap-ax3", "cap-ax", "chateau-pro-ax"]) expect(order(id), id).toBe("wifi1:5ghz,wifi2:2ghz");
    for (const id of ["wap-ax", "hap-ax-s"]) expect(order(id), id).toBe("wifi1:2ghz,wifi2:5ghz");
    for (const id of ["hap-ac2", "hap-ac3", "cap-ac", "wap-ac"]) expect(order(id), id).toBe("wlan1:2ghz,wlan2:5ghz");
    expect(order("hap-ax-lite")).toBe("wifi1:2ghz");
    for (const id of ["rb5009", "rb4011", "l009", "hex-s"]) expect(order(id), id).toBe("");
  });
  it("uses the legacy wireless driver exactly for the ac models", () => {
    expect(routerProfiles.filter(p => p.wifiDriver === "wireless").map(p => p.id).sort()).toEqual(["cap-ac", "hap-ac2", "hap-ac3", "wap-ac"]);
  });
  it("has unique ids, names and board strings, so no model can borrow another's check", () => {
    for (const key of ["id", "productId", "scriptName", "boardModel"] as const) expect(new Set(routerProfiles.map(p => p[key])).size).toBe(routerProfiles.length);
  });
  it.each(routerProfiles.map(p => [p.id, p] as const))("%s is safe to write into a RouterOS string and internally consistent", (_id, p) => {
    // These values are placed inside double quotes in the script; none may need escaping.
    expect(p.boardModel).toMatch(/^[A-Za-z0-9+_.-]+$/);
    expect(p.scriptName).toMatch(/^[ -~]+$/);
    expect(p.scriptName).not.toMatch(/["\\$]/);
    expect(p.wan).toMatch(/^ether\d+$/);
    expect(p.lan.address.startsWith(p.lan.network.replace(/\.0\/24$/, "."))).toBe(true);
    expect(["wifi", "wireless"]).toContain(p.wifiDriver);
    expect(new Set(p.radios.map(r => r.name)).size).toBe(p.radios.length);
    expect(new Set(p.radios.map(r => r.band)).size).toBe(p.radios.length);
    for (const r of p.radios) expect(r.name).toMatch(p.wifiDriver === "wifi" ? /^wifi\d+$/ : /^wlan\d+$/);
    expect(p.docs.length).toBeGreaterThan(0);
    for (const url of p.docs) expect(url).toMatch(/^https:\/\/(?:(?:help|manual|forum)\.)?mikrotik\.com\//);
  });
  it("finds radios by band and unknown models as undefined", () => {
    const ax3 = findProfile("hap-ax3")!;
    expect(radioFor(ax3, "5ghz")?.name).toBe("wifi1");
    expect(radioFor(ax3, "2ghz")?.name).toBe("wifi2");
    expect(findProfile("not-a-model")).toBeUndefined();
  });
  describe("shared with the product catalogue and port panels", () => {
    it.each(routerProfiles.map(p => [p.id, p] as const))("%s is a catalogue model, with the same name, ports and board as the port panel", (_id, p) => {
      expect(findProductPhoto(p.productId)).toBeDefined();
      expect(profileName(p)).toBe(findProductPhoto(p.productId)!.name);
      const { panel } = assetsFor(p);
      if (panel) {
        expect(panel.board.test(p.boardModel)).toBe(true);
        expect(p.ports.filter(x => x.kind === "ether").map(x => x.name).sort()).toEqual(panel.ports.map(x => x.name).sort());
      }
      expect(p.ports.map(x => x.name)).toContain(p.wan);
      for (const r of p.radios) expect(p.ports.map(x => x.name)).not.toContain(r.name);
    });
    it("every port panel's model that has a setup profile agrees on the board", () => {
      for (const panel of portPanels) { const p = routerProfiles.find(x => x.productId === panel.id); if (p) expect(panel.board.test(p.boardModel)).toBe(true); }
    });
    it("looks a router up by board string first, then by its exact model name", () => {
      expect(profileFor("C53UiG+5HPaxD2HPaxD")?.id).toBe("hap-ax3");
      expect(profileFor("C52iG-5HaxD2HaxD", "hAP ax3")?.id).toBe("hap-ax2");
      expect(profileFor(undefined, "hAP ax3")?.id).toBe("hap-ax3");
      expect(profileFor(undefined, "hAP ax lite")?.id).toBe("hap-ax-lite");
      expect(profileFor("L41G-2axD")?.id).toBe("hap-ax-lite");
      expect(profileFor("RB5009UG+S+")?.id).toBe("rb5009");
      // The retail product code is not what the router prints, so it must not match by board.
      expect(profileFor("RB5009UG+S+IN")).toBeUndefined();
      expect(profileFor("RB4011iGS+5HacQ2HnD")).toBeUndefined();
      expect(profileFor(undefined, "CRS310")).toBeUndefined();
    });
    it("returns photo and panel for a profile, and no profile for a catalogue model without a setup row", () => {
      expect(assetsFor(findProfile("hap-ax3")!).panel?.id).toBe("hap-ax3");
      expect(assetsFor(findProfile("hap-ax2")!).photo?.id).toBe("hap-ax2");
      expect(routerProfiles.some(p => p.productId === "cap-xl-ac")).toBe(false);
    });
  });
});
