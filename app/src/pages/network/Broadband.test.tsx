import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Broadband } from "./Broadband";

let calls: Array<{ path: string; body: Record<string, unknown> }>;
let inventoryStatus: number, inventoryBody: unknown, syncStatus: number, accountsStatus: number;
const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", operation = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); calls = [];
  inventoryStatus = 200; syncStatus = 200; accountsStatus = 200;
  inventoryBody = { inventory: { available: { ppp_profiles: true }, objects: { ppp_profiles: [{ values: { name: "default" } }] } } };
  vi.stubGlobal("confirm", vi.fn(() => true));
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === "POST") calls.push({ path: String(url), body: JSON.parse(String(init.body)) });
    if (String(url).endsWith("/network/inventory")) return new Response(JSON.stringify(inventoryBody), { status: inventoryStatus, headers: { "content-type": "application/json" } });
    if (String(url).endsWith("/broadband/sync")) return new Response(JSON.stringify(syncStatus === 200 ? { synchronized: 0 } : { error: "router unavailable" }), { status: syncStatus, headers: { "content-type": "application/json" } });
    if (init?.method !== "POST" && accountsStatus !== 200) return new Response(JSON.stringify({ error: "temporarily unavailable" }), { status: accountsStatus, headers: { "content-type": "application/json" } });
    return new Response(JSON.stringify(init?.method === "POST" ? { operation: { state: "applied" } } : { accounts: [{ id, username: "home001", profile: "default", observation: null, pending_operation: operation }], write_enabled: true }), { status: 200, headers: { "content-type": "application/json" } });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = (canEdit = true) => render(<SWRConfig value={{ provider: () => new Map() }}><I18nProvider><Broadband routerId="router-a" routerName="Shop router" canEdit={canEdit} /></I18nProvider></SWRConfig>);

it("recovers a failed account read without router writes, including for viewers", async () => {
  accountsStatus = 503;
  open(false);
  await screen.findByText("Broadband records are unavailable. Existing services are unchanged.");
  expect(screen.getByText("Shop router")).toBeTruthy();
  const normalFetch = globalThis.fetch;
  let finishRead!: () => void;
  const held = new Promise<void>((resolve) => { finishRead = resolve; });
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => { await held; return normalFetch(url, init); }));
  const reload = screen.getByRole("button", { name: "Reload records" });
  fireEvent.click(reload); fireEvent.click(reload);
  expect(reload.matches(":disabled")).toBe(true);
  accountsStatus = 200; finishRead();
  expect(await screen.findByText("home001")).toBeTruthy();
  expect(screen.queryByText("Broadband records are unavailable. Existing services are unchanged.")).toBeNull();
  expect(calls).toEqual([]);
  expect(window.confirm).not.toHaveBeenCalled();
});

it("retains form details and an uncertain paid identity across failed account reads", async () => {
  const normalFetch = globalThis.fetch;
  let loseReply = true;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === "POST" && loseReply) {
      calls.push({ path: String(url), body: JSON.parse(String(init.body)) });
      loseReply = false; accountsStatus = 503;
      throw new TypeError("reply lost");
    }
    return normalFetch(url, init);
  }));
  open(); fireEvent.click(screen.getByText("New account", { exact: true }));
  const install = screen.getByRole("button", { name: "Install account" });
  await waitFor(() => expect(install.matches(":disabled")).toBe(false));
  fireEvent.change(screen.getByLabelText("Username"), { target: { value: "home002" } });
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "synthetic-password" } });
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "default" } });
  fireEvent.submit(install.closest("form")!);
  await screen.findByText("Broadband records are unavailable. Existing services are unchanged.");
  expect((screen.getByLabelText("Username") as HTMLInputElement).value).toBe("home002");
  expect((screen.getByLabelText("Password") as HTMLInputElement).value).toBe("synthetic-password");
  expect(install.matches(":disabled")).toBe(true);
  expect(screen.getByRole("button", { name: "Retry saved operation" }).matches(":disabled")).toBe(true);
  accountsStatus = 200;
  fireEvent.click(screen.getByRole("button", { name: "Reload records" }));
  await waitFor(() => expect(install.matches(":disabled")).toBe(false));
  expect(calls).toHaveLength(1);
  fireEvent.submit(install.closest("form")!);
  await screen.findByText("Router change verified. Payment records were not duplicated.");
  expect(calls).toHaveLength(2);
  expect(calls[1].body).toEqual(calls[0].body);
});

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

