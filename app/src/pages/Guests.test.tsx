import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { Guests } from "./Guests";
import type { Me } from "../lib/types";

const GW = "7af55780-3e68-40cc-b4ae-e464b18c8961";
const SESSION = { id: "*1", values: { user: "ABCDEFGH", address: "10.5.50.20", "mac-address": "AA:BB:CC:DD:EE:01", uptime: "5m" } };
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let observations: unknown, disconnectBody: unknown;

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const me = (role: string) => ({ user: { user_role: role } }) as unknown as Me;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("confirm", () => true);
  calls = []; disconnectBody = { verified: true };
  observations = { inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] }, write_enabled: true } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path === "/routers") return json(200, { routers: [{ device_id: GW, hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY", site_id: "s1" }] });
    if (path === "/sites") return json(200, { sites: [{ site_id: "s1", name: "Main shop" }] });
    if (path.endsWith("/observations")) return json(200, observations);
    if (path.endsWith("/guests/disconnect")) return json(200, disconnectBody);
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (role: string) => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Guests me={me(role)} /></I18nProvider></SWRConfig>);
const disconnectCalls = () => calls.filter((c) => c.url.endsWith("/guests/disconnect"));

describe("Guests", () => {
  it("lists the active sessions the gateway reported", async () => {
    open("OWNER");
    expect(await screen.findByText("ABCDEFGH")).toBeTruthy();
    expect(screen.getByText(/10\.5\.50\.20/)).toBeTruthy();
    expect(screen.getByText("Active at last read")).toBeTruthy();
  });

  it("offers Disconnect to an owner and sends the session's identity", async () => {
    open("OWNER");
    fireEvent.click(await screen.findByRole("button", { name: "Disconnect" }));
    await waitFor(() => expect(disconnectCalls()).toHaveLength(1));
    expect(disconnectCalls()[0].body).toMatchObject({ id: "*1", mac: "AA:BB:CC:DD:EE:01", address: "10.5.50.20", user: "ABCDEFGH" });
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Session disconnected and confirmed by the router."));
  });

  it("does not claim success unless the router confirmed the disconnect", async () => {
    disconnectBody = { verified: false };
    open("OWNER");
    fireEvent.click(await screen.findByRole("button", { name: "Disconnect" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Could not disconnect."));
    expect(screen.getByRole("status").textContent).not.toContain("confirmed by the router");
  });

  it("asks before disconnecting and does nothing when declined", async () => {
    vi.stubGlobal("confirm", () => false);
    open("OWNER");
    fireEvent.click(await screen.findByRole("button", { name: "Disconnect" }));
    await new Promise((r) => setTimeout(r, 20));
    expect(disconnectCalls()).toHaveLength(0);
  });

  it("shows sessions read-only to a viewer and when writes are held", async () => {
    open("VIEWER");
    await screen.findByText("ABCDEFGH");
    expect(screen.queryByRole("button", { name: "Disconnect" })).toBeNull();
    cleanup();
    observations = { inventory: { available: { hotspot_sessions: true }, objects: { hotspot_sessions: [SESSION] }, write_enabled: false } };
    open("OWNER");
    await screen.findByText("ABCDEFGH");
    expect(screen.queryByRole("button", { name: "Disconnect" })).toBeNull();
  });

  it("never shows a session count when a gateway could not be read", async () => {
    observations = { inventory: { available: { hotspot_sessions: false } } };
    open("OWNER");
    expect(await screen.findByText(/could not|unavailable|not available/i)).toBeTruthy();
    expect(screen.queryByText("ABCDEFGH")).toBeNull();
  });
});
