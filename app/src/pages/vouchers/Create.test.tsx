import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Create } from "./Create";

const SAVED = { device_id: "gw1", plan_label: "1 day", duration_minutes: 1440, data_mb: 1024, price_mmk: 0, count: 5, batch_id: "33333333-3333-4333-8333-333333333333" };
let store: Record<string, string>;
let calls: Array<{ method: string; url: string }>;
let batchReply: () => Response;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  store = { mm2_lang: "en", mm2_gv_pending: JSON.stringify(SAVED) };
  calls = [];
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path });
    if (path === "/routers") return json(200, { routers: [{ device_id: "gw1", name: "Shop", deployment_layer: "MAIN_GATEWAY" }] });
    if (path === "/guest-vouchers/config") return json(200, { max_batch_size: 200 });
    if (path === "/voucher-resellers") return json(200, { resellers: [] });
    if (path.endsWith("/portal/profiles")) return json(200, { profiles: ["hotspot1"] });
    if (path.startsWith("/guest-vouchers/batches/" + SAVED.batch_id)) return batchReply();
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

const mount = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Create /></I18nProvider></SWRConfig>);

describe("Create with a batch saved in this browser", () => {
  it("shows the codes and clears the record when the server says ready", async () => {
    batchReply = () => json(200, { state: "ready", vouchers: [{ code: "ABCD2345" }] });
    mount();
    expect(await screen.findByText("ABCD2345")).toBeTruthy();
    expect(store.mm2_gv_pending).toBeUndefined();
  });

  it("clears a stale record silently when the server never saw the batch", async () => {
    batchReply = () => json(404, {});
    mount();
    await waitFor(() => expect(store.mm2_gv_pending).toBeUndefined());
    expect(screen.queryByText("Recover saved batch")).toBeNull();
  });

  it("offers Discard while the batch is unfinished; confirming clears only the browser record", async () => {
    batchReply = () => json(200, { state: "pending", vouchers: [] });
    const confirm = vi.fn((_: string) => true);
    vi.stubGlobal("confirm", confirm);
    mount();
    const discard = await screen.findByText("Discard this browser's record");
    expect(screen.getByLabelText<HTMLSelectElement>("Router").disabled).toBe(true);
    fireEvent.click(discard);
    expect(confirm.mock.calls[0]?.[0]).toContain("stay in Stock");
    await waitFor(() => expect(store.mm2_gv_pending).toBeUndefined());
    expect(screen.getByLabelText<HTMLSelectElement>("Router").disabled).toBe(false);
    // Nothing was deleted or recreated on the server.
    expect(calls.filter((c) => c.method !== "GET")).toEqual([]);
  });

  it("keeps the record when Discard is not confirmed and never creates a new batch", async () => {
    batchReply = () => json(200, { state: "unknown", vouchers: [] });
    vi.stubGlobal("confirm", () => false);
    mount();
    fireEvent.click(await screen.findByText("Discard this browser's record"));
    expect(JSON.parse(store.mm2_gv_pending).batch_id).toBe(SAVED.batch_id);
    expect(calls.some((c) => c.method === "POST" && c.url === "/guest-vouchers")).toBe(false);
  });
});
