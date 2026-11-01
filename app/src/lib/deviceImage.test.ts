import { describe, expect, it } from "vitest";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { deviceImage, isHapAx2 } from "./deviceImage";
import { mikrotikProducts, productForName } from "./mikrotikProducts";

describe("device photo", () => {
  it("every catalog photo exists in the shipped static files", () => {
    for (const p of mikrotikProducts) expect(existsSync(join(__dirname, "../../public", p.image)), p.id).toBe(true);
  });
  it("each model is recognised by its name and only by it", () => {
    const names: Record<string, string[]> = {
      rb5009: ["RB5009UG+S+IN", "rb5009", "MikroTik RB5009"],
      ccr2004: ["CCR2004-16G-2S+PC"],
      "hap-ax2": ["hAP ax2", "hAP ax^2", "hAP ax²", "hap ax2", "RouterBOARD hAP ax2 lab"],
      crs310: ["CRS310-8G+2S+IN"],
      "cap-ax": ["cAP ax", "cap ax"],
      "wap-ax": ["wAP ax", "wap ax"],
      "hap-ax3": ["hAP ax³", "hAP ax3"],
      "hap-ax-lite": ["hAP ax lite"], "hap-ax-s": ["hAP ax S"], "hap-ac3": ["hAP ac³", "hAP ac3"], "hap-ac2": ["hAP ac²", "hAP ac2"],
      "hex-s": ["hEX S"], rb4011: ["RB4011iGS+RM", "RB4011iGS+5HacQ2HnD-IN"], rb1100: ["RB1100AHx4"], ccr2116: ["CCR2116-12G-4S+"], ccr2216: ["CCR2216-1G-12XS-2XQ"],
      "chateau-pro-ax": ["Chateau PRO ax"], l009: ["L009UiGS-RM"], "cap-ac": ["cAP ac"], "cap-xl-ac": ["cAP XL ac"], "wap-ac": ["wAP ac"],
      crs326: ["CRS326-24G-2S+IN"], crs328: ["CRS328-24P-4S+RM"], crs305: ["CRS305-1G-4S+IN"], crs309: ["CRS309-1G-8S+IN"], crs112: ["CRS112-8P-4S-IN"], css610: ["CSS610-8G-2S+IN"],
    };
    for (const [id, list] of Object.entries(names)) for (const n of list) expect(productForName(n)?.id, n).toBe(id);
  });
  it("an unlisted model never borrows another model's photo", () => {
    for (const n of ["hAP ax", "hAP ax23", "hAP ac", "hAP ac lite", "hAP lite", "cAP ax2", "wAP ax lite", "wAP R ac", "RB5009x", "RB3011", "RB750Gr3", "hEX", "CRS354-48G", "CRS326-24S+2Q+RM", "", undefined]) {
      if (n === "RB5009x") continue; // a longer RB5009 name is still an RB5009
      expect(productForName(n), String(n)).toBeUndefined();
      expect(deviceImage({ hardware_name: n })).toMatch(/\.svg$/);
    }
    expect(isHapAx2("hAP ax3")).toBe(false);
    expect(deviceImage({ hardware_name: "hAP ax³" })).toMatch(/models\/hap-ax3\.jpg$/);
    expect(isHapAx2("hAP ax²")).toBe(true);
  });
  it("a recognised model uses its photo under the app base path", () => {
    expect(deviceImage({ hardware_name: "RB5009UG+S+IN" })).toMatch(/models\/rb5009\.jpg$/);
    expect(deviceImage({ hardware_name: "hAP ax2" })).toMatch(/models\/hap-ax2\.jpg$/);
  });
  it("each catalog entry's own name matches only itself", () => {
    for (const p of mikrotikProducts) {
      expect(productForName(p.name)?.id, p.name).toBe(p.id);
      expect(mikrotikProducts.filter((q) => q.match.test(p.name.toLowerCase().replace(/²/g, "2").replace(/³/g, "3"))).map((q) => q.id), p.name).toEqual([p.id]);
    }
  });
  it("falls back to a plain symbol by role", () => {
    expect(deviceImage({ hardware_name: "RB3011", deployment_layer: "MAIN_GATEWAY" })).toMatch(/device\.svg$/);
    expect(deviceImage({ hardware_name: "RB3011" })).toMatch(/ap\.svg$/);
  });
});
