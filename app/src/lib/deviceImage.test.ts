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
    };
    for (const [id, list] of Object.entries(names)) for (const n of list) expect(productForName(n)?.id, n).toBe(id);
  });
  it("an unlisted model never borrows another model's photo", () => {
    for (const n of ["hAP ax3", "hAP ax lite", "hAP ax", "hAP ax23", "hAP ac2", "cAP ax2", "wAP ax lite", "cAP ac", "RB5009x", "RB4011", "CRS326", "", undefined]) {
      if (n === "RB5009x") continue; // a longer RB5009 name is still an RB5009
      expect(productForName(n), String(n)).toBeUndefined();
      expect(deviceImage({ hardware_name: n })).toMatch(/\.svg$/);
    }
    expect(isHapAx2("hAP ax3")).toBe(false);
    expect(isHapAx2("hAP ax²")).toBe(true);
  });
  it("a recognised model uses its photo under the app base path", () => {
    expect(deviceImage({ hardware_name: "RB5009UG+S+IN" })).toMatch(/models\/rb5009\.jpg$/);
    expect(deviceImage({ hardware_name: "hAP ax2" })).toMatch(/models\/hap-ax2\.jpg$/);
  });
  it("falls back to a plain symbol by role", () => {
    expect(deviceImage({ hardware_name: "hAP ax3", deployment_layer: "MAIN_GATEWAY" })).toMatch(/device\.svg$/);
    expect(deviceImage({ hardware_name: "hAP ax3" })).toMatch(/ap\.svg$/);
  });
});
