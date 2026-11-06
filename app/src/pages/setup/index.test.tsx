import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";

vi.mock("../../lib/useFleet", () => ({ useFleet: () => ({ data: { peers: [{ device_id: "d1", state: "up" }] }, mutate: vi.fn() }) }));
import { QuickSetup } from ".";

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
let data: { sites: unknown[]; routers: unknown[]; profiles?: string[]; profilesFail?: boolean };
beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const p = String(url).replace("/api/v1", "");
    if (p === "/sites") return json({ sites: data.sites });
    if (p === "/routers") return json({ routers: data.routers });
    if (p === "/vps/enrollment-capability") return json({ available: true });
    if (p.endsWith("/portal/profiles")) return data.profilesFail ? new Response("{}", { status: 502, headers: { "content-type": "application/json" } }) : json({ profiles: data.profiles ?? [] });
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
  const CONNECTED = { device_id: "d1", deployment_layer: "MAIN_GATEWAY", management_connected: true };
  const SITE = { site_id: "s1", name: "Shop" };
  it("offers the Hotspot network step once the router is connected and has no Hotspot profile", async () => {
    data = { sites: [SITE], routers: [CONNECTED], profiles: [] };
    open();
    await screen.findByText("The router answers through Magic Hub right now.", { exact: false });
    await screen.findByRole("button", { name: "Check router and preview changes" });
    expect(screen.queryByText("Next: Guest page")).toBeNull();
  });
  it("marks Hotspot done only when the router reports a profile, then points to the guest page", async () => {
    data = { sites: [SITE], routers: [CONNECTED], profiles: ["hsprof1"] };
    open();
    await screen.findByText("The router has a Hotspot login profile.");
    expect(screen.getByRole("link", { name: "Next: Guest page" }).getAttribute("href")).toBe("#/vouchers/guest");
  });
  it("does not call Hotspot done when the profiles cannot be read", async () => {
    data = { sites: [SITE], routers: [CONNECTED], profilesFail: true };
    open();
    await screen.findByText(/Could not read the router's Hotspot profiles/);
    expect(screen.queryByText("Next: Guest page")).toBeNull();
  });
});
