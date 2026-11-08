import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Topology } from "../Topology";
import { Maintenance } from "../hub/Maintenance";
import { discoveryRows } from "../Devices";
import { project } from "./project";
import type { Me } from "../../lib/types";

const GW = { device_id: "gw", hardware_name: "Shop gateway", deployment_layer: "MAIN_GATEWAY", category: "gateway", site_id: "s1" };
const AP_RECORD = { device_id: "ap", hardware_name: "Lobby AP", deployment_layer: "ACCESS_POINT", category: "ap", site_id: "s1", mac_address: "02:00:00:00:00:20" };
const o = (values: Record<string, string>, id = "*1") => ({ id, values });
const inventory = (over: Record<string, unknown> = {}) => ({
  observed_at: "2026-11-05T10:00:00Z", health: { version: "7.25" }, device_controls_enabled: true,
  available: { interfaces: true, interface_labels: true, neighbors: true, bridge_hosts: true, hotspot_sessions: true, device_blocks: true, discovery_leases: true, poe_ports: true, arp: true, wifi_clients: false, wifi_interfaces: true },
  objects: {
    interfaces: [o({ name: "ether2", type: "ether", "mac-address": "02:00:00:00:00:01", running: "true", disabled: "false" }), o({ name: "bridge1", type: "bridge", "mac-address": "02:00:00:00:00:02", running: "true", disabled: "false" })],
    interface_labels: [o({ name: "ether2", comment: "Floor 2" }), o({ name: "bridge1", comment: "" })],
    neighbors: [o({ "mac-address": "02:00:00:00:00:20", address: "192.168.88.20", identity: "AP", platform: "MikroTik", board: "cAP ax", interface: "ether2,bridge1" })],
    bridge_hosts: [o({ "mac-address": "02:00:00:00:00:40", "on-interface": "ether2", local: "false" }), o({ "mac-address": "02:00:00:00:00:01", "on-interface": "ether2", local: "true" })],
    discovery_leases: [o({ "mac-address": "02:00:00:00:00:40", address: "192.168.88.40", "host-name": "phone", status: "bound", server: "dhcp1" })],
    hotspot_sessions: [o({ user: "ABCDEFGH", address: "192.168.88.40", "mac-address": "02:00:00:00:00:40" }, "*9")],
    device_blocks: [], poe_ports: [o({ name: "ether2", "poe-out": "auto-on" })], arp: [], wifi_interfaces: [],
  }, ...over,
});
const me = (role: string) => ({ user: { user_role: role } }) as unknown as Me;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
let calls: Array<{ method: string; path: string; body?: Record<string, unknown> }>, inv: unknown, respond: (path: string, body?: Record<string, unknown>) => Response | null, guestActions: boolean;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("sessionStorage", { getItem: () => null, setItem: () => undefined, removeItem: () => undefined });
  calls = []; inv = inventory(); respond = () => null; guestActions = true;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", ""), method = init?.method ?? "GET", body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ method, path, body });
    const custom = respond(path, body);
    if (custom) return custom;
    if (path === "/topology/map") return json(200, { roots: [{ device: GW }, { device: AP_RECORD }] });
    if (path === "/sites") return json(200, { sites: [{ site_id: "s1", name: "Main" }] });
    if (path.endsWith("/observations")) return json(200, { inventory: inv, guest_actions_enabled: guestActions });
    if (path.endsWith("/discovery-labels")) return json(200, { labels: method === "PUT" ? [body] : [] });
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (role = "OWNER") => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Topology me={me(role)} /></I18nProvider></SWRConfig>);
const pick = async (name: string) => fireEvent.click((await screen.findByText(name)).closest("button")!);
const writes = () => calls.filter((c) => c.method !== "GET");

