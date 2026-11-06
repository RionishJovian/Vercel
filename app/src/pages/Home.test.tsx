import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import type { Me } from "../lib/types";

type FleetResult = { data?: unknown; error?: unknown; isLoading: boolean };
let fleet: FleetResult;
let report: { status: number; body: unknown }, routers: unknown;
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
    if (path === "/routers") return json(200, routers);
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider><Home me={me} /></I18nProvider></SWRConfig>);

describe("Home", () => {
  it("greets the owner by name and shows today's revenue against their own average", async () => {
    open();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("Daw Mya");
    expect(await screen.findByText("12,500")).toBeTruthy();   // today
    expect(screen.getByText("70,000")).toBeTruthy();           // last 7 days
    expect(screen.getByText("300,000")).toBeTruthy();          // last 30 days
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
    fleet = { isLoading: false, data: { total_peers: 3, up: 1 } };
    open();
    expect(await screen.findByText("1 / 3")).toBeTruthy();
    expect(document.querySelector(".chip.warn")).not.toBeNull();
    expect(document.querySelector(".chip.ok")).toBeNull();
  });

  it("reports a failed fleet read rather than an empty fleet", () => {
    fleet = { isLoading: false, data: undefined, error: new Error("x") };
    open();
    expect(screen.getByText("Router status is unavailable.")).toBeTruthy();
    expect(document.querySelector(".leds")).toBeNull();
  });

  it("points to Quick setup when no router is registered", async () => {
    routers = { routers: [] };
    fleet = { isLoading: false, data: { total_peers: 0, up: 0 } };
    open();
    const link = await screen.findByRole("link", { name: "Quick setup" });
    expect(link.getAttribute("href")).toBe("#/setup");
  });
});
