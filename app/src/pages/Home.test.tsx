import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import type { Me } from "../lib/types";

type FleetResult = { data?: unknown; error?: unknown; isLoading: boolean };
let fleet: FleetResult;
let report: { status: number; body: unknown }, routers: unknown;
vi.mock("../components/SiteMap", () => ({ default: ({ home }: { home: boolean }) => <div role="region" aria-label={home ? "Myanmar map with Starlink on" : "Site map"} /> }));
vi.mock("../lib/useFleet", () => ({ useFleet: () => fleet }));
import { Home } from "./Home";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const me = { user: { full_name: "Daw Mya", email: "mya@example.com" } } as unknown as Me;
const REPORT = { activation_today: 12500, activation_last7: 70000, activation_last30: 300000, available_count: 40, reserved_count: 5, activated_count: 55 };

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  report = { status: 200, body: REPORT };
  routers = { routers: [{ device_id: "d1" }, { device_id: "d2" }] };
  fleet = { isLoading: false, data: { total_peers: 2, up: 2 } };
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/voucher-report") return json(report.status, report.body);
    if (path === "/sites") return json(200, { sites: [] });
    if (path === "/router-connections") return json(200, { connections: [] });
    if (path === "/routers") return json(200, routers);
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider><Home me={me} /></I18nProvider></SWRConfig>);

describe("Home", () => {
  it("greets the owner by name and shows the Myanmar map and ports before the stock card", async () => {
    open();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("Daw Mya");
    const map = await screen.findByRole("region", { name: "Myanmar map with Starlink on" });
    const ports = screen.getByRole("heading", { name: "Ports" });
    expect(map.compareDocumentPosition(ports) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByText("12,500")).toBeNull();
    expect(screen.getByText("40")).toBeTruthy();               // vouchers ready
  });

  it("falls back to the email name when there is no full name", () => {
    render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><Home me={{ user: { email: "ko.zaw@example.com" } } as unknown as Me} /></I18nProvider></SWRConfig>);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("ko.zaw");
  });

  it("says the figures are unavailable instead of showing zero when the report fails", async () => {
    report = { status: 500, body: { error: "boom" } };
    open();
    expect(await screen.findByText("Voucher figures are unavailable.")).toBeTruthy();
    expect(screen.queryByText("0")).toBeNull();
    expect(screen.getAllByText("Not available").length).toBeGreaterThan(0);
  });

  it("shows how many routers are online, and warns when some are not", async () => {
    fleet = { isLoading: false, data: { total_peers: 3, up: 1, offline: 2 } };
    open();
    expect(await screen.findByText("1 / 3")).toBeTruthy();
    expect(document.querySelector(".chip.warn")).not.toBeNull();
    expect(document.querySelector(".chip.ok")).toBeNull();
  });

  it("marks unobserved routers unknown instead of falsely marking them offline", async () => {
    fleet = { isLoading: false, data: { total_peers: 2, up: 1, offline: 0, stale: 0, unknown: 1 } };
    open();
    expect(await screen.findByText("1 / 2")).toBeTruthy();
    expect(screen.getByText("Unknown")).toBeTruthy();
    expect(document.querySelectorAll(".leds .unknown").length).toBe(1);
    expect(document.querySelectorAll(".leds .off").length).toBe(0);
  });

  it("treats missing peer classifications as unknown, not confirmed offline", async () => {
    fleet = { isLoading: false, data: { total_peers: 3, up: 1 } };
    open();
    expect(await screen.findByText("1 / 3")).toBeTruthy();
    expect(document.querySelectorAll(".leds .unknown").length).toBe(2);
    expect(document.querySelectorAll(".leds .off").length).toBe(0);
  });

  it("reports a failed fleet read rather than an empty fleet", () => {
    fleet = { isLoading: false, data: undefined, error: new Error("x") };
    open();
    expect(screen.getByText("Router status is unavailable.")).toBeTruthy();
    expect(document.querySelector(".leds")).toBeNull();
  });

  it("offers to add a router when none are registered", async () => {
    routers = { routers: [] };
    fleet = { isLoading: false, data: { total_peers: 0, up: 0 } };
    open();
    expect(await screen.findByRole("link", { name: /add/i })).toBeTruthy();
  });
});
