import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";

const router = { device_id: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", hardware_name: "hAP ax2 lab", deployment_layer: "MAIN_GATEWAY" } as never;
const row = (id: string, values: Record<string, string>) => ({ id, values: { disabled: "no", dynamic: "no", ...values } });
const inventory = {
  interfaces: [{ name: "ether3", type: "ether" }, { name: "ether4", type: "ether" }],
  available: { addresses: true, vlans: true, dhcp: true },
  objects: {
    addresses: [row("*1", { address: "192.168.60.1/24", interface: "ether4", comment: "mega-magic" }), row("*2", { address: "10.9.0.1/24", interface: "ether3", comment: "" }), row("*3", { address: "192.168.70.5/24", interface: "ether3", comment: "mega-magic", dynamic: "yes" })],
    dhcp: [row("*20", { name: "mm-shop", interface: "mm-shop-vlan", "address-pool": "mm-shop", "lease-time": "1h" }), row("*21", { name: "dhcp1", interface: "bridge", "address-pool": "customers", "lease-time": "10m" })],
    vlans: [row("*7", { name: "guests100", "vlan-id": "100", interface: "ether3", comment: "mega-magic" })],
  },
  write_enabled: true, address_write_enabled: true,
};
let posts: Array<{ url: string; body: { kind: string; id: string; values: Record<string, string>; confirmation?: string } }>;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  posts = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      posts.push({ url: String(url), body: JSON.parse(String(init.body)) });
      if (String(url).endsWith("/apply")) return json({ result: { verified: true, backup: "bk" } });
      return json({ review: { steps: ["Delete IP address 192.168.60.1/24 on ether4 — will be deleted"], warnings: [], confirmation: "tok" } });
    }
    return json({ inventory });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = async (workspace: "ip" | "vlan", label: RegExp, record: string) => {
  const view = render(<I18nProvider><NetworkEditor workspace={workspace} canEdit router={router} /></I18nProvider>);
  const task = await waitFor(() => { const t = [...view.container.querySelectorAll(".task")].find((b) => label.test(b.textContent ?? "")); if (!t) throw new Error("task not shown"); return t; });
  fireEvent.click(task);
  const select = await waitFor(() => { const s = view.container.querySelector("select.input") as HTMLSelectElement | null; if (!s) throw new Error("no record list"); return s; });
  fireEvent.change(select, { target: { value: record } });
  return view;
};

describe("deleting what the app created", () => {
  it("a marked address offers Delete, reviews it as a removal, then applies exactly that", async () => {
    await open("ip", /IP addresses/, "*1");
    fireEvent.click(await screen.findByRole("button", { name: "Delete this record" }));
    await waitFor(() => expect(posts.length).toBe(1));
    expect(posts[0].body).toEqual({ kind: "remove_ip_address", id: "*1", values: {} });
    expect(await screen.findByText(/will be deleted/)).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Back up and apply to router" }));
    await waitFor(() => expect(posts.length).toBe(2));
    expect(posts[1].url).toMatch(/network\/apply$/);
    expect(posts[1].body).toEqual({ kind: "remove_ip_address", id: "*1", values: {}, confirmation: "tok" });
  });

  it("an address someone else created, or a dynamic one, has no Delete button", async () => {
    await open("ip", /IP addresses/, "*2");
    expect(screen.queryByRole("button", { name: "Delete this record" })).toBeNull();
    cleanup();
    await open("ip", /IP addresses/, "*3");
    expect(screen.queryByRole("button", { name: "Delete this record" })).toBeNull();
  });

  it("a marked VLAN offers Delete and sends remove_vlan", async () => {
    await open("vlan", /VLANs/, "*7");
    fireEvent.click(await screen.findByRole("button", { name: "Delete this record" }));
    await waitFor(() => expect(posts[0]?.body).toEqual({ kind: "remove_vlan", id: "*7", values: {} }));
  });

  it("editing a field after asking to delete cancels the removal review", async () => {
    const { container } = await open("ip", /IP addresses/, "*1");
    fireEvent.click(await screen.findByRole("button", { name: "Delete this record" }));
    await screen.findByText(/will be deleted/);
    fireEvent.change(container.querySelector('input[name="address"]')!, { target: { value: "192.168.61.1/24" } });
    expect(screen.queryByText(/will be deleted/)).toBeNull();
  });

  it("Remove customer network lists only networks the wizard made and sends remove_network", async () => {
    const view = render(<I18nProvider><NetworkEditor workspace="existing" canEdit router={router} /></I18nProvider>);
    const task = await waitFor(() => { const t = [...view.container.querySelectorAll(".task")].find((b) => /Remove customer network/.test(b.textContent ?? "")); if (!t) throw new Error("task not shown"); return t; });
    fireEvent.click(task);
    const select = await waitFor(() => { const s = view.container.querySelector("select.input") as HTMLSelectElement | null; if (!s) throw new Error("no record list"); return s; });
    expect([...select.options].map((o) => o.textContent)).toEqual(["mm-shop"]);
    const submit = screen.getByRole("button", { name: "Review changes" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.change(select, { target: { value: "*20" } });
    await waitFor(() => expect(submit.disabled).toBe(false));
    fireEvent.submit(view.container.querySelector("form")!);
    await waitFor(() => expect(posts[0]?.body).toEqual({ kind: "remove_network", id: "*20", values: {} }));
  });
});
