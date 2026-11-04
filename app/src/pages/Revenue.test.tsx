import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { Revenue } from "./Revenue";
import type { Me } from "../lib/types";

let report: { status: number; body: unknown };
let paths: string[];
const REPORT = { activation_today: 12500, activation_last7: 70000, activation_last30: 300000, activation_value: 910000, available_count: 40, reserved_count: 5, activated_count: 55, expired_count: 2, revoked_count: 1, gateways: [{ gateway_name: "Shop gateway", available_count: 3, activated_count: 7, activation_value: 21000 }] };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  report = { status: 200, body: REPORT }; paths = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const path = String(url).replace("/api/v1", ""); paths.push(path);
    if (path === "/broadband/report") return json(200, { amount_mmk: 0, today_mmk: 0, payment_count: 0, pending_count: 0 });
    return path === "/voucher-report" ? json(report.status, report.body) : json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (role: string) => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider><Revenue me={{ user: { user_role: role } } as unknown as Me} /></I18nProvider></SWRConfig>);

describe("Revenue", () => {
  it("shows each figure from the report and the per-router split", async () => {
    open("OWNER");
    expect(await screen.findByText("Shop gateway")).toBeTruthy();
    for (const figure of ["70,000", "300,000", "910,000"]) expect(screen.getByText(figure)).toBeTruthy();
    expect(screen.getByText(/21,000/)).toBeTruthy();
  });
  it("says the figures are unavailable instead of showing zero", async () => {
    report = { status: 500, body: { error: "boom" } };
    open("OWNER");
    expect((await screen.findByRole("alert")).textContent).toContain("unavailable");
    expect(screen.queryByText("0")).toBeNull();
  });
  it("loads PPPoE payments for an owner but not for a viewer", async () => {
    open("OWNER");
    await waitFor(() => expect(paths).toContain("/broadband/report"));
    cleanup(); paths = [];
    open("VIEWER");
    await screen.findByText("Shop gateway");
    expect(paths).not.toContain("/broadband/report");
  });
});