it("reuses an unconfirmed renewal identity after the server applies it but the reply is lost", async () => {
  const payments = new Set<string>();
  let loseReply = true;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      calls.push({ path: String(url), body });
      payments.add(body.operation_id);
      if (loseReply) { loseReply = false; throw new TypeError("response lost after commit"); }
      return response({ operation: { state: "applied" } });
    }
    if (String(url).endsWith("/network/inventory")) return response(inventoryBody);
    return response({ accounts: [{ id, username: "home001", profile: "default", observation: { first_use: 1793750400, expires_at: 4102444800, suspended: false, last_clock: 1793750400 } }], write_enabled: true });
  }));
  open();
  const renew = await screen.findByRole("button", { name: "Renew" });
  fireEvent.click(renew);
  await screen.findByText("Change not confirmed. Retry the same action with the same details before recording another payment.");
  await waitFor(() => expect(renew.matches(":disabled")).toBe(false));
  fireEvent.click(renew);
  await screen.findByText("Router change verified. Payment records were not duplicated.");
  expect(calls).toHaveLength(2);
  expect(payments.size).toBe(1);
  expect(calls[1].body).toEqual(calls[0].body);
  expect(vi.mocked(window.confirm).mock.calls[1][0]).toContain("Retry the earlier unconfirmed change with these details.");

  // Once acknowledged, a deliberate next renewal is a distinct purchase.
  await waitFor(() => expect(renew.matches(":disabled")).toBe(false));
  fireEvent.click(renew);
  await waitFor(() => expect(calls).toHaveLength(3));
  expect(payments.size).toBe(2);
  expect(vi.mocked(window.confirm).mock.calls[2][0]).not.toContain("Retry the earlier unconfirmed change with these details.");
});

it("releases a definitively rejected installation identity for a corrected attempt", async () => {
  const normalFetch = globalThis.fetch;
  let reject = true;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method !== "POST") return normalFetch(url, init);
    calls.push({ path: String(url), body: JSON.parse(String(init.body)) });
    const status = reject ? 422 : 200;
    reject = false;
    return new Response(JSON.stringify(status === 422 ? { rejected: true } : { operation: { state: "applied" } }), { status, headers: { "content-type": "application/json" } });
  }));
  open(); fireEvent.click(screen.getByText("New account", { exact: true }));
  const install = screen.getByRole("button", { name: "Install account" });
  await waitFor(() => expect(install.matches(":disabled")).toBe(false));
  fireEvent.change(screen.getByRole("textbox", { name: "Username" }), { target: { value: "home002" } });
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "synthetic-password" } });
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "default" } });
  const form = install.closest("form")!;
  fireEvent.submit(form);
  await screen.findByText("Account was not installed. Choose a different username or PPP plan, then try again.");
  await waitFor(() => expect(install.matches(":disabled")).toBe(false));
  fireEvent.submit(form);
  await screen.findByText("Router change verified. Payment records were not duplicated.");
  expect(calls).toHaveLength(2);
  expect(calls[1].body.operation_id).not.toBe(calls[0].body.operation_id);
});

it("releases the local identity when recovery succeeds through the saved-operation route", async () => {
  let saved: string | undefined, first = true;
  const payments = new Set<string>();
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      calls.push({ path: String(url), body });
      const op = body.operation_id ?? saved;
      payments.add(op);
      if (first) { first = false; saved = op; throw new TypeError("reply lost after router application"); }
      saved = undefined;
      return response({ operation: { state: "applied" } });
    }
    if (String(url).endsWith("/network/inventory")) return response(inventoryBody);
    return response({ accounts: [{ id, username: "home001", profile: "default", pending_operation: saved, observation: { first_use: 1793750400, expires_at: 4102444800, suspended: false, last_clock: 1793750400 } }], write_enabled: true });
  }));
  open(); fireEvent.click(await screen.findByRole("button", { name: "Renew" }));
  fireEvent.click(await screen.findByRole("button", { name: "Retry saved operation" }));
  const renew = await screen.findByRole("button", { name: "Renew" });
  fireEvent.click(renew);
  await waitFor(() => expect(calls).toHaveLength(3));
  expect(calls[1].path).toContain(`/operations/${calls[0].body.operation_id}/retry`);
  expect(payments.size).toBe(2);
});

it("does not replay an obsolete suspension after a successful resume", async () => {
  let suspended = false, first = true;
  const seen = new Set<string>();
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      calls.push({ path: String(url), body });
      if (!seen.has(body.operation_id)) suspended = String(url).endsWith("/suspend");
      seen.add(body.operation_id);
      if (first) { first = false; throw new TypeError("reply lost after suspension"); }
      return response({ operation: { state: "applied" } });
    }
    if (String(url).endsWith("/network/inventory")) return response(inventoryBody);
    return response({ accounts: [{ id, username: "home001", profile: "default", observation: { first_use: 1793750400, expires_at: 4102444800, suspended, last_clock: 1793750400 } }], write_enabled: true });
  }));
  open(); fireEvent.click(await screen.findByRole("button", { name: "Suspend" }));
  fireEvent.click(await screen.findByRole("button", { name: "Resume" }));
  fireEvent.click(await screen.findByRole("button", { name: "Suspend" }));
  await waitFor(() => expect(calls).toHaveLength(3));
  expect(suspended).toBe(true);
  expect(calls[2].body.operation_id).not.toBe(calls[0].body.operation_id);
});

