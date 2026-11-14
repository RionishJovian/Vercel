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
let access: Record<string, unknown>, saved: Array<Record<string, unknown>>, forbidPremium = false;

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en");
  access = { exempt: false, unlocked: ["starlink_mm", "daylight"] }; saved = []; forbidPremium = false;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", "");
    if (path === "/portal/template-access") return json(200, access);
    if (path === "/portal/router-preview") return json(200, { html: "<p>login preview</p>" });
    if (path === "/portal/config" && init?.method === "PUT") { const b = JSON.parse(String(init.body)); saved.push(b); if (forbidPremium && b.template_id && !["starlink_mm", "daylight"].includes(b.template_id)) return json(403, { error: "locked" }); config = { ...config, ...JSON.parse(String(init.body)) }; return json(200, {}); }
    if (path === "/portal/config") return json(200, { config });
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
  it("shows the hosted Guest Portal preview on its own tab", async () => {
    config = { voucher_login_enabled: true, mobile_payment_enabled: true, guest_plans: [PLAN] };
    await open();
    fireEvent.click(screen.getByRole("tab", { name: "Pay page" }));
    expect(screen.getByTitle("Guest Portal").getAttribute("src")).toBe("/portal.html?preview=1");
    expect(document.querySelectorAll(".gp-preview iframe")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Voucher page" })).toBeNull();
  });
  it("warns in both the preview and the install review", async () => {
    config = { voucher_login_enabled: true, mobile_payment_enabled: true, guest_plans: [] };
    await open();
    await screen.findByText(EDITOR);
    await screen.findByText(INSTALL);
    expect(screen.getByRole("button", { name: /Review/i }).hasAttribute("disabled")).toBe(true);
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
    expect(screen.getByRole("button", { name: /Review/i }).hasAttribute("disabled")).toBe(false);
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

describe("login page themes and the Store add-on lock", () => {
  const pickCard = (id: string) => fireEvent.click(document.querySelector(`[data-template="${id}"]`)!);
  const badges = (id: string) => document.querySelector(`[data-template="${id}"]`)!.textContent ?? "";
  it("lists nine themes in two groups and marks seven as add-ons", async () => {
    config = { business_name: "Cafe" };
    await open();
    expect(document.querySelectorAll("[data-template]")).toHaveLength(9);
    expect(screen.getByText("Myanmar ISP & regional hotspot themes")).toBeTruthy();
    expect(screen.getByText("Commercial & hospitality themes")).toBeTruthy();
    for (const id of ["cultural_mm", "mountain_mm", "village_mm", "obsidian", "cafe", "hotel", "express"]) expect(badges(id)).toContain("Add-on");
    for (const id of ["starlink_mm", "daylight"]) expect(badges(id)).not.toContain("Add-on");
  });
  it("previews a locked theme but swaps Save for Unlock in Store", async () => {
    config = { business_name: "Cafe" };
    await open();
    pickCard("hotel");
    const unlock = await screen.findByRole("link", { name: /Unlock in Store/ });
    expect(unlock.getAttribute("href")).toBe("/app/store");
    expect(screen.queryByRole("button", { name: /^Save/ })).toBeNull();
    await waitFor(() => expect(document.querySelector("iframe[title='Login page']")?.getAttribute("srcdoc")).toContain("login preview"));
  });
  it("saves a free theme with no add-on", async () => {
    config = { business_name: "Cafe" };
    await open();
    pickCard("daylight");
    fireEvent.click(screen.getByRole("button", { name: /^Save/ }));
    await waitFor(() => expect(saved).toHaveLength(1));
    expect(saved[0].template_id).toBe("daylight");
  });
  it("saves a locked theme once the add-on is approved", async () => {
    access = { exempt: false, unlocked: ["starlink_mm", "daylight", "cafe"] };
    config = { business_name: "Cafe" };
    await open();
    pickCard("cafe");
    fireEvent.click(await screen.findByRole("button", { name: /^Save/ }));
    await waitFor(() => expect(saved[0]?.template_id).toBe("cafe"));
  });
  it("lets a platform administrator use every theme", async () => {
    access = { exempt: true, unlocked: ["starlink_mm", "cultural_mm", "mountain_mm", "village_mm", "obsidian", "cafe", "hotel", "express", "daylight"] };
    config = { business_name: "Cafe" };
    await open();
    for (const id of ["cultural_mm", "mountain_mm", "village_mm", "obsidian", "cafe", "hotel", "express"]) {
      pickCard(id);
      expect(screen.queryByRole("link", { name: /Unlock in Store/ })).toBeNull();
      expect(badges(id)).not.toContain("Add-on");
    }
  });
  it("does not resend an unchanged theme, so older portals keep saving", async () => {
    config = { business_name: "Cafe", template_id: "" };
    await open();
    fireEvent.click(screen.getByRole("button", { name: /^Save/ }));
    await waitFor(() => expect(saved).toHaveLength(1));
    expect("template_id" in saved[0]).toBe(false);
  });
  it("explains a 403 from the server instead of a generic failure", async () => {
    forbidPremium = true;
    access = { exempt: false, unlocked: ["starlink_mm", "daylight", "express"] };
    config = { business_name: "Cafe" };
    await open();
    pickCard("express");
    fireEvent.click(await screen.findByRole("button", { name: /^Save/ }));
    await screen.findAllByText(/Store add-on/);
  });
});
