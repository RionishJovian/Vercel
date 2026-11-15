import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { gatewayStatus, subscriberState } from "./data";

let fleet: { data?: unknown; error?: unknown; isLoading: boolean };
let api: Record<string, [number, unknown]>;
vi.mock("../../lib/useFleet", () => ({ useFleet: () => fleet }));
import { Setup } from "./Setup";
import { Customers, Hotspot } from "./Services";

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
  it.each([
    ["en", 0, "No active usage records yet"],
    ["my", 0, "လက်ရှိအသုံးပြုမှု မှတ်တမ်းမရှိသေးပါ"],
    ["en", 3, "First use recorded"],
    ["my", 3, "ပထမအသုံးပြုမှု မှတ်တမ်းရှိ"],
  ])("shows an accurate usage hint in %s for %s codes", async (lang, count, hint) => {
    localStorage.setItem("mm2_lang", String(lang));
    api["/voucher-report"] = [200, { activated_count: count, activation_today: 0 }];
    wrap(<Customers params={new URLSearchParams("type=hotspot")} />);
    expect(await screen.findByText(String(hint))).toBeTruthy();
    if (count) expect(screen.queryByText(/No active usage records yet|လက်ရှိအသုံးပြုမှု မှတ်တမ်းမရှိသေးပါ/)).toBeNull();
  });

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

describe("Hotspot service status", () => {
  const gwRow = (id: string, name: string) => ({ device_id: id, hardware_name: name, deployment_layer: "MAIN_GATEWAY" });
  const fresh = () => new Date(Date.now() - 5000).toISOString();

  it("describes voucher, member and package sign-in in the subtitle, in both languages", async () => {
    wrap(<Hotspot />);
    expect(await screen.findByText(/captive portal using a voucher code, member login, or Wi-Fi package/)).toBeTruthy();
    cleanup();
    localStorage.setItem("mm2_lang", "my");
    wrap(<Hotspot />);
    expect(await screen.findByText(/ဘောက်ချာကုဒ်၊ အဖွဲ့ဝင်အကောင့် သို့မဟုတ် Wi-Fi ပက်ကေ့ချ်/)).toBeTruthy();
  });

  it("leaves out a router that was deleted and judges status from the ones that remain", async () => {
    // The router list still holds the deleted router; the status feed, read later, no longer does.
    api["/routers"] = [200, { routers: [gwRow("a", "New Magic"), gwRow("b", "Nish's ax3")] }];
    fleet = { isLoading: false, data: { total_peers: 1, up: 1, peers: [{ device_id: "a", state: "up", checked_at: fresh() }] } };
    wrap(<Hotspot />);
    expect(await screen.findByText("New Magic")).toBeTruthy();
    expect(screen.queryByText(/Nish's ax3/)).toBeNull();
    expect(screen.getByText("Online")).toBeTruthy();
    expect(screen.getByText("Just now")).toBeTruthy();
    expect(screen.queryByText("now")).toBeNull();
  });

  it("says Just now in Burmese", async () => {
    localStorage.setItem("mm2_lang", "my");
    api["/routers"] = [200, { routers: [gwRow("a", "New Magic")] }];
    fleet = { isLoading: false, data: { peers: [{ device_id: "a", state: "up", checked_at: fresh() }] } };
    wrap(<Hotspot />);
    expect(await screen.findByText("ယခုလေးတင်")).toBeTruthy();
  });

  it("rolls up only gateways present in both lists", () => {
    const rows = [gwRow("a", "New Magic"), gwRow("b", "Nish's ax3")];
    // A deleted router that was offline must not turn the service red, and a non-gateway device never counts.
    const gone = gatewayStatus(rows, { peers: [{ device_id: "a", state: "up", checked_at: "2026-11-15T03:00:00Z" }, { device_id: "ap1", state: "offline", checked_at: "2026-11-15T04:00:00Z" }] });
    expect(gone.gateways?.map((g) => g.hardware_name)).toEqual(["New Magic"]);
    expect([gone.tone, gone.key, gone.checkedAt, gone.disagree]).toEqual(["ok", "st.online", "2026-11-15T03:00:00Z", true]);
    const both = gatewayStatus(rows, { peers: [{ device_id: "a", state: "up" }, { device_id: "b", state: "offline" }] });
    expect([both.tone, both.key, both.disagree]).toEqual(["bad", "st.offline", false]);
    expect(gatewayStatus(rows, { peers: [{ device_id: "a", state: "stale" }, { device_id: "b", state: "up" }] }).key).toBe("st.stale");
    expect(gatewayStatus(rows, { peers: [{ device_id: "a", state: "up" }, { device_id: "b" }] }).key).toBe("st.unknown");
    // Before the status has loaded nothing is hidden and nothing is claimed.
    const early = gatewayStatus(rows, undefined);
    expect([early.gateways?.length, early.key, early.disagree]).toEqual([2, "st.unknown", false]);
    expect(gatewayStatus(rows, undefined, true).key).toBe("st.noEvidence");
    expect(gatewayStatus([], { peers: [] }).key).toBe("st.noRouters");
  });
});
