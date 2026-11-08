import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { CAPsMANInventoryPanel } from "./CAPsMANInventoryPanel";

const ID = "7af55780-3e68-40cc-b4ae-e464b18c8961";
let inventory: unknown;
beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; } });
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ inventory }), {
    status: 200, headers: { "content-type": "application/json" },
  })));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function load() {
  render(<I18nProvider><CAPsMANInventoryPanel id={ID} siteId="site-a" canReview /></I18nProvider>);
  fireEvent.click(screen.getByRole("button", { name: "Read Wi-Fi inventory" }));
}
describe("CAPsMAN evidence", () => {
  it("separates unverified broadcast from configured SSID and unknown CAP count", async () => {
    inventory = {
      observed_at: "2026-11-08T10:00:00Z", write_enabled: false, wifi_write_enabled: false,
      capsman: { write_enabled: false, controllers: [
        { family: "modern", inventory_available: true, radio_inventory_available: false,
          caps: [{ identity: "shop-ap", state_known: false, online_verified: false, ssid_verified: false }] },
        { family: "legacy", inventory_available: false, radio_inventory_available: false, caps: [] },
      ], profiles: [{ family: "modern", name: "guest", ssid: "Shop WiFi", broadcasting_verified: false }] },
    };
    load();
    expect(await screen.findByText("shop-ap")).toBeTruthy();
    expect(screen.getByText(/Shop WiFi/).textContent).toContain("Broadcast unverified");
    expect(screen.getByText(/AP count unknown/)).toBeTruthy();
    expect(screen.getByText(/SSID synchronization and AP configuration are not enabled/)).toBeTruthy();
  });
  it("calls the owner-only site-scoped server preview without applying anything", async () => {
    const calls: Array<{ url: string; method: string; body?: { site_id?: string; ssid?: string } }> = [];
    vi.stubGlobal("fetch", vi.fn(async (url: string, options?: RequestInit) => {
      calls.push({ url, method: options?.method ?? "GET", body: options?.body ? JSON.parse(String(options.body)) : undefined });
      if (String(url).endsWith("/observations")) return new Response(JSON.stringify({ inventory: {
        write_enabled: false, wifi_write_enabled: false, capsman: {
          write_enabled: false, controllers: [{ family: "modern", inventory_available: true,
            radio_inventory_available: true, caps: [{ identity: "lobby-cap" }] }],
          profiles: [] } } }), { status: 200, headers: { "content-type": "application/json" } });
      return new Response(JSON.stringify({ gateway_id: ID, apply_available: false, preview: {
        site_id: "site-a", ssid: "Magic Guest", revision: "read-only-revision",
        review_hash: "f".repeat(64), write_enabled: false, eligible_count: 0, blocked_count: 1,
        impacts: [{ device_id: "modern:cap-1", status: "blocked", reason: "unverified",
          target_ssid: "Magic Guest" }],
      } }), { status: 200, headers: { "content-type": "application/json" } });
    }));
    render(<I18nProvider><CAPsMANInventoryPanel id={ID} siteId="site-a" canReview /></I18nProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Read Wi-Fi inventory" }));
    expect(await screen.findByText("lobby-cap")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Proposed Wi-Fi name" }), { target: { value: "Magic Guest" } });
    fireEvent.click(screen.getByRole("button", { name: "Review impact on server (no changes)" }));
    expect(await screen.findByText(/APs eligible for change: 0/)).toBeTruthy();
    expect(screen.getByText(/Blocked — unverified management capability/)).toBeTruthy();
    expect(calls.length).toBe(2);
    expect(calls[1].url).toContain("/routers/" + ID + "/ssid/preview");
    expect(calls[1].method).toBe("POST");
    expect(calls[1].body?.site_id).toBe("site-a");
    expect(calls[1].body?.ssid).toBe("Magic Guest");
    expect(screen.queryByRole("button", { name: /apply|approve|save/i })).toBeNull();
  });
  it("prevents a viewer or an unassigned gateway from requesting a preview", async () => {
    inventory = { write_enabled: false, wifi_write_enabled: false, capsman: {
      write_enabled: false, controllers: [], profiles: [] } };
    render(<I18nProvider><CAPsMANInventoryPanel id={ID} canReview={false} /></I18nProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Read Wi-Fi inventory" }));
    await screen.findByText(/No readable profile records/);
    fireEvent.change(screen.getByRole("textbox", { name: "Proposed Wi-Fi name" }), { target: { value: "Magic Guest" } });
    expect((screen.getByRole("button", { name: "Review impact on server (no changes)" }) as HTMLButtonElement).disabled).toBe(true);
    expect(vi.mocked(fetch).mock.calls.length).toBe(1);
  });
  it("disables invalid oversized UTF-8 proposed SSIDs", async () => {
    inventory = { write_enabled: false, wifi_write_enabled: false, capsman: {
      write_enabled: false, controllers: [], profiles: [] } };
    render(<I18nProvider><CAPsMANInventoryPanel id={ID} siteId="site-a" canReview /></I18nProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Read Wi-Fi inventory" }));
    await screen.findByText(/No readable profile records/);
    fireEvent.change(screen.getByRole("textbox", { name: "Proposed Wi-Fi name" }), { target: { value: "မြန်မာအမည်ရှည်ရှည်" } });
    expect((screen.getByRole("button", { name: "Review impact on server (no changes)" }) as HTMLButtonElement).disabled).toBe(true);
  });
  it("refuses inventory evidence that reports enabled writes", async () => {
    inventory = { write_enabled: true, wifi_write_enabled: false, capsman: { write_enabled: false, controllers: [], profiles: [] } };
    load();
    expect(await screen.findByText(/CAPsMAN observations are unavailable/)).toBeTruthy();
    expect(screen.queryByText("Configured SSID profiles")).toBeNull();
  });
  it("does not invent APs from empty available controller", async () => {
    inventory = { write_enabled: false, wifi_write_enabled: false, capsman: { write_enabled: false,
      controllers: [{ family: "modern", inventory_available: true, radio_inventory_available: true, caps: [] }],
      profiles: [] } };
    load();
    expect(await screen.findByText(/0 CAP records reported/)).toBeTruthy();
    expect(screen.getByText(/No readable profile records/)).toBeTruthy();
  });
});
