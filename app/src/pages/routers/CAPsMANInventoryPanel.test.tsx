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
  render(<I18nProvider><CAPsMANInventoryPanel id={ID} /></I18nProvider>);
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
