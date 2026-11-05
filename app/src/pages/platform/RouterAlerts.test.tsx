import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { RouterAlerts } from "./RouterAlerts";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
let state: { alerts: unknown[]; unread: number }, markStatus: number;

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); markStatus = 200;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    if (String(url).endsWith("/read") && init?.method === "POST") { if (markStatus === 200) state = { ...state, unread: 0 }; return json(markStatus, {}); }
    return json(200, state);
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><RouterAlerts /></I18nProvider></SWRConfig>);
const OFFLINE = { id: "1", kind: "offline", hardware_name: "Shop router", company_name: "Family", device_id: "d1", offline_since: "2026-11-06T01:00:00Z", detail: "Last contact earlier.", created_at: "2026-11-06T01:15:00Z" };

describe("Router alerts", () => {
  it("shows a drop with its router, customer and detail, then clears the unread state", async () => {
    state = { alerts: [OFFLINE, { id: "2", kind: "recovered", hardware_name: "Cafe", company_name: "Ax2", device_id: "d2", read_at: "2026-11-06T02:00:00Z" }], unread: 1 };
    open();
    await screen.findByText("Shop router");
    screen.getByText("Family"); screen.getByText("Last contact earlier."); screen.getByText("Offline"); screen.getByText("Back online");
    fireEvent.click(screen.getByRole("button", { name: "Mark all read" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Mark all read" })).toBeNull());
  });
  it("says so when there are no alerts", async () => {
    state = { alerts: [], unread: 0 };
    open();
    await screen.findByText("No router alerts yet.");
  });
  it("keeps the alerts when marking read fails", async () => {
    state = { alerts: [OFFLINE], unread: 1 }; markStatus = 500;
    open();
    fireEvent.click(await screen.findByRole("button", { name: "Mark all read" }));
    await screen.findByText("Could not mark the alerts read. Try again.");
    screen.getByText("Shop router");
  });
});
