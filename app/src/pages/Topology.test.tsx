import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";

let fleet: { data?: unknown };
vi.mock("../lib/useFleet", () => ({ useFleet: () => fleet }));
import { Topology } from "./Topology";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const GATEWAY = { device_id: "d1", hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY" };

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/topology/map") return json(200, { roots: [{ device: GATEWAY }] });
    // The router answers a read, whatever the shared status says.
    if (path.endsWith("/observations")) return json(200, { inventory: { observed_at: "2026-11-06T01:00:00Z", health: { version: "7.25" }, available: { interfaces: true } } });
    if (path === "/sites") return json(200, { sites: [] });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><Topology me={{ user: { user_role: "VIEWER" } } as never} /></I18nProvider></SWRConfig>);

describe("Topology router status", () => {
  it("shows the same state, source and freshness as Fleet health", async () => {
    fleet = { data: { peers: [{ device_id: "d1", state: "up", source: "hub_tunnel", last_contact_at: "2026-11-06T01:00:00Z", checked_at: "2026-11-06T01:01:00Z" }] } };
    open();
    await screen.findByText("Magic Hub tunnel");
    expect(screen.getAllByText("Online").length).toBeGreaterThan(0);
    screen.getByText("Last contact"); screen.getByText("Checked");
    expect(screen.queryByText("Management reachable")).toBeNull();
  });
  it("offers both Map and Tree views and persists the chosen view", async () => {
    sessionStorage.removeItem("mm_topology_view");
    fleet = { data: { peers: [{ device_id: "d1", state: "up", source: "hub_tunnel" }] } };
    const rendered = open();
    const map = await screen.findByRole("button", { name: "Map" });
    const tree = screen.getByRole("button", { name: "Tree" });
    expect(map.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(tree);
    expect(tree.getAttribute("aria-pressed")).toBe("true");
    expect(map.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText(/Unknown attachments are never invented/)).toBeTruthy();
    expect(sessionStorage.getItem("mm_topology_view")).toBe("tree");
    rendered.unmount();
    open();
    expect((await screen.findByRole("button", { name: "Tree" })).getAttribute("aria-pressed")).toBe("true");
    sessionStorage.removeItem("mm_topology_view");
  });

  it("does not call a router Online just because a read worked", async () => {
    fleet = { data: { peers: [{ device_id: "d1", state: "unknown", source: "none" }] } };
    open();
    await screen.findByText("No signal yet");
    expect(screen.getAllByText("Unknown").length).toBeGreaterThan(0);
    expect(screen.queryByText("Online")).toBeNull();
  });
});
