import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Create } from "./Create";

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
let store: Record<string, string>;

beforeEach(() => {
  store = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/routers") return json({ routers: [{ device_id: "mine", hardware_name: "Shop", deployment_layer: "MAIN_GATEWAY" }] });
    if (path.endsWith("/portal/profiles")) return json({ profiles: ["guests"] });
    if (path === "/guest-vouchers/config") return json({ max_batch_size: 200 });
    return json({});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Create /></I18nProvider></SWRConfig>);
const button = async () => (await screen.findByRole("button", { name: "Create batch" })) as HTMLButtonElement;

describe("a saved voucher batch", () => {
  it("is dropped when it belongs to a router this account does not have", async () => {
    store.mm2_gv_pending = JSON.stringify({ batch_id: "old", device_id: "someone-elses", count: 2 });
    open();
    await waitFor(async () => expect((await button()).disabled).toBe(false));
    expect(store.mm2_gv_pending).toBeUndefined();
    expect(screen.queryByRole("button", { name: "Recover saved batch" })).toBeNull();
  });
  it("is kept for a router this account has, so it can't make new codes", async () => {
    store.mm2_gv_pending = JSON.stringify({ batch_id: "mine-1", device_id: "mine", count: 2 });
    open();
    expect(await screen.findByRole("button", { name: "Recover saved batch" })).toBeTruthy();
    expect((await button()).disabled).toBe(true);
    expect(store.mm2_gv_pending).toBeTruthy();
  });
});