it("preserves an unconfirmed renewal when another account completes an action", async () => {
  const other = "cccccccc-cccc-4ccc-8ccc-cccccccccccc", payments = new Set<string>();
  let lose = true;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      calls.push({ path: String(url), body }); payments.add(body.operation_id);
      if (lose) { lose = false; throw new TypeError("response lost"); }
      return response({ operation: { state: "applied" } });
    }
    if (String(url).endsWith("/network/inventory")) return response(inventoryBody);
    return response({ accounts: [id, other].map((account, i) => ({ id: account, username: `home${i}`, profile: "default", observation: { first_use: 1793750400, expires_at: 4102444800, suspended: false, last_clock: 1793750400 } })), write_enabled: true });
  }));
  open(); await screen.findByText("home0");
  const renew = (name: string) => within(screen.getByText(name).closest("article")!).getByRole("button", { name: "Renew" });
  fireEvent.click(renew("home0"));
  await screen.findByText("Change not confirmed. Retry the same action with the same details before recording another payment.");
  fireEvent.click(renew("home1"));
  await screen.findByText("Router change verified. Payment records were not duplicated.");
  fireEvent.click(renew("home0"));
  await waitFor(() => expect(calls).toHaveLength(3));
  expect(calls[2].body.operation_id).toBe(calls[0].body.operation_id);
  expect(payments.size).toBe(2);
});

it("does not discard a newer uncertain payment when acknowledging an older one", async () => {
  const payments = new Set<string>();
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      calls.push({ path: String(url), body }); payments.add(body.operation_id);
      if (calls.length <= 2) throw new TypeError("reply lost after payment");
      return response({ operation: { state: "applied" } });
    }
    if (String(url).endsWith("/network/inventory")) return response(inventoryBody);
    return response({ accounts: [{ id, username: "home001", profile: "default", observation: { first_use: 1793750400, expires_at: 4102444800, suspended: false, last_clock: 1793750400 } }], write_enabled: true });
  }));
  open(); const renew = await screen.findByRole("button", { name: "Renew" });
  const amount = screen.getByLabelText("Payment received (MMK)");
  const lostReply = "Change not confirmed. Retry the same action with the same details before recording another payment.";
  fireEvent.change(amount, { target: { value: "15000" } }); fireEvent.click(renew);
  await screen.findByText(lostReply);
  fireEvent.change(amount, { target: { value: "20000" } }); fireEvent.click(renew);
  await waitFor(() => expect(calls).toHaveLength(2)); await screen.findByText(lostReply);
  fireEvent.change(amount, { target: { value: "15000" } }); fireEvent.click(renew);
  await screen.findByText("Router change verified. Payment records were not duplicated.");
  fireEvent.change(amount, { target: { value: "20000" } }); fireEvent.click(renew);
  await waitFor(() => expect(calls).toHaveLength(4));
  expect(calls[3].body.operation_id).toBe(calls[1].body.operation_id);
  expect(payments.size).toBe(2);
});

it("preserves uncertain payments when delivery order differs from click order", async () => {
  const payments = new Set<string>();
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body)); calls.push({ path: String(url), body });
      if (calls.length === 1) throw new TypeError("request never arrived");
      payments.add(body.operation_id);
      if (calls.length <= 3) throw new TypeError("reply lost after payment");
      return response({ operation: { state: "applied" } });
    }
    if (String(url).endsWith("/network/inventory")) return response(inventoryBody);
    return response({ accounts: [{ id, username: "home001", profile: "default", observation: { first_use: 1793750400, expires_at: 4102444800, suspended: false, last_clock: 1793750400 } }], write_enabled: true });
  }));
  open(); const renew = await screen.findByRole("button", { name: "Renew" });
  const amount = screen.getByLabelText("Payment received (MMK)");
  for (const [i, value] of ["15000", "20000", "15000"].entries()) {
    fireEvent.change(amount, { target: { value } }); fireEvent.click(renew);
    await waitFor(() => expect(calls).toHaveLength(i + 1));
    await screen.findByText("Change not confirmed. Retry the same action with the same details before recording another payment.");
  }
  fireEvent.change(amount, { target: { value: "20000" } }); fireEvent.click(renew);
  await screen.findByText("Router change verified. Payment records were not duplicated.");
  fireEvent.change(amount, { target: { value: "15000" } }); fireEvent.click(renew);
  await waitFor(() => expect(calls).toHaveLength(5));
  expect(calls[4].body.operation_id).toBe(calls[0].body.operation_id);
  expect(payments.size).toBe(2);
});
