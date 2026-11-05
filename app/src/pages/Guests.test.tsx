import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { Guests } from "./Guests";
import type { Me } from "../lib/types";

const GW = "7af55780-3e68-40cc-b4ae-e464b18c8961";
const SESSION = { id: "*1", values: { user: "ABCDEFGH", address: "10.5.50.20", "mac-address": "AA:BB:CC:DD:EE:01", uptime: "00:05:12", "bytes-out": "321000", "bytes-in": "626000" } };
const SOON = { id: "*2", values: { user: "SOONSOON", address: "10.5.50.21", "mac-address": "AA:BB:CC:DD:EE:02", uptime: "5m", "bytes-out": "1", "bytes-in": "1" } };
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let observations: unknown, disconnectBody: unknown, vouchers: unknown[];

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const me = (role: string) => ({ user: { user_role: role } }) as unknown as Me;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("confirm", () => true);
  calls = []; disconnectBody = { verified: true }; vouchers = [];
  observations = { inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] }, write_enabled: true } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path === "/routers") return json(200, { routers: [{ device_id: GW, hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY", site_id: "s1" }] });
    if (path === "/sites") return json(200, { sites: [{ site_id: "s1", name: "Main shop" }] });
    if (path.endsWith("/observations")) return json(200, observations);
    if (path.endsWith("/guests/disconnect")) return json(200, disconnectBody);
    if (path.startsWith("/vouchers?")) return json(200, { vouchers });
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
    observations = { inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION, SOON] }, write_enabled: true } };
    open("OWNER");
    await screen.findByText("SOONSOON");
    await waitFor(() => expect(screen.getByRole("button", { name: /^Near expiry · 1/ })).toBeTruthy());
    expect(screen.getByRole("button", { name: /^Active · 1/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /^All · 2/ })).toBeTruthy();
    expect((screen.getByText("ABCDEFGH").closest("tr") as HTMLElement).textContent).toContain("—");
    fireEvent.click(screen.getByRole("button", { name: /^Near expiry · 1/ }));
    expect(screen.queryByText("ABCDEFGH")).toBeNull();
  });

  it("does not invent idle, over-quota or banned counts, or working Ban and Rate actions", async () => {
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    for (const name of [/^Idle · —/, /^Over quota · —/, /^Banned MAC · —/, /^Ban MAC$/, /^Rate$/]) expect((screen.getByRole("button", { name }) as HTMLButtonElement).disabled).toBe(true);
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
    observations = { inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION, SOON] }, write_enabled: true } };
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

  it("disables Kick for a viewer and when writes are held", async () => {
    open("VIEWER");
    await screen.findByText("ABCDEFGH");
    expect(button("Kick").disabled).toBe(true);
    cleanup();
    observations = { inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] }, write_enabled: false } };
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
