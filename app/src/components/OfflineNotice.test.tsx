import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { OfflineNotice } from "./OfflineNotice";

let online = true;
beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: () => {} });
  online = true;
  vi.spyOn(navigator, "onLine", "get").mockImplementation(() => online);
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("OfflineNotice", () => {
  it("keeps an empty live region while online and announces going offline and back", () => {
    render(<I18nProvider><OfflineNotice /></I18nProvider>);
    const region = screen.getByRole("status");
    expect(region.textContent).toBe("");
    act(() => { online = false; window.dispatchEvent(new Event("offline")); });
    expect(region.textContent).toMatch(/This device is offline/);
    act(() => { online = true; window.dispatchEvent(new Event("online")); });
    expect(region.textContent).toBe("");
  });
});
