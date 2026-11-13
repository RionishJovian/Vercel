import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Hub } from "./index";
import { Enrollment } from "./Enrollment";
import { MARK, withNewRouterNetwork } from "./newRouterNetwork";

const SITE = { site_id: "bbbbbbbb-0000-4000-8000-000000000001", name: "Main shop" };
const A = { device_id: "aaaaaaaa-0000-4000-8000-000000000001", hardware_name: "Gateway A", deployment_layer: "MAIN_GATEWAY", transport: "tunnel", site_id: SITE.site_id, management_connected: true };
const B = { device_id: "aaaaaaaa-0000-4000-8000-000000000002", hardware_name: "Gateway B", deployment_layer: "MAIN_GATEWAY", transport: "tunnel", site_id: SITE.site_id };
const SCRIPT = "{\n:put \"check\"\n# APPLY: tunnel\n:put \"apply\"\n}\n";
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const capacity = (over: Record<string, unknown> = {}) => ({ included: 1, legacy_router_grants: 0, purchased: 0, operator_granted: 0, capacity: 1, used: 1, available: 0, exempt: false, over_capacity: false, purchases: [], ...over });
let routers: unknown[], cap: unknown, posts: Array<{ url: string; body: Record<string, unknown> }>, routerAnswer: { status: number; body: unknown }, enrollHeld: Promise<void> | null;

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); posts = []; routers = [A]; cap = capacity(); enrollHeld = null;
  routerAnswer = { status: 201, body: { router: B } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", "");
    if (init?.method === "POST") {
      const body = init.body ? JSON.parse(String(init.body)) : {};
      posts.push({ url: path, body });
      if (path === "/routers") return json(routerAnswer.status, routerAnswer.body);
      if (path.endsWith("/quick-enroll")) {
        if (enrollHeld) await enrollHeld;
        return json(200, { state: "hub_peer_enrolled", router_configuration_required: true, one_paste: true, automatic_management: true, router_script: SCRIPT, recovery: body.start_over ? "start_over" : "resume" });
      }
    }
    if (path === "/routers") return json(200, { routers });
    if (path === "/sites") return json(200, { sites: [SITE] });
    if (path === "/gateway-capacity") return json(200, { capacity: cap });
    if (path === "/vps/enrollment-capability") return json(200, { available: true, lookup_enabled: false });
    if (path.endsWith("/live")) return json(200, { connection: { management: "missing" } });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const openHub = () => render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><Hub me={{ user: { user_role: "OWNER" } } as never} /></SWRConfig></I18nProvider>);

describe("paid additional gateways", () => {
  it("shows the slots in use and offers a purchase instead of the form when all are used", async () => {
    cap = capacity({ purchases: [{ request_id: "r1", name_en: "Extra gateway", name_my: "Gateway", status: "pending", fulfillment: "not_applicable", submitted_at: "2026-11-13T00:00:00Z" }] });
    openHub();
    await screen.findByText("1 of 1 in use");
    expect(screen.getByText("Payment under review")).toBeTruthy();
    expect(screen.getByText(/All gateway slots are in use\. Buy another gateway slot/)).toBeTruthy();
    expect(screen.queryByLabelText("RouterBOARD name")).toBeNull();
    expect(screen.getAllByRole("link", { name: "Buy a gateway slot" })[0].getAttribute("href")).toBe("/app/store");
  });

  it("adds gateway B to the existing site in ONE request once a paid slot is fulfilled, keeping A", async () => {
    cap = capacity({ purchased: 1, capacity: 2, available: 1, purchases: [{ request_id: "r1", name_en: "Extra gateway", name_my: "Gateway", status: "approved", fulfillment: "fulfilled", submitted_at: "2026-11-13T00:00:00Z" }] });
    openHub();
    await screen.findByText("1 of 2 in use");
    expect(screen.getByText("Slot added")).toBeTruthy();
    fireEvent.change(await screen.findByLabelText(/name/i, { selector: "#gw-name" }), { target: { value: "Gateway B" } });
    fireEvent.change(screen.getByLabelText("Site", { selector: "#gw-site" }), { target: { value: SITE.site_id } });
    routers = [A, B]; cap = capacity({ purchased: 1, capacity: 2, used: 2, available: 0 });
    fireEvent.click(screen.getByRole("button", { name: "Add RouterBOARD" }));
    await screen.findByText("Gateway B");
    expect(posts).toEqual([{ url: "/routers", body: expect.objectContaining({ deployment_layer: "MAIN_GATEWAY", hardware_name: "Gateway B", site_id: SITE.site_id }) }]);
    expect(posts.some((p) => p.url.endsWith("/site"))).toBe(false);
    expect(screen.getByText("Gateway A")).toBeTruthy();
    await screen.findByText("2 of 2 in use");
  });

  it("explains a capacity refusal and refreshes the slots", async () => {
    cap = capacity({ purchased: 1, capacity: 2, available: 1 });
    routerAnswer = { status: 409, body: { code: "gateway_capacity_reached" } };
    openHub();
    fireEvent.change(await screen.findByLabelText(/name/i, { selector: "#gw-name" }), { target: { value: "Gateway B" } });
    fireEvent.change(screen.getByLabelText("Site", { selector: "#gw-site" }), { target: { value: SITE.site_id } });
    fireEvent.click(screen.getByRole("button", { name: "Add RouterBOARD" }));
    await screen.findByText(/No free gateway slot/);
  });

  it("locks the other gateway's actions while one script is being made", async () => {
    routers = [{ ...A, management_connected: false }, B];
    let release!: () => void; enrollHeld = new Promise<void>((r) => { release = r; });
    openHub();
    const connects = await screen.findAllByRole("button", { name: "Connect Magic Hub" });
    fireEvent.click(connects[0]);
    expect(await screen.findByText("Gateway: Gateway A")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: /I authorize/ }));
    fireEvent.click(screen.getByRole("button", { name: "Generate one-paste script" }));
    await waitFor(() => expect(screen.getAllByRole("button", { name: "Connect Magic Hub" })[1].hasAttribute("disabled")).toBe(true));
    expect(screen.getByText(/Another gateway is being set up/)).toBeTruthy();
    release();
    await screen.findByRole("textbox", { name: "Connection script" });
    await waitFor(() => expect(screen.getAllByRole("button", { name: "Connect Magic Hub" })[1].hasAttribute("disabled")).toBe(false));
  });
});

