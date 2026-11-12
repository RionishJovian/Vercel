import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import type { Me } from "../lib/types";

type FleetResult = { data?: unknown; error?: unknown; isLoading: boolean };
let fleet: FleetResult;
let report: { status: number; body: unknown }, routers: unknown, accounts: { status: number; body: unknown }, incidents: unknown;
vi.mock("../lib/useFleet", () => ({ useFleet: () => fleet }));
import { Home } from "./Home";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const me = { user: { full_name: "Daw Mya", email: "mya@example.com" }, business: { company_name: "Golden Tea" } } as unknown as Me;
const REPORT = { activation_today: 12500, activation_last7: 70000, activation_last30: 300000, activation_value: 90000, available_count: 40, reserved_count: 5, activated_count: 55, expired_count: 0 };

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  report = { status: 200, body: REPORT };
  routers = { routers: [{ device_id: "d1", hardware_name: "Gate", deployment_layer: "MAIN_GATEWAY" }] };
  accounts = { status: 200, body: { accounts: [
    { id: "a1", username: "ko.aung", profile: "10M", observation: { first_use: 1, expires_at: 4102444800, suspended: false, last_clock: 1 }, session: { id: "s", download_bytes: 1, upload_bytes: 1, uptime_seconds: 60 } },
    { id: "a2", username: "ma.su", profile: "5M", observation: { first_use: 1, expires_at: 4102444800, suspended: true, last_clock: 1 }, session: null },
  ], write_enabled: false } };
  incidents = { incidents: [] };
  fleet = { isLoading: false, data: { total_peers: 1, up: 1 } };
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/voucher-report") return json(report.status, report.body);
    if (path === "/sites") return json(200, { sites: [{ site_id: "s1", name: "Shop" }] });
    if (path === "/router-connections") return json(200, { connections: [] });
    if (path === "/routers") return json(200, routers);
    if (path === "/routers/d1/broadband/accounts") return json(accounts.status, accounts.body);
    if (path === "/broadband/report") return json(200, { amount_mmk: 30000, today_mmk: 15000, payment_count: 2, pending_count: 0 });
    if (path === "/incidents") return json(200, incidents);
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider><Home me={me} /></I18nProvider></SWRConfig>);
const card = (name: string) => screen.getByRole("heading", { name }).closest("section") as HTMLElement;

describe("Home", () => {
  it("greets the owner and shows Hotspot and PPPoE as separate services", async () => {
    open();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("Daw Mya");
    expect(screen.getByText("Golden Tea")).toBeTruthy();
    const hotspot = card("Hotspot"), pppoe = card("PPPoE ISP");
    expect(await within(hotspot).findByText("40")).toBeTruthy();        // codes ready
    expect(within(hotspot).getByText("Value of codes used, not cash received")).toBeTruthy();
    expect(await within(pppoe).findByText("2")).toBeTruthy();            // subscribers
    expect(within(pppoe).getByText("15,000 MMK")).toBeTruthy();          // recorded payments, not a gateway
  });

  it("falls back to the email name when there is no full name", () => {
    render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><Home me={{ user: { email: "ko.zaw@example.com" } } as unknown as Me} /></I18nProvider></SWRConfig>);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("ko.zaw");
  });

  it("says the voucher figures are unavailable instead of showing zero when the report fails", async () => {
    report = { status: 500, body: { error: "boom" } };
    open();
    const hotspot = card("Hotspot");
    expect(await within(hotspot).findByText("Voucher figures are unavailable")).toBeTruthy();
    expect(within(hotspot).queryByText("0")).toBeNull();
    // The other service still renders on its own.
    expect(await within(card("PPPoE ISP")).findByText("2")).toBeTruthy();
  });

  it("counts a subscriber as connected only from a recorded session", async () => {
    open();
    const pppoe = card("PPPoE ISP");
    await within(pppoe).findByText("2");
    const connected = within(pppoe).getByText("Connected").closest(".metric") as HTMLElement;
    expect(within(connected).getByText("1")).toBeTruthy();
    const suspended = within(pppoe).getByText("Suspended").closest(".metric") as HTMLElement;
    expect(within(suspended).getByText("1")).toBeTruthy();
  });

  it("shows how many routers are online, and flags offline routers", async () => {
    fleet = { isLoading: false, data: { total_peers: 3, up: 1, offline: 2 } };
    open();
    expect(await screen.findByText("1 of 3 routers online")).toBeTruthy();
    expect(screen.getAllByText("Offline").length).toBeGreaterThan(0);
    expect(document.querySelector(".chip.state.ok")).toBeNull();
  });

  it("marks unobserved routers as not confirmed instead of offline", async () => {
    fleet = { isLoading: false, data: { total_peers: 2, up: 1, offline: 0, stale: 0, unknown: 1 } };
    open();
    expect(await screen.findByText("Not confirmed")).toBeTruthy();
    expect(document.querySelector(".chip.state.bad")).toBeNull();
  });

  it("treats missing peer classifications as not confirmed, not offline", async () => {
    fleet = { isLoading: false, data: { total_peers: 3, up: 1 } };
    open();
    expect(await screen.findByText("Not confirmed")).toBeTruthy();
    expect(document.querySelector(".chip.state.bad")).toBeNull();
  });

  it("reports a failed fleet read rather than an empty or offline fleet", async () => {
    fleet = { isLoading: false, error: new Error("down") };
    open();
    expect(await screen.findByText("Status unavailable")).toBeTruthy();
    expect(document.querySelector(".chip.state.bad")).toBeNull();
  });

  it("says no router yet, never offline, when none are registered", async () => {
    fleet = { isLoading: false, data: { total_peers: 0, up: 0 } };
    routers = { routers: [] };
    open();
    expect(await screen.findByText("No router yet")).toBeTruthy();
    expect(await within(card("PPPoE ISP")).findByText("No gateway router yet")).toBeTruthy();
    expect(screen.queryByText("Offline", { selector: ".chip *, .chip" })).toBeNull();
  });

  it("puts open alerts first with a route to the alert", async () => {
    incidents = { incidents: [{ id: "i1", severity: "critical", subject_label: "Gate", detail: "Router stopped reporting", opened_at: new Date().toISOString() }] };
    open();
    expect(await screen.findByText("Router stopped reporting")).toBeTruthy();
    expect(screen.getByRole("link", { name: /All alerts/ }).getAttribute("href")).toBe("#/incidents");
  });
});
