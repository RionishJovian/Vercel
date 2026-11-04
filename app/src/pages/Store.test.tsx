import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { Store } from "./Store";
import type { Me } from "../lib/types";

const ADDON = { id: "a1", kind: "feature", period: "monthly", price_mmk: 25000, name_en: "Extra router" };
let addons: { status: number; body: unknown }, post: () => Response, subscription: { status: number; body: unknown }, posted: Array<{ url: string; body: Record<string, unknown> }>;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  addons = { status: 200, body: { addons: [ADDON], requests: [] } }; subscription = { status: 200, body: { mode: "legacy" } }; posted = [];
  post = () => json(200, {});
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", "");
    if (init?.method === "POST") { posted.push({ url: path, body: JSON.parse(String(init.body)) }); return post(); }
    if (path === "/subscription") return json(subscription.status, subscription.body);
    if (path === "/store/addons") return json(addons.status, addons.body);
    if (path === "/me/app-pass/checkout-options") return json(200, { bank_accounts: [{ bank_name: "KBZ", holder_name: "Mega", account_number: "0123456" }] });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (role = "OWNER") => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider><Store me={{ user: { user_role: role } } as unknown as Me} /></I18nProvider></SWRConfig>);
const request = async (ref: string) => {
  const { container } = { container: document.body };
  fireEvent.click(await screen.findByRole("button", { name: "Request" }));
  const input = container.querySelector('input[name="ref"]') as HTMLInputElement;
  fireEvent.change(input, { target: { value: ref } });
  fireEvent.submit(input.form!);
};

describe("Store add-ons", () => {
  it("sends the trimmed bank reference with a retry identity", async () => {
    open();
    await request("  TX-1234 ");
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0].url).toBe("/store/addons/a1/requests");
    expect(posted[0].body.bank_reference).toBe("TX-1234");
    expect(typeof posted[0].body.idempotency_key).toBe("string");
  });
  it("shows the bank details to pay", async () => {
    open();
    expect(await screen.findByText("0123456")).toBeTruthy();
  });
  it("offers no request while one is pending, or to a viewer", async () => {
    addons = { status: 200, body: { addons: [ADDON], requests: [{ id: "r1", addon_id: "a1", status: "pending", bank_reference: "TX-1", price_mmk: 25000, name_en: "Extra router" }] } };
    open();
    await screen.findByText("TX-1");
    expect(screen.queryByRole("button", { name: "Request" })).toBeNull();
    cleanup();
    addons = { status: 200, body: { addons: [ADDON], requests: [] } };
    open("VIEWER");
    expect(((await screen.findByRole("button", { name: "Request" })) as HTMLButtonElement).disabled).toBe(true);
  });
  it("offers no checkout when the Store cannot be read", async () => {
    subscription = { status: 500, body: {} };
    open();
    expect(await screen.findByText("Store status is unavailable, so no checkout is offered.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Request" })).toBeNull();
  });
});
