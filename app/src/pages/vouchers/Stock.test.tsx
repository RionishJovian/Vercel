import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Stock } from "./Stock";
import * as printing from "./printCodes";

const BATCH = "11111111-1111-4111-8111-111111111111", OPEN = "22222222-2222-4222-8222-222222222222";
const row = (i: number, extra: Record<string, unknown> = {}) => ({ id: `v${i}`, code: `CODE${i}`, plan_label: "1 hour", status: "unused", price_mmk: 1500, duration_minutes: 60, issue_batch_id: BATCH, install_state: "verified", ...extra });
let pages: Record<string, unknown>;
let calls: Array<{ method: string; url: string }>;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("confirm", () => true);
  calls = [];
  pages = {};
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path });
    if (path === "/guest-vouchers/batches") return json(200, { batches: [{ batch_id: OPEN, verified_count: 1, total_count: 2 }], stock_batches: [{ batch_id: BATCH, device_id: "d1", plan_label: "1 hour", total_count: 505, installed_count: 505, activated_count: 3, shop_name: "Main shop" }] });
    if (path === "/voucher-print-layout") return json(200, { layout: { business_name: "Cafe", paper_width_mm: 80 } });
    if (path.startsWith("/guest-vouchers/batches/" + BATCH)) return json(200, { state: "ready", vouchers: [row(1)] });
    if (path.startsWith("/guest-vouchers/batches/" + OPEN)) return json(200, { state: "unknown", vouchers: [] });
    if (method === "DELETE") return json(200, { status: "deleted" });
    if (path.startsWith("/vouchers?")) {
      const page = Number(new URLSearchParams(path.split("?")[1]).get("page"));
      return json(200, pages[page] ?? pages[1] ?? { vouchers: [], total: 0, status_counts: {} });
    }
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><Stock /></I18nProvider></SWRConfig>);
const rowOf = (code: string) => screen.getByText(code).closest("tr") as HTMLElement;
const btn = (root: HTMLElement, name: string) => within(root).getByRole("button", { name }) as HTMLButtonElement;

describe("Vouchers table", () => {
  it("asks the server for one page and shows the range of the total", async () => {
    pages = { 1: { vouchers: [row(1)], total: 505, status_counts: { unused: 505 } }, 3: { vouchers: [row(201), row(202)], total: 505, status_counts: { unused: 505 } } };
    open();
    await screen.findByText("CODE1");
    expect(calls.some((c) => c.url.includes("/vouchers?page=1&per_page=100"))).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Page 3" }));
    await screen.findByText("CODE201");
    expect(screen.getByText("Page 3 · 201–202 of 505")).toBeTruthy();
  });

  it("keeps a selection across pages and prints it", async () => {
    pages = { 1: { vouchers: [row(1)], total: 150, status_counts: { unused: 150 } }, 2: { vouchers: [row(101)], total: 150, status_counts: { unused: 150 } } };
    const print = vi.spyOn(printing, "printCodes").mockImplementation(() => undefined);
    open();
    await screen.findByText("CODE1");
    fireEvent.click(screen.getByRole("checkbox", { name: "Select CODE1" }));
    fireEvent.click(screen.getByRole("button", { name: "Page 2" }));
    await screen.findByText("CODE101");
    fireEvent.click(screen.getByRole("checkbox", { name: "Select CODE101" }));
    expect(screen.getByText("2 selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Slips (2)" }));
    expect(print.mock.calls[0][0].map((v) => v.code)).toEqual(["CODE1", "CODE101"]);
  });

  it("does not offer a slip for a code the router has not verified, and hides that code", async () => {
    pages = { 1: { vouchers: [row(1, { install_state: "unknown" }), row(2, { issue_batch_id: OPEN }), row(3, { status: "used" })], total: 3, status_counts: { unused: 2, used: 1 } } };
    open();
    await screen.findByText("CODE2");
    expect(screen.queryByText("CODE1")).toBeNull();
    expect(screen.getByText("Not on router")).toBeTruthy();
    expect(btn(rowOf("CODE2"), "Slip").disabled).toBe(true); // verified, but its batch is unfinished
    expect(btn(rowOf("CODE3"), "Slip").disabled).toBe(true); // already used
  });

  it("never offers Delete for a code on the router, and Hand out stays unavailable", async () => {
    pages = { 1: { vouchers: [row(1), row(2, { issue_batch_id: undefined, install_state: undefined }), row(3, { issue_batch_id: undefined, install_state: undefined, status: "used" })], total: 3, status_counts: { unused: 2, used: 1 } } };
    open();
    await screen.findByText("CODE1");
    expect(btn(rowOf("CODE1"), "Delete").disabled).toBe(true);
    expect(btn(rowOf("CODE3"), "Delete").disabled).toBe(true);
    expect(btn(rowOf("CODE1"), "Hand out").disabled).toBe(true);
    fireEvent.click(btn(rowOf("CODE2"), "Delete"));
    await waitFor(() => expect(calls.filter((c) => c.method === "DELETE").map((c) => c.url)).toEqual(["/vouchers/v2"]));
    await screen.findByText("1 of 1 deleted.");
  });

  it("bulk delete touches only deletable codes and does nothing when declined", async () => {
    pages = { 1: { vouchers: [row(1), row(2, { issue_batch_id: undefined, install_state: undefined })], total: 2, status_counts: { unused: 2 } } };
    open();
    await screen.findByText("CODE1");
    fireEvent.click(screen.getByRole("checkbox", { name: "Select this page" }));
    vi.stubGlobal("confirm", () => false);
    fireEvent.click(screen.getByRole("button", { name: "Delete (1)" }));
    expect(calls.filter((c) => c.method === "DELETE")).toHaveLength(0);
    vi.stubGlobal("confirm", () => true);
    fireEvent.click(screen.getByRole("button", { name: "Delete (1)" }));
    await waitFor(() => expect(calls.filter((c) => c.method === "DELETE").map((c) => c.url)).toEqual(["/vouchers/v2"]));
  });

  it("sends the status filter to the server and returns to the first page", async () => {
    pages = { 1: { vouchers: [row(1)], total: 150, status_counts: { unused: 100, used: 50 } } };
    open();
    await screen.findByText("CODE1");
    fireEvent.click(screen.getByRole("button", { name: "Page 2" }));
    fireEvent.click(screen.getByRole("button", { name: /^Used/ }));
    await waitFor(() => expect(calls.some((c) => c.url.includes("page=1") && c.url.includes("status=used"))).toBe(true));
  });

  it("prints a batch only when the server says it is ready", async () => {
    pages = { 1: { vouchers: [row(1)], total: 1, status_counts: { unused: 1 } } };
    const print = vi.spyOn(printing, "printCodes").mockImplementation(() => undefined);
    open();
    await screen.findByText("CODE1");
    await screen.findByRole("option", { name: /Main shop/ });
    fireEvent.change(screen.getByRole("combobox"), { target: { value: BATCH } });
    fireEvent.click(await screen.findByRole("button", { name: "Print this batch" }));
    await waitFor(() => expect(print).toHaveBeenCalledTimes(1));
  });
});