describe("topology projection for controls", () => {
  const g = project(GW, inventory() as never, [], [AP_RECORD]);
  it("reads RouterOS 7.25's semicolon as well as a comma between port and bridge", () => {
    const inv = inventory();
    (inv.objects as { neighbors: Array<{ values: Record<string, string> }> }).neighbors[0].values.interface = "ether2;bridge1";
    expect(project(GW, inv as never, [], [AP_RECORD]).devices.find((d) => d.mac === "02:00:00:00:00:20")?.ports).toEqual(["ether2"]);
  });
  it("shows the port, not the bridge, when the router reports both", () => { expect(g.devices.find((d) => d.mac === "02:00:00:00:00:20")?.ports).toEqual(["ether2"]); });
  it("keeps a platform out of the model field", () => { expect(g.devices.find((d) => d.mac === "02:00:00:00:00:20")).toMatchObject({ model: "cAP ax", platform: "MikroTik" }); });
  it("matches saved records to observations and shows unobserved APs without claiming connectivity", () => {
    expect(g.devices.find((d) => d.mac === "02:00:00:00:00:20")).toMatchObject({ kind: "ap", record: { device_id: "ap" } });
    expect(g.devices).toHaveLength(2);
    expect(project(GW, inventory() as never, [], [{ ...AP_RECORD, mac_address: "02:00:00:00:00:77" }]).devices.find((d) => d.record?.device_id === "ap")).toMatchObject({ state: "Not observed", direct: false, ports: [] });
  });
  it("marks port state, label and PoE from the reading", () => { expect(g.ports).toEqual([{ name: "ether2", running: true, disabled: false, label: "Floor 2", poe: true }]); });
  it("still lists ports when their labels could not be read, and does not call that a missing source", () => {
    const i = inventory(); (i.available as Record<string, boolean>).interface_labels = false;
    const g2 = project(GW, i as never);
    expect(g2.ports).toEqual([{ name: "ether2", running: true, disabled: false, label: "", poe: true }]);
    expect(g2.unavailable).not.toContain("interface_labels");
  });
  it("keeps the session id the router needs", () => { expect(g.devices.find((d) => d.mac === "02:00:00:00:00:40")?.session).toMatchObject({ id: "*9", user: "ABCDEFGH" }); });
  it("does not report a router without PoE as a missing source", () => {
    const i = inventory(); (i.available as Record<string, boolean>).poe_ports = false;
    expect(project(GW, i as never).unavailable).not.toContain("poe_ports");
  });
  it("marks a blocked device", () => {
    const i = inventory(); (i.objects as Record<string, unknown>).device_blocks = [o({ "src-mac-address": "02:00:00:00:00:40", comment: "mm-block-020000000040", disabled: "false" })];
    expect(project(GW, i as never).devices.find((d) => d.mac === "02:00:00:00:00:40")?.blocked).toBe(true);
  });
});

