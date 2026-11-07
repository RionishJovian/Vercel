import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig, useSWRConfig } from "swr";
import { I18nProvider } from "../i18n";
import { ProfilePage } from "./Profile";
let role = "DEVELOPER";
let receiver = "disabled";
let refresh: () => Promise<unknown>;
let saves: Record<string, unknown>[];
const json = (body: unknown) => new Response(JSON.stringify(body), { headers: { "content-type": "application/json" } });
beforeEach(() => {
  role = "DEVELOPER"; receiver = "disabled"; saves = [];
  vi.stubGlobal("localStorage", { getItem: () => "en", setItem: () => undefined });
  vi.stubGlobal("fetch", vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith("/me/profile")) return json({ full_name: "Platform owner", email: "fixture@example.test", user_role: role });
    if (options?.method === "PUT") { saves.push(JSON.parse(String(options.body))); return json({}); }
    if (url.endsWith("/test")) return json({ ok: true });
    return json({ enabled: true, target_chat_id: 123, approver_user_id: 123, bot_token: "••••abcd", runtime: { scope: role === "DEVELOPER" ? "platform" : "business", receiver, token: "valid", approver_configured: true, last_delivery: "not_tested" } });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function Page() { const { mutate } = useSWRConfig(); refresh = () => mutate("/guest-wifi/telegram-bot"); return <ProfilePage />; }
const open = () => render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><Page /></SWRConfig></I18nProvider>);
describe("Account Telegram settings", () => {
  it("keeps one platform form and distinguishes token validity from a disabled receiver", async () => {
    open();
    await screen.findByText(/Your platform bot handles app-pass/);
    expect(screen.getAllByLabelText("Bot token")).toHaveLength(1);
    expect(screen.getByText("Valid")).toBeTruthy();
    expect(screen.getByText("Disabled")).toBeTruthy();
    expect(screen.getByText("No delivery recorded")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Test bot" }));
    await screen.findByText(/Token valid and webhook setup compatible/);
    expect(screen.getByText("Disabled")).toBeTruthy();
  });
  it("preserves a token being typed when background status changes", async () => {
    open();
    const token = await screen.findByLabelText("Bot token") as HTMLInputElement;
    fireEvent.change(token, { target: { value: "new-fixture-token" } });
    receiver = "running";
    await act(async () => { await refresh(); });
    expect(screen.getByText("Running")).toBeTruthy();
    expect((screen.getByLabelText("Bot token") as HTMLInputElement).value).toBe("new-fixture-token");
  });
  it("keeps the saved token when only approval settings are saved", async () => {
    open();
    await screen.findByLabelText("Bot token");
    fireEvent.click(screen.getByRole("button", { name: "Save Telegram settings" }));
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]).toEqual({ bot_token: "", target_chat_id: 123, approver_user_id: 123, enabled: true });
  });
  it("rejects mismatched private-chat and approver IDs before saving", async () => {
    open();
    const chat = await screen.findByLabelText("Approval chat ID");
    fireEvent.change(chat, { target: { value: "999" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Telegram settings" }));
    await screen.findByText(/For a private chat, the approval chat ID must match/);
    expect(saves).toHaveLength(0);
  });
  it("explains customer isolation instead of platform scope", async () => {
    role = "OWNER";
    open();
    await screen.findByText(/Your business bot handles your guest-payment/);
    expect(screen.queryByText(/Your platform bot handles/)).toBeNull();
  });
});
