import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { portPanels } from "../../lib/portPanels";
import { PortPanelView, type PortState } from "./PortPanel";

const ax3 = portPanels.find((p) => p.id === "hap-ax3")!;
const words = { full: "Full duplex", half: "Half duplex", updateAvail: "", noNewer: "", fwUpgrade: "" };
const ports: PortState[] = [
  { name: "ether1", running: true, link_rate: "1Gbps", full_duplex: true, rx_bps: 5000, tx_bps: 7000 },
  { name: "ether2", running: false },
  { name: "ether3", running: false, disabled: true },
  { name: "ether4", running: true, link_rate: "100Mbps", full_duplex: false },
];

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const show = (list = ports, measuring: string | null = null) => render(<I18nProvider><PortPanelView panel={ax3} ports={list} words={words} measuring={measuring} /></I18nProvider>);

describe("port panel", () => {
  it("shows one marker per port with the router's own state, and never guesses a port it was not told about", () => {
    show();
    const labels = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(labels).toEqual([
      "ether1: Online, 1 Gbps · Full duplex · ↓ 5 kbps ↑ 7 kbps",
      "ether2: No link",
      "ether3: Off",
      "ether4: Online, 100 Mbps · Half duplex · ↓ — ↑ —",
      "ether5: Not reported",
    ]);
  });
  it("colours markers by state", () => {
    show();
    const cls = screen.getAllByRole("button").map((b) => b.className);
    expect(cls[0]).toContain("up"); expect(cls[1]).toContain("down"); expect(cls[2]).toContain("off"); expect(cls[4]).toContain("unknown");
  });
  it("shows the chosen port's details, and a hint until one is chosen", () => {
    show();
    expect(screen.getByText("Select a port on the picture to see its status.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /^ether1/ }));
    expect(screen.getByText("ether1")).toBeTruthy();
    expect(screen.getByText(/1 Gbps · Full duplex · ↓ 5 kbps ↑ 7 kbps/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /^ether1/ }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: /^ether1/ }));
    expect(screen.getByText("Select a port on the picture to see its status.")).toBeTruthy();
  });
  it("does not invent a speed for a port that has no link", () => {
    show();
    fireEvent.click(screen.getByRole("button", { name: /^ether2/ }));
    expect(screen.getByText("No link")).toBeTruthy();
    expect(screen.queryByText(/kbps/)).toBeNull();
  });
  it("says measuring while speeds warm up, and treats an empty report as unknown ports", () => {
    show([{ name: "ether1", running: true }], "Measuring…");
    expect(screen.getByRole("button", { name: /^ether1/ }).getAttribute("aria-label")).toBe("ether1: Online, ↓ Measuring… ↑ Measuring…");
    cleanup();
    show([]);
    expect(screen.getAllByRole("button").every((b) => /Not reported/.test(b.getAttribute("aria-label") ?? ""))).toBe(true);
  });
});
