import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";
import { Wizard } from "./Wizard";

// Pages must never hide: with no router, or one that cannot be read, every task and
// form is still shown, locked under a banner, and nothing can reach the API.

const router = { device_id: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", hardware_name: "hAP ax2 lab", deployment_layer: "MAIN_GATEWAY" } as never;
const wrap = (ui: React.ReactNode) => render(<I18nProvider>{ui}</I18nProvider>);
let calls: Array<{ url: string; method: string }>;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  calls = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url: String(url), method: init?.method ?? "GET" });
    return new Response(JSON.stringify({ error: "gateway unreachable: dial 10.77.0.8:22: i/o timeout" }), { status: 502, headers: { "content-type": "application/json" } });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const controls = (root: Element) => [...root.querySelectorAll("input, select, button, textarea")];

describe("network pages without a usable router", () => {
  it("with no router: shows the banner, every task and a locked form, and makes no request", async () => {
    const { container } = wrap(<NetworkEditor workspace="ip" canEdit />);
    expect(await screen.findByText(/No router is connected to this account/)).toBeTruthy();
    const tasks = [...container.querySelectorAll(".task")];
    expect(tasks.length).toBeGreaterThanOrEqual(5);
    fireEvent.click(tasks.find((b) => /Static DHCP/.test(b.textContent ?? ""))!);
    const locked = container.querySelector("fieldset.nolock") as HTMLFieldSetElement;
    expect(locked.disabled).toBe(true);
    const inside = controls(locked);
    expect(inside.length).toBeGreaterThan(3);
    expect(inside.every((c) => c.matches(":disabled"))).toBe(true);
    expect(container.textContent).toContain("Reserved client address");
    expect(calls).toEqual([]);
  });

  it("with a router that cannot be read: shows the real reason, a retry button and a locked form", async () => {
    const { container } = wrap(<NetworkEditor workspace="pppoe" canEdit router={router} />);
    expect(await screen.findByText(/dial 10\.77\.0\.8:22: i\/o timeout/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Refresh status/ })).toBeTruthy();
    expect(container.querySelectorAll(".task").length).toBeGreaterThanOrEqual(3);
    const locked = container.querySelector("fieldset.nolock") as HTMLFieldSetElement;
    expect(locked.disabled).toBe(true);
    // Only reads were attempted; no preview/apply request can be made from a locked form.
    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    expect(calls.every((c) => c.method === "GET")).toBe(true);
  });

  it("the new-network wizard stays visible with its address and VLAN fields, locked, when there is no router", async () => {
    const { container } = wrap(<Wizard canEdit />);
    expect(await screen.findByText(/No router is connected to this account/)).toBeTruthy();
    const form = container.querySelector("form.nwform") as HTMLFormElement;
    const names = controls(form).map((c) => (c as HTMLInputElement).name).filter(Boolean);
    for (const field of ["name", "lan_interface", "vlan_id", "gateway_cidr", "pool_start", "pool_end", "dns_servers"]) expect(names).toContain(field);
    expect(controls(form).every((c) => c.matches(":disabled"))).toBe(true);
    fireEvent.submit(form);
    expect(calls).toEqual([]);
  });
});
