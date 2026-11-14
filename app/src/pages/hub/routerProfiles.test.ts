import { describe, expect, it } from "vitest";
import { findProfile, radioFor, routerProfiles } from "./routerProfiles";

describe("router profile table", () => {
  it("lists at least the hAP ax2 and the hAP ax3", () => {
    expect(routerProfiles.map(p => p.id)).toEqual(expect.arrayContaining(["hap-ax2", "hap-ax3"]));
  });
  it("keeps the hAP ax2 board string and layout that the lab router verified", () => {
    expect(findProfile("hap-ax2")).toMatchObject({ boardModel: "C52iG-5HaxD2HaxD", wan: "ether1", radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }] });
  });
  it("uses MikroTik's product code for the hAP ax3", () => {
    expect(findProfile("hap-ax3")).toMatchObject({ boardModel: "C53UiG+5HPaxD2HPaxD", wan: "ether1" });
  });
  it("has unique ids, names and board strings, so no model can borrow another's check", () => {
    for (const key of ["id", "name", "scriptName", "boardModel"] as const) expect(new Set(routerProfiles.map(p => p[key])).size).toBe(routerProfiles.length);
  });
  it.each(routerProfiles.map(p => [p.id, p] as const))("%s is safe to write into a RouterOS string and internally consistent", (_id, p) => {
    // These values are placed inside double quotes in the script; none may need escaping.
    expect(p.boardModel).toMatch(/^[A-Za-z0-9+_.-]+$/);
    expect(p.scriptName).toMatch(/^[ -~]+$/);
    expect(p.scriptName).not.toMatch(/["\\$]/);
    expect(p.wan).toMatch(/^ether\d+$/);
    expect(p.lan.address.startsWith(p.lan.network.replace(/\.0\/24$/, "."))).toBe(true);
    expect(p.wifiDriver).toBe("wifi");
    expect(p.radios.length).toBeGreaterThan(0);
    expect(new Set(p.radios.map(r => r.name)).size).toBe(p.radios.length);
    expect(new Set(p.radios.map(r => r.band)).size).toBe(p.radios.length);
    for (const r of p.radios) expect(r.name).toMatch(/^wifi\d+$/);
    expect(p.docs.length).toBeGreaterThan(0);
    for (const url of p.docs) expect(url).toMatch(/^https:\/\/(help|manual)\.mikrotik\.com\//);
  });
  it("finds radios by band and unknown models as undefined", () => {
    const ax3 = findProfile("hap-ax3")!;
    expect(radioFor(ax3, "5ghz")?.name).toBe("wifi1");
    expect(radioFor(ax3, "2ghz")?.name).toBe("wifi2");
    expect(findProfile("not-a-model")).toBeUndefined();
  });
});
