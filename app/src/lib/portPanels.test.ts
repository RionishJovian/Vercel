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
    expect(panelFor("", "hAP ax lite")?.id).toBe("hap-ax-lite");
    expect(panelFor(undefined, "hAP ax S")).toBeUndefined();
    expect(panelFor("RB5009UG+S+IN", "Office router")).toBeUndefined();
    expect(panelFor(undefined, undefined)).toBeUndefined();
    expect(panelFor("C53UiG+5HPaxD2HPaxD", "hAP ax2")?.id).toBe("hap-ax3");
    expect(panelFor("unsupported board", "hAP ax2")).toBeUndefined();
    expect(panelFor("C52iG-other", "hAP ax2")).toBeUndefined();
    expect(panelFor(undefined, "Office hAP ax2 lab")).toBeUndefined();
  });
  it.each([
    ["L41G-2axD", "hap-ax-lite", 4], ["RBD53iG-5HacD2HnD", "hap-ac3", 5],
    ["RB760iGS", "hex-s", 5], ["L009UiGS-RM", "l009", 8], ["RB4011iGS+RM", "rb4011", 10],
    ["CCR2004-16G-2S+PC", "ccr2004", 16], ["CCR2116-12G-4S+", "ccr2116", 13],
    ["CCR2216-1G-12XS-2XQ", "ccr2216", 1], ["CRS112-8P-4S-IN", "crs112", 8],
    ["CRS305-1G-4S+IN", "crs305", 1], ["CRS309-1G-8S+IN", "crs309", 1],
    ["CRS310-8G+2S+IN", "crs310", 8], ["CRS328-24P-4S+RM", "crs328", 24],
  ])("uses the exact %s Ethernet layout", (board, id, count) => {
    const panel = panelFor(String(board));
    expect(panel?.id).toBe(id);
    expect(panel?.ports.map((p) => p.name).sort()).toEqual(Array.from({ length: Number(count) }, (_, i) => `ether${i + 1}`).sort());
  });
  it.each(["CCR2004-1G-12S+2XS", "CCR2004-16G-2S+", "CRS310-1G-5S-4S+IN", "CRS328-4C-20S-4S+RM", "RB5009UPr+S+IN", "E60iUGS", "L009UiGS-2HaxD-IN"])("does not borrow a layout for %s", (board) => {
    expect(panelFor(board, "hAP ax2")).toBeUndefined();
  });
  it("has real pictures and sane port positions", () => {
    for (const p of portPanels) {
      expect(existsSync(new URL("../../public/" + p.image, import.meta.url)), p.image).toBe(true);
      expect(new Set(p.ports.map((s) => s.name)).size).toBe(p.ports.length);
      for (const s of p.ports) {
        expect(s.x).toBeGreaterThan(0); expect(s.x).toBeLessThan(100);
        expect(s.y).toBeGreaterThan(0); expect(s.y).toBeLessThan(100);
        expect(s.name).toMatch(/^ether[1-9][0-9]*$/);
      }
      expect(p.size).toBeGreaterThan(2); expect(p.size).toBeLessThanOrEqual(15);
    }
  });
  it("keeps dense diagram controls far enough apart for their 28 px hit targets", () => {
    for (const panel of portPanels.filter((p) => p.minWidth)) {
      const width = panel.minWidth!, height = width * panel.height / panel.width;
      for (const [i, port] of panel.ports.entries()) {
        for (const other of panel.ports.slice(i + 1)) {
          const dx = Math.abs(port.x - other.x) * width / 100;
          const dy = Math.abs(port.y - other.y) * height / 100;
          expect(Math.max(dx, dy), `${panel.id}: ${port.name}/${other.name}`).toBeGreaterThanOrEqual(28);
        }
      }
    }
  });
});