describe("one-paste recovery and Safe Mode", () => {
  const open = () => render(<I18nProvider><Enrollment id={B.device_id} name="Gateway B" onVerified={vi.fn()} /></I18nProvider>);
  it("keeps the Safe Mode commit instruction visible and explains a resume", async () => {
    open();
    fireEvent.click(screen.getByRole("checkbox")); fireEvent.click(screen.getByRole("button", { name: "Generate one-paste script" }));
    await screen.findByRole("textbox", { name: "Connection script" });
    expect(screen.getByText(/press Ctrl\+X again after the script finishes/)).toBeTruthy();
    expect(screen.getByText(/continues with the key already on the router/)).toBeTruthy();
    expect(screen.getByText(/at most 30 days/)).toBeTruthy();
  });
  it("starts over only after the owner confirms it", async () => {
    open();
    fireEvent.click(screen.getByRole("checkbox")); fireEvent.click(screen.getByRole("button", { name: "Generate one-paste script" }));
    await screen.findByRole("textbox", { name: "Connection script" });
    const button = screen.getByRole("button", { name: "Create a start-over script" });
    expect(button.hasAttribute("disabled")).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: "Start over for this gateway" }));
    fireEvent.click(button);
    await screen.findByText(/This is a start-over script/);
    expect(posts.map((p) => p.body.start_over)).toEqual([undefined, true]);
  });
});

describe("optional Wi-Fi WAN step", () => {
  it("resumes only its own marked partial work and runs after every Magic Hub check", () => {
    const out = withNewRouterNetwork(SCRIPT, "wifi1", "Shop", "password123");
    const check = out.indexOf(':put "check"'), firstNetworkWrite = out.indexOf("/interface bridge port set"), hubWrite = out.indexOf("# APPLY: tunnel");
    expect(check).toBeGreaterThan(0);
    expect(check).toBeLessThan(firstNetworkWrite);
    expect(firstNetworkWrite).toBeLessThan(hubWrite);
    expect(out).toContain(`:local mmMark "${MARK}"`);
    expect(out).toContain(":if (!$mmPortOurs) do={ /interface bridge port set $mmBridgePort disabled=yes comment=$mmMark }");
    expect(out).toContain(":if ([:len $mmMember] = 0) do={ /interface list member add");
    expect(out).toContain(":if ([:len $mmClient] = 0) do={ /ip dhcp-client add");
    expect(out).toContain("changed outside this setup; nothing changed");
    expect(out).toContain("RouterOS 7.13 or later");
    expect(() => withNewRouterNetwork("{\n:put \"x\"\n}", "ethernet", "", "")).toThrow();
  });
});
