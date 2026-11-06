import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";

vi.mock("../../lib/useFleet", () => ({ useFleet: () => ({ data: { peers: [{ device_id: "d1", state: "up" }] }, mutate: vi.fn() }) }));
import { QuickSetup } from ".";

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
let data: { sites: unknown[]; routers: unknown[] };
beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const p = String(url).replace("/api/v1", "");
    if (p === "/sites") return json({ sites: data.sites });
    if (p === "/routers") return json({ routers: data.routers });
    if (p === "/vps/enrollment-capability") return json({ available: true });
    return json({});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = () => render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><QuickSetup me={{ user: { user_role: "OWNER" } } as never} /></I18nProvider></SWRConfig>);

describe("Quick setup", () => {
  it("sends you to add a site first", async () => {
    data = { sites: [], routers: [] };
    open();
    await screen.findByText(/Add a site first/);
    expect(screen.queryByText("Next: Hotspot network")).toBeNull();
  });
  it("offers to add the router once a site exists", async () => {
    data = { sites: [{ site_id: "s1", name: "Shop" }], routers: [] };
    open();
    await screen.findByRole("heading", { name: "Add RouterBOARD" });
  });
  it("shows the next step only after management access is verified", async () => {
    data = { sites: [{ site_id: "s1", name: "Shop" }], routers: [{ device_id: "d1", deployment_layer: "MAIN_GATEWAY", management_connected: true }] };
    open();
    await screen.findByText("Next: Hotspot network");
    screen.getByText("The router answers through Magic Hub right now.");
  });
});
