import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "./i18n";
import { ThemeProvider } from "./lib/theme";
import { App } from "./App";

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
const open = (hash: string) => { location.hash = hash; return render(<ThemeProvider><I18nProvider><SWRConfig value={{ provider: () => new Map(), shouldRetryOnError: false }}><App /></SWRConfig></I18nProvider></ThemeProvider>); };

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    if (String(url).endsWith("/me")) return json({ user: { user_role: "OWNER", full_name: "Aye", email: "a@example.com" } });
    if (String(url).endsWith("/profile")) return json({ full_name: "Aye", email: "a@example.com", user_role: "OWNER" });
    return json({});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); location.hash = ""; document.title = ""; });

describe("page titles", () => {
  it("names the page in the browser tab", async () => {
    open("#/backups");
    await waitFor(() => expect(document.title).toBe("Backups · MegaMagic"));
  });
  it("names the network map in the browser tab and on the page", async () => {
    open("#/router/map");
    await waitFor(() => expect(document.title).toBe("Network map · MegaMagic"));
    await screen.findByRole("heading", { level: 1, name: "Network map" });
  });
  it("calls the More tab More", async () => {
    open("#/more");
    await screen.findByRole("heading", { level: 1, name: "More" });
  });
  it("names Your profile even when the person has no name", async () => {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => (String(url).endsWith("/me") ? json({ user: { user_role: "OWNER" } }) : json({ email: "a@example.com", user_role: "OWNER" }))));
    open("#/profile");
    await screen.findByRole("heading", { level: 1, name: "Your profile" });
  });
});
