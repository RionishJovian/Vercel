import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";

const router = { device_id: "gw", hardware_name: "Shop", deployment_layer: "MAIN_GATEWAY" } as never;
const row = (id: string, values: Record<string, string>) => ({ id, values });
const base = {
  interfaces: [{ name: "ether3", type: "ether" }],
  available: { subscribers: true, ppp_profiles: true, pppoe_servers: true, ppp_active: true },
  write_enabled: true,
};
let subscribers: ReturnType<typeof row>[], active: ReturnType<typeof row>[];

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: () => undefined });
  subscribers = []; active = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (String(url).endsWith("/preview")) return json({ review: { steps: ["Create subscriber"], warnings: null, confirmation: "tok" } });
    if (String(url).endsWith("/apply")) { subscribers.push(row("*1", { name: "trial01", profile: "plan", service: "pppoe" })); return json({ result: { verified: true, backup: "bk" } }); }
    if (String(url).endsWith("/gateway/defaults")) return json({ defaults: {} });
    void init;
    return json({ inventory: { ...base, objects: { subscribers, ppp_active: active, ppp_profiles: [row("*2", { name: "plan" })], pppoe_servers: [] } } });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<I18nProvider><NetworkEditor workspace="pppoe" router={router} canEdit /></I18nProvider>);
const choose = async () => fireEvent.click((await screen.findByText("PPPoE subscribers")).closest("button")!);

describe("PPPoE page", () => {
  it("makes the owner type a login instead of offering one", async () => {
    open(); await choose();
    expect((await screen.findByLabelText("Subscriber login") as HTMLInputElement).value).toBe("");
  });
  it("updates the record count after a change, keeping the result on screen", async () => {
    open(); await choose();
    expect(screen.getByText(/0 records/, { selector: ".task[aria-pressed=true] *" })).toBeTruthy();
    fireEvent.change(await screen.findByLabelText("Subscriber login"), { target: { value: "trial01" } });
    fireEvent.change(screen.getByLabelText("Subscriber password"), { target: { value: "test-pass" } });
    fireEvent.change(screen.getByLabelText("Subscriber plan"), { target: { value: "plan" } });
    fireEvent.submit(screen.getByLabelText("Subscriber login").closest("form")!);
    await screen.findByText("Create subscriber");
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    await waitFor(() => expect(screen.getByText(/1 records/, { selector: ".task[aria-pressed=true] *" })).toBeTruthy());
    expect(screen.getByRole("status").textContent).toMatch(/bk/);
  });
  it("lists who is connected, including logins that aren't paid accounts", async () => {
    active = [row("*a", { name: "trial01", service: "pppoe", address: "192.168.51.250", "caller-id": "AA:BB:CC:DD:EE:FF", uptime: "5m" }), row("*b", { name: "vpn-user", service: "l2tp", address: "10.0.0.2" })];
    open();
    const list = await screen.findByLabelText("Connected now");
    expect(list.textContent).toContain("trial01");
    expect(list.textContent).toContain("192.168.51.250");
    expect(list.textContent).not.toContain("vpn-user");
  });
  it("says so when nobody is connected", async () => {
    open();
    expect(await screen.findByText("No PPPoE clients are connected right now.")).toBeTruthy();
  });
});
