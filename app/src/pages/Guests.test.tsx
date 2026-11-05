import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { Guests } from "./Guests";
import type { Me } from "../lib/types";

const GW = "7af55780-3e68-40cc-b4ae-e464b18c8961";
const SESSION = { id: "*1", values: { user: "ABCDEFGH", address: "10.5.50.20", "mac-address": "AA:BB:CC:DD:EE:01", uptime: "00:05:12", "bytes-out": "321000", "bytes-in": "626000" } };
const SOON = { id: "*2", values: { user: "SOONSOON", address: "10.5.50.21", "mac-address": "AA:BB:CC:DD:EE:02", uptime: "5m", "bytes-out": "1", "bytes-in": "1" } };
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let observations: unknown, disconnectBody: unknown, vouchers: unknown[], voucherExtras: Record<string, unknown>, voucherStatus: number;

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const me = (role: string) => ({ user: { user_role: role } }) as unknown as Me;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("confirm", () => true);
  calls = []; disconnectBody = { verified: true }; vouchers = []; voucherExtras = {}; voucherStatus = 200;
  observations = { guest_actions_enabled: true, inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] } } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path === "/routers") return json(200, { routers: [{ device_id: GW, hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY", site_id: "s1" }] });
    if (path === "/sites") return json(200, { sites: [{ site_id: "s1", name: "Main shop" }] });
    if (path.endsWith("/observations")) return json(200, observations);
    if (path.endsWith("/guests/disconnect") || path.endsWith("/guests/block") || path.endsWith("/guests/unblock")) return json(200, disconnectBody);
    if (path.startsWith("/vouchers?")) return json(voucherStatus, { vouchers, ...voucherExtras });
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (role: string) => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Guests me={me(role)} /></I18nProvider></SWRConfig>);
const disconnectCalls = () => calls.filter((c) => c.url.endsWith("/guests/disconnect"));
const button = (name: string) => screen.getByRole("button", { name }) as HTMLButtonElement;

describe("Live users", () => {
  it("lists the sessions the gateway reported, one row each", async () => {
    open("OWNER");
    expect(await screen.findByText("ABCDEFGH")).toBeTruthy();
    expect(screen.getByText("10.5.50.20")).toBeTruthy();
    expect(screen.getByText("AA:BB:CC:DD:EE:01")).toBeTruthy();
    expect(screen.getByText("5m12s")).toBeTruthy();
    expect(screen.getByText("321 KB ↓ / 626 KB ↑")).toBeTruthy();
    expect(screen.getByText(/Main shop · Updated/)).toBeTruthy();
  });

  it("shows time left from the app's voucher record, and no time when there is none", async () => {
    vouchers = [{ code: "SOONSOON", expires_at: new Date(Date.now() + 10 * 60000).toISOString() }];
    observations = { guest_actions_enabled: true, inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION, SOON] } } };
    open("OWNER");
    await screen.findByText("SOONSOON");
    await waitFor(() => expect(screen.getByRole("button", { name: /^Near expiry · 1/ })).toBeTruthy());
    expect(screen.getByRole("button", { name: /^Active · 1/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /^All · 2/ })).toBeTruthy();
    expect((screen.getByText("ABCDEFGH").closest("tr") as HTMLElement).textContent).toContain("—");
    fireEvent.click(screen.getByRole("button", { name: /^Near expiry · 1/ }));
    expect(screen.queryByText("ABCDEFGH")).toBeNull();
  });

  it("shows a dash when the router gave no idle or binding read, and Rate stays unavailable", async () => {
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    for (const name of [/^Idle · —/, /^Banned MAC · —/, /^Rate$/]) expect((screen.getByRole("button", { name }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("counts idle sessions and takes time left from the router when the app has no expiry", async () => {
    observations = { guest_actions_enabled: true, inventory: { available: { hotspot_sessions: true, hotspot_session_times: true }, objects: { hotspot_sessions: [SESSION, SOON], hotspot_session_times: [{ id: "*1", values: { "idle-time": "00:06:00", "session-time-left": "1h" } }, { id: "*2", values: { "idle-time": "10s", "session-time-left": "" } }] } } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(await screen.findByRole("button", { name: /^Idle · 1/ }));
    expect(screen.queryByText("SOONSOON")).toBeNull();
    expect((screen.getByText("ABCDEFGH").closest("tr") as HTMLElement).textContent).toContain("1h");
  });

  it("counts a code over its data limit from saved usage", async () => {
    vouchers = [{ id: "v1", code: "ABCDEFGH" }];
    voucherExtras = { usage: { v1: { bytes_used: 600_000_000 } }, data_limits_mb: { v1: 500 } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    await screen.findByRole("button", { name: /^Over quota · 1/ });
    expect(within(screen.getByText("ABCDEFGH").closest("tr") as HTMLElement).getByText("Over quota")).toBeTruthy();
  });

  it("leaves over quota unknown when the voucher records could not be read", async () => {
    voucherStatus = 503;
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    await screen.findByRole("button", { name: /^Over quota · —/ });
  });

  it("Ban MAC asks first, sends the MAC, and says the router confirmed", async () => {
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    vi.stubGlobal("confirm", () => false);
    fireEvent.click(button("Ban MAC"));
    expect(calls.some((c) => c.url.endsWith("/guests/block"))).toBe(false);
    vi.stubGlobal("confirm", () => true);
    fireEvent.click(button("Ban MAC"));
    await waitFor(() => expect(calls.find((c) => c.url.endsWith("/guests/block"))?.body).toMatchObject({ mac: "AA:BB:CC:DD:EE:01", confirm: true }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Device blocked and confirmed by the router."));
  });

  it("does not claim a ban unless the router confirmed it", async () => {
    disconnectBody = { verified: false };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(button("Ban MAC"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Could not change the block."));
  });

  it("lists blocks, and only offers to remove the app's own", async () => {
    const ours = { id: "*5", values: { "mac-address": "AA:BB:CC:DD:EE:05", type: "blocked", comment: "mega-magic:ban", disabled: "false" } };
    const theirs = { id: "*6", values: { "mac-address": "AA:BB:CC:DD:EE:06", type: "blocked", comment: "by hand", disabled: "false" } };
    const bypass = { id: "*7", values: { "mac-address": "AA:BB:CC:DD:EE:07", type: "bypassed", comment: "", disabled: "false" } };
    const off = { id: "*8", values: { "mac-address": "AA:BB:CC:DD:EE:08", type: "blocked", comment: "", disabled: "true" } };
    observations = { guest_actions_enabled: true, inventory: { available: { hotspot_sessions: true, hotspot_bindings: true }, objects: { hotspot_sessions: [SESSION], hotspot_bindings: [ours, theirs, bypass, off] } } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(screen.getByRole("button", { name: /^Banned MAC · 2/ }));
    expect(screen.queryByText("AA:BB:CC:DD:EE:07")).toBeNull();
    const row = (mac: string) => screen.getByText(mac).closest("tr") as HTMLElement;
    expect((within(row("AA:BB:CC:DD:EE:06")).getByRole("button", { name: "Remove block" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(within(row("AA:BB:CC:DD:EE:05")).getByRole("button", { name: "Remove block" }));
    await waitFor(() => expect(calls.find((c) => c.url.endsWith("/guests/unblock"))?.body).toMatchObject({ mac: "AA:BB:CC:DD:EE:05" }));
  });

  it("Kick sends the session's identity", async () => {
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(button("Kick"));
    await waitFor(() => expect(disconnectCalls()).toHaveLength(1));
    expect(disconnectCalls()[0].body).toMatchObject({ id: "*1", mac: "AA:BB:CC:DD:EE:01", address: "10.5.50.20", user: "ABCDEFGH" });
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Session disconnected and confirmed by the router."));
  });

  it("does not claim success unless the router confirmed the disconnect", async () => {
    disconnectBody = { verified: false };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(button("Kick"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Could not disconnect."));
    expect(screen.getByRole("status").textContent).not.toContain("confirmed by the router");
  });

  it("asks before disconnecting and does nothing when declined", async () => {
    vi.stubGlobal("confirm", () => false);
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(button("Kick"));
    await new Promise((r) => setTimeout(r, 20));
    expect(disconnectCalls()).toHaveLength(0);
  });

  it("kicks the selected sessions and counts only what the router confirmed", async () => {
    observations = { guest_actions_enabled: true, inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION, SOON] } } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(screen.getByRole("checkbox", { name: "Select all shown" }));
    fireEvent.click(button("Kick (2)"));
    await waitFor(() => expect(disconnectCalls()).toHaveLength(2));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("2 of 2 disconnected"));
  });

  it("copies the voucher, IP and MAC", async () => {
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(button("Copy"));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("ABCDEFGH\t10.5.50.20\tAA:BB:CC:DD:EE:01"));
  });

  // What the server sends today: /observations forces write_enabled=false and disconnect answers 503.
  it("disables Kick and says why when the server holds router writes", async () => {
    observations = { guest_actions_enabled: false, inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] } } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    expect(button("Kick").disabled).toBe(true);
    expect(screen.getByText(/Kick and Ban MAC are not available yet/)).toBeTruthy();
    fireEvent.click(button("Kick"));
    expect(disconnectCalls()).toHaveLength(0);
  });

  it("reports a failure, not success, when the server refuses the disconnect", async () => {
    const real = globalThis.fetch;
    vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => (String(url).endsWith("/guests/disconnect") ? json(503, { error: "Guest disconnection is unavailable", code: "guest_disconnect_unavailable", verified: false }) : real(url, init))));
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    fireEvent.click(button("Kick"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Could not disconnect."));
    expect(screen.getByRole("status").textContent).not.toContain("confirmed by the router");
  });

  it("disables Kick for a viewer and when writes are held", async () => {
    open("VIEWER");
    await screen.findByText("ABCDEFGH");
    expect(button("Kick").disabled).toBe(true);
    cleanup();
    observations = { guest_actions_enabled: false, inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] } } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    expect(button("Kick").disabled).toBe(true);
  });

  it("never shows a session count when a gateway could not be read", async () => {
    observations = { inventory: { available: { hotspot_sessions: false } } };
    open("OWNER");
    expect(await screen.findByText(/could not report/i)).toBeTruthy();
    expect(screen.queryByText("ABCDEFGH")).toBeNull();
    expect(screen.getByRole("button", { name: /^All · —/ })).toBeTruthy();
  });
});
