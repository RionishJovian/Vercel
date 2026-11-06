import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { AddGateway } from ".";

vi.mock("../../lib/useFleet", () => ({ useFleet: () => ({ data: { peers: [] }, mutate: vi.fn() }) }));
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const SID = "0a1b2c3d-0000-4000-8000-000000000001", KEY = "sw:0123456789abcdef";
let calls: Array<{ method: string; path: string; body?: unknown }>;

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); calls = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", ""), method = init?.method ?? "GET";
    calls.push({ method, path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path === "/discovery/capability") return json(200, { enabled: true });
    if (path === "/discovery/sessions" && method === "POST") return json(201, { session_id: SID, pairing_code: "ABCDEF-GHJKMNPQ" });
    if (path === `/discovery/sessions/${SID}` && method === "GET") return json(200, { session_id: SID, state: "paired", expires_at: "2099-01-01T00:00:00Z", candidates: [{ device_key: KEY, macs: ["48:A9:8A:00:00:06"], identity: "hAP", board: "C52iG-5HaxD2HaxD", platform: "MikroTik", version: "7.25", claimed_ip: "192.168.88.1", source_ip: "192.168.88.1", heard_count: 1, last_seen: "2026-11-06T02:00:00Z", selected: false, status: "nearby_unverified" }] });
    if (path.endsWith("/select")) return json(200, { device_key: KEY, suggested_name: "hAP", device_model: "C52iG-5HaxD2HaxD", local_address: "192.168.88.1", site_id: "s1" });
    if (path === "/routers" && method === "POST") return json(201, { router: { device_id: "r1" } });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("Add RouterBOARD with nearby discovery", () => {
  it("prefills from the selection, registers through the existing flow, then remembers the link", async () => {
    const onDone = vi.fn();
    render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><AddGateway sites={[{ site_id: "s1", name: "Shop" }]} onDone={onDone} /></I18nProvider></SWRConfig>);
    fireEvent.click(screen.getByRole("button", { name: "Show" }));
    fireEvent.click(await screen.findByRole("button", { name: "Start a helper session" }));
    await screen.findByText("Nearby MikroTik router");
    fireEvent.click(screen.getByRole("button", { name: "Select router" }));
    await screen.findByText(/Filled in from a nearby router you selected/);
    expect((screen.getByLabelText("RouterBOARD name") as HTMLInputElement).value).toBe("hAP");
    // Nothing is registered by selecting.
    expect(calls.some((c) => c.method === "POST" && c.path === "/routers")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Add RouterBOARD" }));
    await vi.waitFor(() => expect(onDone).toHaveBeenCalledWith("r1"));
    const create = calls.find((c) => c.method === "POST" && c.path === "/routers");
    expect(create?.body).toMatchObject({ hardware_name: "hAP", mac_address: null, management_ip: null, transport: "tunnel" });
    expect(calls.find((c) => c.path === `/discovery/sessions/${SID}/link`)?.body).toEqual({ device_key: KEY, router_id: "r1" });
  });
});
