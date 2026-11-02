import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";

type FleetResult = { data?: unknown; error?: unknown; isLoading: boolean };
let fleet: FleetResult;
vi.mock("../lib/useFleet", () => ({ useFleet: () => fleet }));
import { Fleet } from "./Fleet";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("matchMedia", () => ({ matches: true })); // reduced motion: the count shows at once
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/routers") return json(200, { routers: [{ device_id: "d1", hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY" }] });
    if (path === "/fleet/pulse") return json(200, { open_incidents_head: [{}, {}] });
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Fleet /></I18nProvider></SWRConfig>);

describe("Fleet", () => {
  it("shows each router's state and what needs attention", async () => {
    fleet = { isLoading: false, data: { total_peers: 3, up: 1, stale: 1, offline: 1, overall_state: "offline", peers: [
      { device_id: "d1", hardware_name: "Shop gateway", state: "up" },
      { device_id: "d2", hardware_name: "Back office", state: "stale" },
      { device_id: "d3", hardware_name: "Warehouse", state: "offline" },
    ] } };
    open();
    expect(await screen.findByText("Shop gateway")).toBeTruthy();
    expect(screen.getByText("Back office")).toBeTruthy();
    expect(screen.getByText("Warehouse")).toBeTruthy();
    expect(screen.getByText("Gateway")).toBeTruthy();
    for (const label of ["Online", "Stale", "Offline"]) expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    // Two of the three routers need attention; two incidents are open.
    const attention = screen.getByText("Needs attention").parentElement!;
    expect(attention.textContent).toContain("2");
    await screen.findByText("Open incidents");
  });

  it("says so when there are no routers", () => {
    fleet = { isLoading: false, data: { total_peers: 0, peers: [] } };
    open();
    expect(screen.getByText("No routers yet. Add a router to see it here.")).toBeTruthy();
  });

  it("shows an error, not an empty fleet, when the health read fails", () => {
    fleet = { isLoading: false, error: new Error("x") };
    open();
    expect(screen.queryByText("No routers yet. Add a router to see it here.")).toBeNull();
    expect(screen.getByText("Router status is unavailable.")).toBeTruthy();
  });

  it("treats a missing state as unknown, never online", () => {
    fleet = { isLoading: false, data: { total_peers: 1, up: 0, peers: [{ device_id: "d9", hardware_name: "Mystery" }] } };
    open();
    expect(screen.getByText("Unknown", { selector: ".chip" })).toBeTruthy();
    expect(screen.queryByText("Online", { selector: ".chip" })).toBeNull();
  });
});
