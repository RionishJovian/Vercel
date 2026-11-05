import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { GuestPage } from "./GuestPage";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const PLAN = { key: "hour", label: "1 hour", minutes: 60, amount: 1000, data_mb: 1024 };
const EDITOR = /Guests can't pay by bank transfer yet/;
const INSTALL = /Bank transfer is on, but no plan is saved/;
let config: Record<string, unknown>;

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/portal/config" && init?.method === "PUT") { config = { ...config, ...JSON.parse(String(init.body)) }; return json(200, {}); }
    if (path === "/portal/config") return json(200, { config });
    if (path === "/portal/router-preview") return json(200, { html: "<p>preview</p>" });
    if (path === "/routers") return json(200, { routers: [{ device_id: "d1", hardware_name: "Shop", deployment_layer: "MAIN_GATEWAY" }] });
    if (path.endsWith("/portal/profiles")) return json(200, { profiles: ["hotspot1"] });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = async () => {
  render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><I18nProvider><GuestPage /></I18nProvider></SWRConfig>);
  await screen.findByText("Add plan");
};

describe("bank transfer with no saved plans", () => {
  it("warns in both the preview and the install review", async () => {
    config = { voucher_login_enabled: true, mobile_payment_enabled: true, guest_plans: [] };
    await open();
    await screen.findByText(EDITOR);
    await screen.findByText(INSTALL);
  });
  it("treats an older saved page without the setting as bank transfer on", async () => {
    config = { business_name: "Old page" };
    await open();
    await screen.findByText(EDITOR);
    await screen.findByText(INSTALL);
  });
  it("stays quiet when a plan is saved", async () => {
    config = { voucher_login_enabled: true, mobile_payment_enabled: true, guest_plans: [PLAN] };
    await open();
    await screen.findByText("hotspot1");
    expect(screen.queryByText(EDITOR)).toBeNull();
    expect(screen.queryByText(INSTALL)).toBeNull();
  });
  it("stays quiet when bank transfer is off", async () => {
    config = { voucher_login_enabled: true, mobile_payment_enabled: false, guest_plans: [] };
    await open();
    await screen.findByText("hotspot1");
    expect(screen.queryByText(EDITOR)).toBeNull();
    expect(screen.queryByText(INSTALL)).toBeNull();
  });
  it("keeps warning about the install until the new plan is saved", async () => {
    config = { voucher_login_enabled: true, mobile_payment_enabled: true, guest_plans: [] };
    await open();
    await screen.findByText(INSTALL);
    fireEvent.click(screen.getByText("Add plan"));
    const fields = document.querySelectorAll<HTMLInputElement>(".gp-form input[type=number]");
    fireEvent.change(fields[fields.length - 5], { target: { value: "60" } });
    await waitFor(() => expect(screen.queryByText(EDITOR)).toBeNull());
    expect(screen.getByText(INSTALL)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /^Save/ }));
    await waitFor(() => expect(screen.queryByText(INSTALL)).toBeNull());
  });
});
