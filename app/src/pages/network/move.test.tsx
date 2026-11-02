import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";

const router = { device_id: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", hardware_name: "hAP ax2 lab", deployment_layer: "MAIN_GATEWAY" } as never;
const row = (id: string, values: Record<string, string>) => ({ id, values: { disabled: "no", dynamic: "no", ...values } });
const inventory = {
  interfaces: [{ name: "ether4", type: "ether" }],
  available: { dhcp: true, pools: true, addresses: true, dhcp_networks: true },
  objects: {
    dhcp: [row("*20", { name: "mm-shop", interface: "mm-shop-vlan", "address-pool": "mm-shop", "lease-time": "1h" }), row("*21", { name: "dhcp1", interface: "bridge", "address-pool": "customers" })],
    pools: [row("*30", { name: "mm-shop", ranges: "192.168.99.10-192.168.99.200" })],
    addresses: [row("*71", { address: "192.168.99.1/24", interface: "mm-shop-vlan", comment: "mm-shop" }), row("*72", { address: "192.168.88.1/24", interface: "bridge", comment: "" })],
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
      if (String(url).endsWith("/apply")) return json({ result: { verified: true, backup: "bk", pending_confirm: true, revert_at: new Date(Date.now() + 290_000).toISOString() } });
      return json({ review: { steps: ["Move gateway address of mm-shop — address: 192.168.99.1/24 → 10.20.0.1/24"], warnings: ["The change reverts by itself in 5 minutes unless you press Keep."], confirmation: "tok" } });
    }
    return json({ inventory });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const openMove = async () => {
  const view = render(<I18nProvider><NetworkEditor workspace="existing" canEdit router={router} /></I18nProvider>);
  const task = await waitFor(() => { const t = [...view.container.querySelectorAll(".task")].find((b) => /Move customer network/.test(b.textContent ?? "")); if (!t) throw new Error("task not shown"); return t; });
  fireEvent.click(task);
  const select = await waitFor(() => { const s = view.container.querySelector("select.input") as HTMLSelectElement | null; if (!s) throw new Error("no record list"); return s; });
  return { view, select };
};

describe("Move customer network", () => {
  it("lists only wizard-made networks and starts from the current gateway and range", async () => {
    const { view, select } = await openMove();
    expect([...select.options].map((o) => o.textContent)).toEqual(["mm-shop"]);
    expect((screen.getByRole("button", { name: "Review changes" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(select, { target: { value: "*20" } });
    await waitFor(() => expect((view.container.querySelector('input[name="gateway_cidr"]') as HTMLInputElement).value).toBe("192.168.99.1/24"));
    expect((view.container.querySelector('input[name="pool_start"]') as HTMLInputElement).value).toBe("192.168.99.10");
    expect((view.container.querySelector('input[name="pool_end"]') as HTMLInputElement).value).toBe("192.168.99.200");
  });

  it("sends exactly the three fields, and after applying waits for Keep or Revert instead of finishing", async () => {
    const { view, select } = await openMove();
    fireEvent.change(select, { target: { value: "*20" } });
    await waitFor(() => expect((view.container.querySelector('input[name="gateway_cidr"]') as HTMLInputElement).value).toBe("192.168.99.1/24"));
    fireEvent.change(view.container.querySelector('input[name="gateway_cidr"]')!, { target: { value: "10.20.0.1/24" } });
    fireEvent.change(view.container.querySelector('input[name="pool_start"]')!, { target: { value: "10.20.0.10" } });
    fireEvent.change(view.container.querySelector('input[name="pool_end"]')!, { target: { value: "10.20.0.200" } });
    fireEvent.submit(view.container.querySelector("form")!);
    await waitFor(() => expect(posts[0]?.body).toEqual({ kind: "move_network", id: "*20", values: { gateway_cidr: "10.20.0.1/24", pool_start: "10.20.0.10", pool_end: "10.20.0.200" } }));
    expect((await screen.findAllByText(/reverts by itself in 5 minutes/)).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Back up and apply to router" }));
    expect(await screen.findByText("Waiting for you to keep this change")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Keep these changes" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Revert now" })).toBeTruthy();
    expect((view.container.querySelector("fieldset.nolock") as HTMLFieldSetElement).disabled).toBe(true);
  });
});
