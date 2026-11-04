import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { Capabilities } from "./Capabilities";

const ID = "7af55780-3e68-40cc-b4ae-e464b18c8961";
let mode: unknown;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ capabilities: { device_id: ID, detected: true, device_mode: mode } }), { status: 200, headers: { "content-type": "application/json" } })));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const read = async () => {
  render(<I18nProvider><Capabilities id={ID} /></I18nProvider>);
  fireEvent.click(screen.getByRole("button"));
  await screen.findByText("Device mode");
};
const warning = () => screen.queryByText(/turns off Hotspot or Scheduler/);

describe("Router tools device mode", () => {
  it("warns with the fix when device mode blocks what vouchers need", async () => {
    mode = { known: true, mode: "home", hotspot: false, scheduler: false };
    await read();
    expect(screen.getByText("home")).toBeTruthy();
    expect(warning()?.textContent).toContain("hotspot=yes scheduler=yes");
  });
  it("warns when only one feature is off", async () => {
    mode = { known: true, mode: "basic", hotspot: false, scheduler: true };
    await read();
    expect(warning()).toBeTruthy();
  });
  it("stays quiet when everything is allowed", async () => {
    mode = { known: true, mode: "advanced", hotspot: true, scheduler: true };
    await read();
    expect(warning()).toBeNull();
  });
  it("says unknown, without a warning, when the router cannot report it", async () => {
    mode = { known: false };
    await read();
    expect(warning()).toBeNull();
  });
});
