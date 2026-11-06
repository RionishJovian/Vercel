import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { DiscoverHelper, type SessionView } from "./DiscoverHelper";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const SID = "0a1b2c3d-0000-4000-8000-000000000001";
const SITES = [{ site_id: "s1", name: "Shop" }];
const CAND = { device_key: "sw:0123456789abcdef", macs: ["48:A9:8A:00:00:06"], identity: "hAP", board: "C52iG-5HaxD2HaxD", platform: "MikroTik", version: "7.25 2026-10-22 14:12:00", interface: "bridge/wifi2", claimed_ip: "192.168.88.1", source_ip: "192.168.88.1", heard_count: 3, last_seen: "2026-11-06T02:00:00Z", selected: false, status: "nearby_unverified" };

let enabled: boolean, view: SessionView, calls: Array<{ method: string; path: string; body?: unknown }>, createStatus: number;
beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  enabled = true; createStatus = 201; calls = [];
  view = { session_id: SID, state: "open", expires_at: "2099-01-01T00:00:00Z", pairing_expires_at: new Date(Date.now() + 600000).toISOString(), candidates: [] };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", ""), method = init?.method ?? "GET";
    calls.push({ method, path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path === "/discovery/capability") return enabled ? json(200, { enabled: true }) : json(404, {});
    if (path === "/discovery/sessions" && method === "POST") return createStatus === 201 ? json(201, { session_id: SID, pairing_code: "ABCDEF-GHJKMNPQ" }) : json(createStatus, { error: "x" });
    if (path === `/discovery/sessions/${SID}` && method === "GET") return json(200, view);
    if (path === `/discovery/sessions/${SID}` && method === "DELETE") return new Response(null, { status: 204 });
    if (path === `/discovery/sessions/${SID}/select`) return json(200, { device_key: CAND.device_key, suggested_name: "hAP", device_model: "C52iG-5HaxD2HaxD", local_address: "192.168.88.1", site_id: "s1" });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (onSelected = vi.fn()) => { render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><DiscoverHelper sites={SITES} onSelected={onSelected} /></I18nProvider></SWRConfig>); return onSelected; };
const start = async () => { fireEvent.click(await screen.findByRole("button", { name: "Start a helper session" })); await screen.findByText(/magic-discover --pair ABCDEF-GHJKMNPQ/); };

describe("DiscoverHelper", () => {
  it("is hidden when the server has discovery switched off", async () => {
    enabled = false;
    const { container } = render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><DiscoverHelper sites={SITES} onSelected={vi.fn()} /></I18nProvider></SWRConfig>);
    await vi.waitFor(() => expect(calls.some((c) => c.path === "/discovery/capability")).toBe(true));
    expect(container.textContent).toBe("");
  });

  it("starts a session, shows the one-time code and says no download exists yet", async () => {
    open();
    expect(await screen.findByText("The helper is not available to download yet.")).toBeTruthy();
    await start();
    expect(calls.find((c) => c.method === "POST" && c.path === "/discovery/sessions")?.body).toEqual({ site_id: "s1" });
    await screen.findByText(/Waiting for the helper/);
  });

  it("shows nearby routers as not verified, never as connected, and lets the owner select one", async () => {
    view = { ...view, state: "paired", pairing_expires_at: undefined, candidates: [CAND] };
    const onSelected = open();
    await start();
    await screen.findByText("Nearby MikroTik router");
    const card = screen.getByLabelText("hAP");
    expect(card.textContent).toMatch(/C52iG-5HaxD2HaxD/);
    expect(card.textContent).toMatch(/Nearby, not verified/);
    expect(card.textContent).not.toMatch(/Connected|Online|Managed/);
    fireEvent.click(screen.getByRole("button", { name: "Select router" }));
    await vi.waitFor(() => expect(onSelected).toHaveBeenCalledTimes(1));
    expect(onSelected.mock.calls[0][0]).toEqual({ sessionId: SID, deviceKey: CAND.device_key, name: "hAP", model: "C52iG-5HaxD2HaxD", address: "192.168.88.1", siteId: "s1" });
    expect(calls.find((c) => c.path.endsWith("/select"))?.body).toEqual({ device_key: CAND.device_key });
  });

  it("shows Unknown for details the router did not advertise instead of inventing them", async () => {
    view = { ...view, state: "paired", pairing_expires_at: undefined, candidates: [{ ...CAND, identity: "", board: "", platform: "", version: "", claimed_ip: "", source_ip: "" }] };
    open(); await start();
    await screen.findByText("Nearby MikroTik router");
    expect(screen.getAllByText("Unknown").length).toBe(4);
  });

  it("points an already-registered device to its router instead of offering to select it", async () => {
    view = { ...view, state: "paired", pairing_expires_at: undefined, candidates: [{ ...CAND, existing_router: { device_id: "d1", name: "Shop router" } }] };
    open(); await start();
    const link = await screen.findByRole("link", { name: "Shop router" });
    expect(link.getAttribute("href")).toBe("#/router/d1");
    expect(screen.queryByRole("button", { name: "Select router" })).toBeNull();
  });

  it("warns about a mismatched or moving address", async () => {
    view = { ...view, state: "paired", pairing_expires_at: undefined, candidates: [{ ...CAND, ip_mismatch: true, ip_changed: true }] };
    open(); await start();
    await screen.findByText(/different address than the one it names/);
    screen.getByText(/more than one address/);
  });

  it("says when the code ran out and when the session ended", async () => {
    view = { ...view, pairing_expires_at: new Date(Date.now() - 1000).toISOString() };
    open(); await start();
    await screen.findByText(/code has expired before the helper used it/);
    cleanup(); view = { ...view, state: "expired" };
    open(); await start();
    await screen.findByText("The session ran out of time. Start a new one.");
    screen.getByRole("button", { name: "Start a helper session" });
  });

  it("explains an already-open session", async () => {
    createStatus = 409;
    open();
    fireEvent.click(await screen.findByRole("button", { name: "Start a helper session" }));
    await screen.findByText(/session is already open/);
  });

  it("cancels the session", async () => {
    open(); await start();
    fireEvent.click(screen.getByRole("button", { name: "Cancel session" }));
    await vi.waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.path === `/discovery/sessions/${SID}`)).toBe(true));
  });

  it("keeps checking by itself and shows a router once the helper has heard it", async () => {
    open(); await start();
    await screen.findByText(/Waiting for the helper/);
    expect(screen.queryByText("Nearby MikroTik router")).toBeNull();
    // The helper pairs and uploads; nobody touches the page.
    view = { ...view, state: "paired", pairing_expires_at: undefined, candidates: [CAND] };
    await screen.findByText("Nearby MikroTik router", {}, { timeout: 6000 });
    screen.getByText(/The helper is connected and listening/);
  }, 10000);

  it("stops checking once the session has ended", async () => {
    open(); await start();
    await screen.findByText(/Waiting for the helper/);
    view = { ...view, state: "cancelled" };
    await screen.findByText("The session was cancelled.", {}, { timeout: 6000 });
    const before = calls.filter((c) => c.method === "GET" && c.path === `/discovery/sessions/${SID}`).length;
    await new Promise((r) => setTimeout(r, 3600));
    expect(calls.filter((c) => c.method === "GET" && c.path === `/discovery/sessions/${SID}`).length).toBe(before);
  }, 15000);
});
