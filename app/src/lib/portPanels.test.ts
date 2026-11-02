import { existsSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { panelFor, portPanels } from "./portPanels";

describe("port panels", () => {
  it("finds the diagram from the router's board name", () => {
    expect(panelFor("C53UiG+5HPaxD2HPaxD")?.id).toBe("hap-ax3");
    expect(panelFor("c52iG-5HaxD2HaxD-TC")?.id).toBe("hap-ax2");
  });
  it("falls back to the owner's name only on an exact model match, and never lends one model's panel to another", () => {
    expect(panelFor(undefined, "hAP ax3")?.id).toBe("hap-ax3");
    expect(panelFor(undefined, "hAP ax²")?.id).toBe("hap-ax2");
    expect(panelFor("", "hAP ax lite")).toBeUndefined();
    expect(panelFor(undefined, "hAP ax S")).toBeUndefined();
    expect(panelFor("RB5009UG+S+IN", "Office router")).toBeUndefined();
    expect(panelFor(undefined, undefined)).toBeUndefined();
    expect(panelFor("C53UiG+5HPaxD2HPaxD", "hAP ax2")?.id).toBe("hap-ax3");
  });
  it("has real pictures and sane port positions", () => {
    for (const p of portPanels) {
      expect(existsSync(new URL("../../public/" + p.image, import.meta.url)), p.image).toBe(true);
      expect(new Set(p.ports.map((s) => s.name)).size).toBe(p.ports.length);
      for (const s of p.ports) {
        expect(s.x).toBeGreaterThan(0); expect(s.x).toBeLessThan(100);
        expect(s.y).toBeGreaterThan(0); expect(s.y).toBeLessThan(100);
        expect(s.name).toMatch(/^ether[1-9]$/);
      }
      expect(p.size).toBeGreaterThan(2); expect(p.size).toBeLessThan(15);
    }
  });
});
