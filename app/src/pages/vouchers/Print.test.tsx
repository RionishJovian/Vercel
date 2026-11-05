import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Print } from "./Print";
import { printCodes, receipt } from "./printCodes";

const LAYOUT = { business_id: "b1", business_name: "Neon Cafe", wifi_name: "Hotspot", support_contact: "Front desk", terms: "Fair use.", paper_width_mm: 80, show_qr: true, show_price: true, show_expiry: true, a4_columns: 3, a4_rows_per_page: 12, thermal_columns: 2, thermal_rows_per_strip: 8, created_at: "x", updated_at: "x" };
let saved: Record<string, unknown> | null, layoutStatus: number;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  saved = null; layoutStatus = 200;
  vi.stubGlobal("fetch", vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === "PUT") { saved = JSON.parse(String(init.body)); return json(200, { layout: saved, saved: true }); }
    return layoutStatus === 200 ? json(200, { layout: LAYOUT, saved: true }) : json(layoutStatus, {});
  }));
  vi.stubGlobal("print", vi.fn());
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); document.body.innerHTML = ""; document.body.className = ""; });

const open = () => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><I18nProvider><Print /></I18nProvider></SWRConfig>);

describe("Voucher print layouts", () => {
  it("says the settings do not touch the guest portal or the Hotspot", async () => {
    open();
    await screen.findByLabelText("Business name");
    expect(screen.getByText(/do not change the guest portal or the RouterOS Hotspot/)).toBeTruthy();
    expect((screen.getByRole("link", { name: "Back to vouchers" }) as HTMLAnchorElement).getAttribute("href")).toBe("#/vouchers");
  });

  it("updates the preview as the form changes", async () => {
    open();
    fireEvent.change(await screen.findByLabelText("Business name"), { target: { value: "Jade Tea" } });
    expect(screen.getByText("Jade Tea")).toBeTruthy();
    expect(document.querySelector(".pl-qr")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Include QR code"));
    expect(document.querySelector(".pl-qr")).toBeNull();
    fireEvent.click(screen.getByLabelText("Show expiry / use terms"));
    expect(document.querySelector(".pl-terms")).toBeNull();
  });

  it("saves every field the API requires, without its read-only ones", async () => {
    open();
    fireEvent.change(await screen.findByLabelText("Thermal paper width"), { target: { value: "58" } });
    fireEvent.click(screen.getByLabelText("Show price"));
    fireEvent.click(screen.getByRole("button", { name: "Save layout" }));
    await screen.findByText("Layout saved.");
    expect(saved).toMatchObject({ paper_width_mm: 58, show_price: false, show_qr: true, a4_columns: 3, a4_rows_per_page: 12, thermal_columns: 2, thermal_rows_per_strip: 8 });
    expect(saved).not.toHaveProperty("business_id");
    expect(saved).not.toHaveProperty("updated_at");
  });

  it("shows an error rather than an empty form that could overwrite the saved layout", async () => {
    layoutStatus = 503;
    open();
    await screen.findByRole("alert");
    expect(screen.queryByLabelText("Business name")).toBeNull();
  });

  it("prints a test receipt from the unsaved form", async () => {
    open();
    fireEvent.change(await screen.findByLabelText("Business name"), { target: { value: "Jade Tea" } });
    fireEvent.click(screen.getByRole("button", { name: "Print test receipt" }));
    await waitFor(() => expect(window.print).toHaveBeenCalled());
    const root = document.querySelector(".print-root") as HTMLElement;
    expect(root.textContent).toContain("Jade Tea");
    expect(root.textContent).toContain("DEMO-1234");
    expect(saved).toBeNull();
  });
});

describe("printCodes", () => {
  it("uses the paper width for a thermal receipt and draws the QR code", () => {
    printCodes([{ code: "ABCDEFGH", plan_label: "1 hour", price_mmk: 1500, duration_minutes: 60 }], LAYOUT, "first use");
    const root = document.querySelector(".print-root") as HTMLElement;
    expect(root.classList.contains("thermal")).toBe(true);
    expect(root.style.width).toBe("80mm");
    expect(root.querySelector("svg.qr path")?.getAttribute("d")).toMatch(/^M\d/);
  });

  it("keeps the A4 grid and adds no QR code when it is off", () => {
    printCodes([{ code: "ABCDEFGH" }], { ...LAYOUT, paper_width_mm: 210, show_qr: false, a4_columns: 4 }, "first use");
    const root = document.querySelector(".print-root") as HTMLElement;
    expect(root.classList.contains("thermal")).toBe(false);
    expect(root.style.gridTemplateColumns).toContain("repeat(4");
    expect(root.querySelector("svg")).toBeNull();
  });

  it("writes voucher text as text, never as markup", () => {
    printCodes([{ code: "<img src=x>" }], { ...LAYOUT, business_name: "<b>x</b>" }, "first use");
    expect(document.querySelector(".print-root img, .print-root b b")).toBeNull();
    expect(document.querySelector(".print-root")?.textContent).toContain("<img src=x>");
  });

  it("drops the price, expiry and terms lines when they are off", () => {
    const r = receipt({ code: "A", price_mmk: 1, duration_minutes: 60 }, { ...LAYOUT, show_price: false, show_expiry: false }, "first");
    expect(r.rows.map(([k]) => k)).toEqual(["Wi-Fi", "ဆက်သွယ်ရန် / Support"]);
    expect(r.terms).toBe("");
  });
});
