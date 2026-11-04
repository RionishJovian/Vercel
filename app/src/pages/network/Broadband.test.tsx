import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Broadband } from "./Broadband";

let calls: Array<{ path: string; body: Record<string, unknown> }>;
const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", operation = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); calls = [];
  vi.stubGlobal("confirm", () => true);
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === "POST") calls.push({ path: String(url), body: JSON.parse(String(init.body)) });
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
