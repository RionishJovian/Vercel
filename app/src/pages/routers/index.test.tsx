import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";

vi.mock("../../lib/useFleet", () => ({ useFleet: () => ({ data: { peers: [] } }) }));
import { Routers } from ".";

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
const GATEWAY = { device_id: "d1", hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY" };
const AP = { device_id: "d2", hardware_name: "Hall AP", deployment_layer: "ACCESS_POINT" };

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  vi.stubGlobal("fetch", vi.fn(async () => json({ routers: [GATEWAY, AP] })));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><Routers sub="" params={new URLSearchParams()} /></I18nProvider></SWRConfig>);

describe("Router page navigation", () => {
  it("links to Internet setup once, through the page tabs, and does not repeat that page's own menu", async () => {
    open();
    await screen.findByText("Shop gateway");
    expect(screen.getAllByRole("link", { name: "Internet setup" })).toHaveLength(1);
    // The Internet setup pages (WAN, DHCP, VLANs ...) are reached from that one link, not listed again here.
    expect(screen.queryByRole("link", { name: "New network setup" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Internet connections" })).toBeNull();
  });
  it("keeps one link each to Magic Hub, Devices and Topology", async () => {
    open();
    await screen.findByText("Shop gateway");
    expect(screen.getAllByRole("link", { name: /Magic Hub/ })).toHaveLength(1);
    expect(document.querySelectorAll('a[href="#/managed-devices"]')).toHaveLength(1);
    expect(document.querySelectorAll('a[href="#/topology"]')).toHaveLength(1);
  });
  it("has one Quick setup entry, in Router tools", async () => {
    open();
    await screen.findByText("Shop gateway");
    expect(screen.getAllByRole("link", { name: "Quick setup" })).toHaveLength(1);
  });
});