describe("controls on the topology page", () => {
  it("keeps the page up when a review has no warnings", async () => {
    respond = (path) => path.endsWith("/network/preview") ? json(200, { review: { steps: ["Remove the block"], warnings: null, confirmation: "tok" } }) : null;
    open(); await pick("AP");
    fireEvent.click(await screen.findByRole("button", { name: "Block internet" }));
    expect(await screen.findByText("Remove the block")).toBeTruthy();
    expect(screen.getByRole("button", { name: /apply/i })).toBeTruthy();
  });
  it("reviews, then applies, a block only after consent", async () => {
    respond = (path, body) => {
      if (path.endsWith("/network/preview")) { expect(body).toEqual({ kind: "device_block", id: "", values: { mac: "02:00:00:00:00:20" } }); return json(200, { review: { steps: ["Block internet"], warnings: ["Same-switch traffic continues."], confirmation: "tok" } }); }
      if (path.endsWith("/network/apply")) { expect(body?.confirmation).toBe("tok"); return json(200, { result: { verified: true } }); }
      return null;
    };
    open(); await pick("AP");
    fireEvent.click(await screen.findByRole("button", { name: "Block internet" }));
    expect(await screen.findByText("Same-switch traffic continues.")).toBeTruthy();
    expect(writes().some((c) => c.path.endsWith("/apply"))).toBe(false);
    const apply = screen.getByRole("button", { name: /apply/i }) as HTMLButtonElement;
    expect(apply.disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(apply);
    expect(await screen.findByText(/Blocked\. The router confirmed/)).toBeTruthy();
    await waitFor(() => expect(calls.filter((c) => c.path.endsWith("/observations")).length).toBeGreaterThan(1));
  });

  it("does not report success when the router did not verify", async () => {
    respond = (path) => path.endsWith("/network/preview") ? json(200, { review: { steps: ["x"], warnings: [], confirmation: "tok" } }) : path.endsWith("/network/apply") ? json(200, { result: { verified: false } }) : null;
    open(); await pick("AP");
    fireEvent.click(await screen.findByRole("button", { name: "Block internet" }));
    fireEvent.click(await screen.findByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    expect(await screen.findByText(/did not confirm/)).toBeTruthy();
    expect(screen.queryByText(/Blocked\. The router confirmed/)).toBeNull();
  });

  it("locks router changes with a reason while the switch is off, and still allows naming", async () => {
    inv = inventory({ device_controls_enabled: false });
    open(); await pick("AP");
    expect((await screen.findAllByText(/stay off until/)).length).toBeGreaterThan(0);
    expect((screen.getByRole("button", { name: "Block internet" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: /^Save name/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("holds a Hotspot guest's actions on the guest switch, not the device switch", async () => {
    inv = inventory({ device_controls_enabled: false });
    const first = open(); await pick("phone");
    expect((await screen.findByRole("button", { name: "Disconnect guest" }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole("button", { name: "Block internet" }) as HTMLButtonElement).disabled).toBe(false);
    first.unmount();
    guestActions = false; inv = inventory();
    open(); await pick("phone");
    expect((await screen.findByRole("button", { name: "Disconnect guest" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Block internet" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("blocks a Hotspot guest through the guest action, and removes it the same way", async () => {
    respond = (path) => path.endsWith("/guests/block") || path.endsWith("/guests/unblock") ? json(200, { verified: true }) : null;
    open(); await pick("phone");
    fireEvent.click(await screen.findByRole("button", { name: "Block internet" }));
    expect(writes()).toHaveLength(0);
    fireEvent.click(screen.getAllByRole("button", { name: "Block internet" }).at(-1)!);
    await waitFor(() => expect(writes()[0]).toMatchObject({ path: "/routers/gw/guests/block", body: { mac: "02:00:00:00:00:40", confirm: true } }));
    expect(writes().some((c) => c.path.endsWith("/network/preview"))).toBe(false);
    cleanup(); calls = [];
    const banned = inventory(); (banned.available as Record<string, boolean>).hotspot_bindings = true;
    (banned.objects as Record<string, unknown>).hotspot_bindings = [o({ "mac-address": "02:00:00:00:00:40", type: "blocked", comment: "mega-magic:ban", disabled: "false" })];
    (banned.objects as Record<string, unknown>).hotspot_sessions = [];
    inv = banned;
    open(); await pick("phone");
    fireEvent.click(await screen.findByRole("button", { name: "Remove block" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Remove block" }).at(-1)!);
    await waitFor(() => expect(writes()[0]).toMatchObject({ path: "/routers/gw/guests/unblock", body: { mac: "02:00:00:00:00:40" } }));
  });

  it("does not treat another owner's IP binding as a ban", () => {
    const i = inventory(); (i.available as Record<string, boolean>).hotspot_bindings = true;
    (i.objects as Record<string, unknown>).hotspot_bindings = [o({ "mac-address": "02:00:00:00:00:40", type: "blocked", comment: "owner rule", disabled: "false" })];
    expect(project(GW, i as never).devices.find((d) => d.mac === "02:00:00:00:00:40")?.guestBanned).toBeUndefined();
  });

  it("shows a viewer the facts and no controls", async () => {
    open("VIEWER"); await pick("phone");
    await screen.findByText("192.168.88.40", { selector: "dd" });
    expect(screen.queryByRole("button", { name: "Block internet" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Save name/ })).toBeNull();
  });

  it("sends the exact session when disconnecting a guest", async () => {
    respond = (path) => path.endsWith("/guests/disconnect") ? json(200, { verified: true }) : null;
    open(); await pick("phone");
    fireEvent.click(await screen.findByRole("button", { name: "Disconnect guest" }));
    expect(writes()).toHaveLength(0);
    fireEvent.click(screen.getAllByRole("button", { name: "Disconnect guest" }).at(-1)!);
    expect(await screen.findByText(/Session ended/)).toBeTruthy();
    expect(writes()[0].body).toEqual({ id: "*9", mac: "02:00:00:00:00:40", address: "192.168.88.40", user: "ABCDEFGH", confirm: true });
  });

  it("links to the reservation and speed forms with this device filled in", async () => {
    open(); await pick("phone");
    const reserve = await screen.findByRole("link", { name: "Reserve this IP" }), limit = screen.getByRole("link", { name: "Limit speed" });
    const params = new URLSearchParams(reserve.getAttribute("href")!.split("?")[1]);
    expect(Object.fromEntries(params)).toEqual({ task: "lease", "mac-address": "02:00:00:00:00:40", address: "192.168.88.40", server: "dhcp1" });
    expect(limit.getAttribute("href")).toContain("target=192.168.88.40%2F32");
  });

  it("names a device from the map", async () => {
    open(); await pick("phone");
    const input = await screen.findByLabelText(/name/i, { selector: "input[name=name]" });
    fireEvent.change(input, { target: { value: "Cashier phone" } });
    fireEvent.click(screen.getByRole("button", { name: /^Save name/ }));
    await waitFor(() => expect(writes()[0]).toMatchObject({ method: "PUT", body: { mac: "02:00:00:00:00:40", name: "Cashier phone" } }));
  });

  it("shows the keep-or-revert panel when a port change waits on the owner", async () => {
    inv = inventory({ pending_confirm: { revert_at: new Date(Date.now() + 240000).toISOString() } });
    open();
    expect(await screen.findByRole("timer")).toBeTruthy();
  });

  it("re-reads the router on Refresh", async () => {
    open();
    const refresh = await screen.findByRole("button", { name: "Refresh" });
    await waitFor(() => expect((refresh as HTMLButtonElement).disabled).toBe(false));
    const before = calls.filter((c) => c.path.endsWith("/observations")).length;
    fireEvent.click(refresh);
    await waitFor(() => expect(calls.filter((c) => c.path.endsWith("/observations")).length).toBe(before + 1));
  });
});

describe("reboot result", () => {
  const openMaintenance = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Maintenance id="gw" name="Shop" canEdit /></I18nProvider></SWRConfig>);
  async function ask() {
    fireEvent.click(screen.getByRole("button", { name: "Reboot router" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Reboot router" }).at(-1)!);
  }
  it("says it is waiting, then shows the recorded outcome", async () => {
    let outcome = false;
    respond = (path) => {
      if (path === "/topology/gw/reboot") return json(202, { status: "queued" });
      if (path === "/devices/gw/changes") return json(200, { changes: outcome ? [{ action: "router.reboot.denied", timestamp: new Date().toISOString() }] : [] });
      return null;
    };
    openMaintenance(); await ask();
    expect(await screen.findByText(/Waiting for approval/)).toBeTruthy();
    outcome = true;
    expect(await screen.findByText(/Reboot refused/, undefined, { timeout: 7000 })).toBeTruthy();
    expect(screen.queryByText(/Waiting for approval/)).toBeNull();
  }, 10000);
  it("ignores an older reboot's result", async () => {
    respond = (path) => {
      if (path === "/topology/gw/reboot") return json(202, {});
      if (path === "/devices/gw/changes") return json(200, { changes: [{ action: "router.reboot.applied", timestamp: "2026-01-01T00:00:00Z" }] });
      return null;
    };
    openMaintenance(); await ask();
    expect(await screen.findByText(/Waiting for approval/)).toBeTruthy();
    expect(screen.queryByText(/router came back/)).toBeNull();
  });
  it("shows the server's refusal instead of claiming it was queued", async () => {
    respond = (path) => path === "/topology/gw/reboot" ? json(503, { error: "Reboot from Mega Magic is not available on this server; no router was changed" }) : null;
    openMaintenance(); await ask();
    expect(await screen.findByText(/not available on this server/)).toBeTruthy();
    expect(screen.queryByText(/queued/)).toBeNull();
  });
});

describe("devices list", () => {
  it("includes devices seen only on a bridge port or Wi-Fi, without the router's own ports", () => {
    const rows = discoveryRows({ available: { bridge_hosts: true, wifi_clients: true, interfaces: true }, objects: {
      interfaces: [{ values: { "mac-address": "02:00:00:00:00:01" } }],
      bridge_hosts: [{ values: { "mac-address": "02:00:00:00:00:40", "on-interface": "ether2", local: "false" } }, { values: { "mac-address": "02:00:00:00:00:01", local: "true" } }, { values: { "mac-address": "01:80:C2:00:00:00", local: "false" } }, { values: { "mac-address": "02:00:00:00:00:01", local: "false" } }],
      wifi_clients: [{ values: { "mac-address": "02:00:00:00:00:50", interface: "wifi1" } }],
    } });
    expect(rows.map((r) => r.mac).sort()).toEqual(["02:00:00:00:00:40", "02:00:00:00:00:50"]);
  });
});
