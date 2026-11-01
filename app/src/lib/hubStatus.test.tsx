import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { HubChip } from "../pages/routers/HubChip";
import { hubStatusOf } from "./hubStatus";

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("router online label comes from the Magic Hub handshake", () => {
  it("maps the lookup result", () => {
    expect(hubStatusOf({ handshake_status: "recent_handshake", last_handshake_at: "2026-11-02T01:00:00Z" })).toEqual({ state: "up", at: "2026-11-02T01:00:00Z" });
    expect(hubStatusOf({ handshake_status: "no_recent_handshake" }).state).toBe("down");
    expect(hubStatusOf({ handshake_status: "inactive" }).state).toBe("down");
    expect(hubStatusOf({}).state).toBe("unknown");
    expect(hubStatusOf(null).state).toBe("unknown");
  });
  it("shows Online, Offline, and never claims Offline when the lookup is unavailable", () => {
    const show = (status?: Parameters<typeof HubChip>[0]["status"]) => { cleanup(); render(<I18nProvider><HubChip status={status} /></I18nProvider>); return document.body.textContent; };
    expect(show({ state: "up" })).toBe("Online");
    expect(show({ state: "down" })).toBe("Offline");
    expect(show({ state: "unknown" })).toBe("Status unavailable");
    expect(show(undefined)).toBe("Checking…");
    expect(screen.queryByText("Offline")).toBeNull();
  });
});
