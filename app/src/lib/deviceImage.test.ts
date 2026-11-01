import { describe, expect, it } from "vitest";
import { deviceImage, isHapAx2 } from "./deviceImage";

describe("device photo", () => {
  it("uses the hAP ax2 photo only for an hAP ax2", () => {
    for (const name of ["hAP ax2", "hAP ax^2", "hAP ax²", "RouterBOARD hAP ax2 lab", "hap ax2"]) expect(isHapAx2(name), name).toBe(true);
    expect(deviceImage({ hardware_name: "hAP ax2" })).toMatch(/hap-ax2\.webp$/);
  });
  it("never shows the ax2 photo for another model", () => {
    for (const name of ["hAP ax3", "hAP ax lite", "hAP ax", "hAP ax23", "hAP ac2", "RB5009", "", undefined]) {
      expect(isHapAx2(name), String(name)).toBe(false);
      expect(deviceImage({ hardware_name: name })).not.toMatch(/hap-ax2/);
    }
  });
  it("falls back to a plain symbol by role", () => {
    expect(deviceImage({ hardware_name: "hAP ax3", deployment_layer: "MAIN_GATEWAY" })).toMatch(/device\.svg$/);
    expect(deviceImage({ hardware_name: "hAP ax3" })).toMatch(/ap\.svg$/);
  });
});
