import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Broadband } from "./Broadband";

let calls: Array<{ path: string; body: Record<string, unknown> }>;
let inventoryStatus: number, inventoryBody: unknown, syncStatus: number;
const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", operation = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); calls = [];
  inventoryStatus = 200; syncStatus = 200;
  inventoryBody = { inventory: { available: { ppp_profiles: true }, objects: { ppp_profiles: [{ values: { name: "default" } }] } } };
  vi.stubGlobal("confirm", () => true);
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === "POST") calls.push({ path: String(url), body: JSON.parse(String(init.body)) });
    if (String(url).endsWith("/network/inventory")) return new Response(JSON.stringify(inventoryBody), { status: inventoryStatus, headers: { "content-type": "application/json" } });
    if (String(url).endsWith("/broadband/sync")) return new Response(JSON.stringify(syncStatus === 200 ? { synchronized: 0 } : { error: "router unavailable" }), { status: syncStatus, headers: { "content-type": "application/json" } });
    return new Response(JSON.stringify(init?.method === "POST" ? { operation: { state: "applied" } } : { accounts: [{ id, username: "home001", profile: "default", observation: null, pending_operation: operation }], write_enabled: true }), { status: 200, headers: { "content-type": "application/json" } });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = (canEdit = true) => render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><Broadband routerId="router-a" routerName="Shop router" canEdit={canEdit} /></I18nProvider></SWRConfig>);

it("retries the stored operation rather than recording another payment", async () => {
  open(); fireEvent.click(await screen.findByRole("button", { name: "Retry saved operation" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0].path).toContain(`/operations/${operation}/retry`);
  expect(calls[0].body).toEqual({ confirm: true });
});
it("keeps unknown observations distinct from unused accounts and prevents viewer writes", async () => {
  open(false); expect(await screen.findByText("Awaiting router confirmation")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Retry saved operation" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Install account" })).toBeNull();
});

it("guides missing management setup to Magic Hub and prevents a new installation", async () => {
  inventoryStatus = 409; inventoryBody = { code: "router_management_required", error: "setup required" };
  open();
  expect((await screen.findByRole("link", { name: "Connect Magic Hub" })).getAttribute("href")).toBe("#/hub");
  fireEvent.click(screen.getByText("New account", { exact: true }));
  expect(screen.getByRole("button", { name: "Install account" }).matches(":disabled")).toBe(true);
  expect(calls).toEqual([]);
});

it("keeps saved-operation recovery available when PPP plan inventory alone fails", async () => {
  inventoryStatus = 502; inventoryBody = { error: "inventory unavailable" };
  open();
  expect(await screen.findByText("PPP plans could not be loaded. Refresh the connection before installing a new account.")).toBeTruthy();
  expect(screen.queryByRole("link", { name: "Connect Magic Hub" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Retry saved operation" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0].path).toContain(`/operations/${operation}/retry`);
});

it("rechecks the connection and unlocks installation only after plans load", async () => {
  inventoryStatus = 409; inventoryBody = { code: "router_management_required" };
  open(); await screen.findByRole("link", { name: "Connect Magic Hub" });
  inventoryStatus = 200; inventoryBody = { inventory: { available: { ppp_profiles: true }, objects: { ppp_profiles: [{ values: { name: "home20M" } }] } } };
  fireEvent.click(screen.getByRole("button", { name: "Refresh connection" }));
  fireEvent.click(screen.getByText("New account", { exact: true }));
  await screen.findByRole("option", { name: "home20M" });
  expect(screen.queryByRole("link", { name: "Connect Magic Hub" })).toBeNull();
  expect(screen.getByRole("button", { name: "Install account" }).matches(":disabled")).toBe(false);
  expect(calls).toEqual([]);
});

it("explains an empty plan list without claiming the connection is missing", async () => {
  inventoryBody = { inventory: { available: { ppp_profiles: true }, objects: { ppp_profiles: [] } } };
  open();
  expect(await screen.findByText("No usable PPP plan was reported. Set up a PPP plan in Customer internet, then refresh the connection.")).toBeTruthy();
  expect(screen.queryByRole("link", { name: "Connect Magic Hub" })).toBeNull();
  fireEvent.click(screen.getByText("New account", { exact: true }));
  expect(screen.getByRole("button", { name: "Install account" }).matches(":disabled")).toBe(true);
});

it("does not describe a failed read-only refresh as an uncertain payment", async () => {
  syncStatus = 502;
  open(); await screen.findByText("home001"); fireEvent.click(screen.getByRole("button", { name: "Refresh from router" }));
  expect(await screen.findByText("Could not refresh router readings. The last saved readings are still shown; refreshing does not record a payment.")).toBeTruthy();
  expect(screen.queryByText("Change not confirmed. Refresh and retry the saved operation before making another payment.")).toBeNull();
  expect(calls[0]).toEqual({ path: expect.stringContaining("/broadband/sync"), body: {} });
});
