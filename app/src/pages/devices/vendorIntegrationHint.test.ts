import { describe, expect, it } from "vitest";
import { vendorIntegrationHint } from "./vendorIntegrationHint";

describe("mixed-brand vendor integration status", () => {
  it("recognizes only supported names without converting model or owner labels to proof", () => {
    const families = [
      ["TP-Link", "tp-link-omada"],
      ["Omada", "tp-link-omada"],
      ["Ruijie/Reyee", "ruijie-reyee"],
      ["iKuai", "ikuai"],
      ["Huawei", "huawei"],
    ] as const;
    for (const [name, family] of families) {
      expect(vendorIntegrationHint(name).family).toBe(family);
      expect(vendorIntegrationHint(name).configuration).toBe("not_verified");
    }
    expect(vendorIntegrationHint("Ruijie AP owner label").family).toBe("unknown");
    expect(vendorIntegrationHint("unidentified").family).toBe("unknown");
  });
  it("only Omada advertises an available read-only enrollment path", () => {
    expect(vendorIntegrationHint("TP-Link").inventory).toBe("enrollment_required");
    for (const name of ["Ruijie", "iKuai", "Huawei", "", "unsupported"]) {
      expect(vendorIntegrationHint(name).inventory).toBe("not_integrated");
      expect(vendorIntegrationHint(name).configuration).toBe("not_verified");
    }
  });
});
