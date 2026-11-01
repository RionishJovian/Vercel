import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";

const router = { device_id: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", hardware_name: "hAP ax2 lab", deployment_layer: "MAIN_GATEWAY" } as never;
const inventory = (flags: Record<string, boolean>) => ({
  interfaces: [{ name: "ether1", type: "ether", running: true }, { name: "ether3", type: "ether" }, { name: "bridge", type: "bridge", running: true }, { name: "vlan10", type: "vlan" }, { name: "pppoe-out1", type: "pppoe-out" }, { name: "mmh-abc", type: "wireguard" }],
  available: { addresses: true, vlans: true, routes: true, dhcp: true, pools: true, dhcp_networks: true, leases: true },
  objects: { addresses: [{ id: "*1", values: { address: "192.168.88.1/24", interface: "bridge", dynamic: "no", disabled: "no", comment: "" } }, { id: "*2", values: { address: "203.0.113.2/30", interface: "ether1", dynamic: "yes", disabled: "no", comment: "" } }], vlans: [{ id: "*7", values: { name: "guests100", "vlan-id": "100", interface: "ether3", disabled: "no" } }] },
  write_enabled: true, ...flags,
});
let posts: Array<{ url: string; body: { kind: string; id: string; values: Record<string, string> } }>;
let flags: Record<string, boolean>;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  posts = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") { posts.push({ url: String(url), body: JSON.parse(String(init.body)) }); return json({ review: { steps: ["Add IP address"], warnings: [], confirmation: "tok" } }); }
    return json({ inventory: inventory(flags) });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = async (workspace: "ip" | "vlan", label: RegExp) => {
  const view = render(<I18nProvider><NetworkEditor workspace={workspace} canEdit router={router} /></I18nProvider>);
  const task = await waitFor(() => { const t = [...view.container.querySelectorAll(".task")].find((b) => label.test(b.textContent ?? "")); if (!t) throw new Error("task not shown"); return t; });
  fireEvent.click(task);
  return view;
};

describe("IP address and VLAN tasks", () => {
  it("with the server switch off: shows the form locked with the reason, never hidden", async () => {
    flags = { address_write_enabled: false };
    const { container } = await open("ip", /IP addresses/);
    expect(await screen.findByText(/can.t be applied yet: they are waiting for a test on a real router/)).toBeTruthy();
    const fieldset = container.querySelector("fieldset.nolock") as HTMLFieldSetElement;
    expect(fieldset.disabled).toBe(true);
    expect(container.textContent).toContain("IP address with prefix");
    expect([...fieldset.querySelectorAll("input, select, button")].every((c) => c.matches(":disabled"))).toBe(true);
  });

  it("in preview-only mode: the form is usable and a banner says nothing will be applied", async () => {
    flags = { address_write_enabled: true, address_preview_only: true };
    const { container } = await open("ip", /IP addresses/);
    expect(await screen.findByText(/Preview only: the app reads your router/)).toBeTruthy();
    await waitFor(() => expect((container.querySelector("fieldset.nolock") as HTMLFieldSetElement).disabled).toBe(false));
  });

  it("with the switch on: the address form is usable and sends only the three accepted fields", async () => {
    flags = { address_write_enabled: true };
    const { container } = await open("ip", /IP addresses/);
    await waitFor(() => expect((container.querySelector("fieldset.nolock") as HTMLFieldSetElement).disabled).toBe(false));
    const ports = [...container.querySelectorAll('select[name="interface"] option')].map((o) => o.textContent ?? "");
    expect(ports.join(" ")).toMatch(/ether3/); expect(ports.join(" ")).toMatch(/bridge/); expect(ports.join(" ")).toMatch(/vlan10/);
    expect(ports.join(" ")).not.toMatch(/pppoe-out1|mmh-abc/);
    fireEvent.change(container.querySelector('input[name="address"]')!, { target: { value: "192.168.50.1/24" } });
    fireEvent.change(container.querySelector('select[name="interface"]')!, { target: { value: "ether3" } });
    fireEvent.submit(container.querySelector("form")!);
    await waitFor(() => expect(posts.length).toBe(1));
    expect(posts[0].url).toMatch(/network\/preview$/);
    expect(posts[0].body).toEqual({ kind: "ip_address", id: "", values: { address: "192.168.50.1/24", interface: "ether3", disabled: "no" } });
  });

  it("editing an address cannot change its port, and dynamic addresses are not offered", async () => {
    flags = { address_write_enabled: true };
    const { container } = await open("ip", /IP addresses/);
    await waitFor(() => expect(container.querySelector("select.input")).toBeTruthy());
    const choose = container.querySelector("label.field select.input") as HTMLSelectElement;
    expect([...choose.options].map((o) => o.textContent)).toEqual(["Create new", "192.168.88.1/24"]);
    fireEvent.change(choose, { target: { value: "*1" } });
    await waitFor(() => expect((container.querySelector('select[name="interface"]') as HTMLSelectElement).disabled).toBe(true));
    fireEvent.change(container.querySelector('input[name="address"]')!, { target: { value: "192.168.89.1/24" } });
    fireEvent.submit(container.querySelector("form")!);
    await waitFor(() => expect(posts.length).toBe(1));
    expect(posts[0].body.id).toBe("*1");
    expect(Object.keys(posts[0].body.values).sort()).toEqual(["address", "disabled", "interface"]);
  });

  it("a VLAN is created with its ID under the key the server expects", async () => {
    flags = { address_write_enabled: true };
    const { container } = await open("vlan", /VLANs/);
    await waitFor(() => expect((container.querySelector("fieldset.nolock") as HTMLFieldSetElement).disabled).toBe(false));
    const parents = [...container.querySelectorAll('select[name="interface"] option')].map((o) => o.textContent ?? "").join(" ");
    expect(parents).toMatch(/ether3/); expect(parents).not.toMatch(/vlan10|pppoe-out1|mmh-abc/);
    fireEvent.change(container.querySelector('input[name="name"]')!, { target: { value: "guests200" } });
    fireEvent.change(container.querySelector('select[name="interface"]')!, { target: { value: "ether3" } });
    fireEvent.change(container.querySelector('input[name="vlan-id"]')!, { target: { value: "200" } });
    fireEvent.submit(container.querySelector("form")!);
    await waitFor(() => expect(posts.length).toBe(1));
    expect(posts[0].body).toEqual({ kind: "vlan", id: "", values: { name: "guests200", interface: "ether3", "vlan-id": "200", disabled: "no" } });
  });
});
