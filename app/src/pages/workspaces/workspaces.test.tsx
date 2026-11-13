import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { subscriberState } from "./data";

let fleet: { data?: unknown; error?: unknown; isLoading: boolean };
let api: Record<string, [number, unknown]>;
vi.mock("../../lib/useFleet", () => ({ useFleet: () => fleet }));
import { Setup } from "./Setup";
import { Customers } from "./Services";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: () => {} });
  fleet = { isLoading: false, data: { total_peers: 0, up: 0 } };
  api = { "/sites": [200, { sites: [] }], "/router-connections": [200, { connections: [] }], "/routers": [200, { routers: [] }], "/voucher-report": [200, { available_count: 0, reserved_count: 0, activated_count: 0, expired_count: 0 }], "/portal/config": [404, {}] };
  vi.stubGlobal("fetch", vi.fn(async (url: string) => { const r = api[String(url).replace("/api/v1", "")]; return r ? json(r[0], r[1]) : json(404, {}); }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const wrap = (ui: React.ReactNode) => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider>{ui}</I18nProvider></SWRConfig>);
const step = (name: string) => screen.getByText(name).closest("li") as HTMLElement;

describe("Quick setup", () => {
  it("starts at the site step and waits on steps that depend on it", async () => {
    wrap(<Setup />);
    expect(await screen.findByText("0 of 5 checked steps done")).toBeTruthy();
    expect(within(step("Create your site")).getByText("To do")).toBeTruthy();
    expect(within(step("Connect Magic Hub")).getByText("Waiting")).toBeTruthy();
    expect(within(step("Create your site")).getByRole("link", { name: "Open Sites" }).getAttribute("href")).toBe("/app/sites");
  });

  it("never ticks steps the cloud cannot observe", async () => {
    api["/sites"] = [200, { sites: [{ site_id: "s", name: "Shop" }] }];
    api["/router-connections"] = [200, { connections: [{ site_id: "s", cloud_status: "online", cloud_last_seen_at: new Date().toISOString() }] }];
    api["/routers"] = [200, { routers: [{ device_id: "d", hardware_name: "Gate", deployment_layer: "MAIN_GATEWAY" }] }];
    api["/voucher-report"] = [200, { available_count: 10, reserved_count: 0, activated_count: 1, expired_count: 0 }];
    api["/portal/config"] = [200, { config: {} }];
    fleet = { isLoading: false, data: { total_peers: 1, up: 1 } };
    wrap(<Setup />);
    expect(await screen.findByText("Every checked step is done")).toBeTruthy();
    expect(within(step("Internet setup")).getByText("Check on router")).toBeTruthy();
    expect(within(step("Guest login page")).getByText("Check on router")).toBeTruthy();
    expect(within(step("First customer test")).getByText("Done")).toBeTruthy();
  });

  it("does not count a router as online without fresh fleet evidence", async () => {
    api["/sites"] = [200, { sites: [{ site_id: "s", name: "Shop" }] }];
    api["/routers"] = [200, { routers: [{ device_id: "d", deployment_layer: "MAIN_GATEWAY" }] }];
    fleet = { isLoading: false, data: { total_peers: 1, up: 0, stale: 1 } };
    wrap(<Setup />);
    await screen.findByText(/checked steps done/);
    expect(within(step("Router online")).queryByText("Done")).toBeNull();
  });
});

describe("Customers", () => {
  it("labels PPPoE subscribers by service and filters by search", async () => {
    api["/routers"] = [200, { routers: [{ device_id: "d", hardware_name: "Gate", deployment_layer: "MAIN_GATEWAY" }] }];
    api["/routers/d/broadband/accounts"] = [200, { accounts: [
      { id: "1", username: "ko.aung", profile: "10M", observation: { first_use: 1, expires_at: 4102444800, suspended: false, last_clock: 1 }, session: null },
      { id: "2", username: "ma.su", profile: "5M", observation: null, session: null },
    ] }];
    const { container } = wrap(<Customers params={new URLSearchParams("type=pppoe")} />);
    expect(await screen.findByText("ko.aung")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Hotspot users" })).toBeNull();
    expect(within(screen.getByText("ma.su").closest("li") as HTMLElement).getByText("Not confirmed")).toBeTruthy();
    const search = screen.getByRole("searchbox");
    (await import("@testing-library/react")).fireEvent.change(search, { target: { value: "aung" } });
    expect(screen.queryByText("ma.su")).toBeNull();
    expect(container.querySelectorAll(".sub-row").length).toBe(1);
  });

  it("explains a failed router list instead of loading forever, and retries it", async () => {
    api["/routers"] = [500, { error: "boom" }];
    wrap(<Customers params={new URLSearchParams("type=pppoe")} />);
    expect(await screen.findByText("Subscriber records could not be read")).toBeTruthy();
    api["/routers"] = [200, { routers: [{ device_id: "d", hardware_name: "Gate", deployment_layer: "MAIN_GATEWAY" }] }];
    api["/routers/d/broadband/accounts"] = [200, { accounts: [{ id: "1", username: "ko.aung", profile: "10M", observation: null, session: null }] }];
    (await import("@testing-library/react")).fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(await screen.findByText("ko.aung")).toBeTruthy();
  });
});

describe("subscriber state", () => {
  const base = { id: "x", username: "u", profile: "p" };
  it("does not infer active without a router observation", () => { expect(subscriberState({ ...base, observation: null }).key).toBe("isp.s.unverified"); });
  it("shows a pending change before anything else", () => { expect(subscriberState({ ...base, pending_operation: "op", observation: { first_use: 0, expires_at: 0, suspended: true, last_clock: 0 } }).key).toBe("isp.s.pending"); });
  it("marks past service end as expired", () => { expect(subscriberState({ ...base, observation: { first_use: 1, expires_at: 100, suspended: false, last_clock: 1 } }, 200).key).toBe("isp.s.expired"); });
});
