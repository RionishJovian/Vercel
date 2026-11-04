import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { BroadbandRevenue } from "./BroadbandRevenue";

let status: number;
let report: { amount_mmk: number; today_mmk: number; payment_count: number; pending_count: number };
let requests: Array<{ url: string; method: string }>;
beforeEach(() => {
 localStorage.setItem("mm2_lang", "en");
 status = 200; requests = [];
 report = { amount_mmk: 45000, today_mmk: 15000, payment_count: 3, pending_count: 1 };
 vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
  requests.push({ url: String(url), method: init?.method ?? "GET" });
  return new Response(JSON.stringify(status === 200 ? report : { error: "report unavailable" }), { status, headers: { "content-type": "application/json" } });
 }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = () => render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><BroadbandRevenue /></I18nProvider></SWRConfig>);

it("shows confirmed zero totals without treating them as unavailable", async () => {
 report = { amount_mmk: 0, today_mmk: 0, payment_count: 0, pending_count: 0 };
 open();
 expect(await screen.findAllByText("0 MMK")).toHaveLength(2);
 expect(screen.queryByText("Payment totals could not be refreshed.")).toBeNull();
});

it("reloads unavailable payment totals without displaying zero or submitting a change", async () => {
 status = 503; open();
 await screen.findByText("Payment totals could not be refreshed.");
 expect(screen.queryByText("Total recorded:", { exact: false })).toBeNull();
 const normalFetch = globalThis.fetch;
 let finishRead!: () => void;
 const held = new Promise<void>((resolve) => { finishRead = resolve; });
 vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => { await held; return normalFetch(url, init); }));
 const reload = screen.getByRole("button", { name: "Reload records" });
 fireEvent.click(reload); fireEvent.click(reload);
 expect(reload.matches(":disabled")).toBe(true);
 status = 200; finishRead();
 expect(await screen.findByText("45,000 MMK")).toBeTruthy();
 expect(screen.queryByText("Payment totals could not be refreshed.")).toBeNull();
 expect(requests).toEqual([
  { url: "/api/v1/broadband/report", method: "GET" },
  { url: "/api/v1/broadband/report", method: "GET" },
 ]);
});

it("labels retained totals stale after refresh failure and replaces them after recovery", async () => {
 open(); await screen.findByText("45,000 MMK");
 status = 503;
 fireEvent.click(screen.getByRole("button", { name: "Reload records" }));
 await screen.findByText("Showing last loaded payment totals.");
 expect(screen.getByText("45,000 MMK")).toBeTruthy();
 expect(screen.getByText("15,000 MMK")).toBeTruthy();
 expect(screen.getByText("Pending router confirmation: 1")).toBeTruthy();
 status = 200; report = { amount_mmk: 60000, today_mmk: 30000, payment_count: 4, pending_count: 0 };
 await waitFor(() => expect(screen.getByRole("button", { name: "Reload records" }).matches(":disabled")).toBe(false));
 fireEvent.click(screen.getByRole("button", { name: "Reload records" }));
 expect(await screen.findByText("60,000 MMK")).toBeTruthy();
 expect(screen.queryByText("45,000 MMK")).toBeNull();
 expect(screen.queryByText("Showing last loaded payment totals.")).toBeNull();
 expect(screen.queryByText("Pending router confirmation: 1")).toBeNull();
 expect(requests).toHaveLength(3);
 expect(requests.every((r) => r.url === "/api/v1/broadband/report" && r.method === "GET")).toBe(true);
});
