import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { HubChip } from "../pages/routers/HubChip";
import { hubStatusFrom, hubStatusOf } from "./hubStatus";

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("the Router tab's label comes from the shared fleet status", () => {
  it("maps the shared status", () => {
    expect(hubStatusOf({ state: "up", source: "hub_tunnel", last_contact_at: "2026-11-02T01:00:00Z" })).toEqual({ state: "up", at: "2026-11-02T01:00:00Z" });
    expect(hubStatusOf({ state: "up", source: "router_read", last_contact_at: "2026-11-02T01:00:00Z" })).toEqual({ state: "up", at: undefined });
    expect(hubStatusOf({ state: "offline", source: "hub_tunnel" }).state).toBe("down");
    expect(hubStatusOf({ state: "stale" }).state).toBe("unknown");
    expect(hubStatusOf({ state: "unknown" }).state).toBe("unknown");
    expect(hubStatusOf(undefined).state).toBe("unknown");
    expect(hubStatusFrom({ peers: [{ device_id: "a", state: "up" }, { state: "up" }] })).toEqual({ a: { state: "up", at: undefined } });
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
