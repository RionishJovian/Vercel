import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { afterEach, beforeEach, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import { AccountPass } from "./AccountPass";

const me = { user: { user_id: "u1", email: "a@b.c" }, business: { business_id: "b1" } };
const KEY = "mm-account-pass-checkout:b1:u1";
const posts: Array<{ idempotency_key: string; bank_reference: string }> = [];
let failFirst = true;

function fetchMock(url: string, init?: RequestInit) {
  const json = (o: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify(o), { status, headers: { "content-type": "application/json" } }));
  if (url.endsWith("/me/app-pass/renewals") && init?.method === "POST") {
    posts.push(JSON.parse(String(init.body)));
    if (failFirst) { failFirst = false; return Promise.reject(new TypeError("lost")); }
    return json({ ok: true });
  }
  if (url.endsWith("/me/app-pass/renewals")) return json({ requests: [] });
  if (url.endsWith("/me/app-pass/checkout-options")) return json({ available: true, plans: { monthly: { amount_mmk: 15000 }, annual: { amount_mmk: 150000 } }, bank_accounts: [{ id: "bank1", bank_name: "KBZ", holder_name: "Mega", account_number: "1234" }] });
  return json({});
}

// Node's own experimental storage globals can shadow jsdom's, so give the page a plain in-memory one.
const memoryStorage = (seed: Record<string, string> = {}): Storage => { const m = new Map(Object.entries(seed)); return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, String(v)), removeItem: (k: string) => void m.delete(k), clear: () => m.clear(), key: (i: number) => [...m.keys()][i] ?? null, get length() { return m.size; } } as Storage; };

beforeEach(() => {
  posts.length = 0; failFirst = true;
  vi.stubGlobal("sessionStorage", memoryStorage()); vi.stubGlobal("localStorage", memoryStorage({ mm2_lang: "en" }));
  vi.stubGlobal("fetch", vi.fn(fetchMock));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function mount() {
  return render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><AccountPass me={me} pass={{ state: "pass_required", authorized_login: true }} onRefresh={() => undefined} /></SWRConfig></I18nProvider>);
}

describe("account pass checkout", () => {
  it("saves the request ID before sending and reuses it after a lost response", async () => {
    mount();
    fireEvent.click(await screen.findByRole("button", { name: /choose monthly/i }));
    fireEvent.change(await screen.findByLabelText(/bank transaction reference/i), { target: { value: "TX-1001" } });
    fireEvent.click(screen.getByLabelText(/i transferred this amount/i));
    fireEvent.click(screen.getByRole("button", { name: /send for review/i }));

    await waitFor(() => expect(posts).toHaveLength(1));
    const saved = JSON.parse(sessionStorage.getItem(KEY) ?? "null");
    expect(saved.idempotency_key).toBe(posts[0].idempotency_key);

    // The response was lost: the same details come back as a retry, and money is not asked for twice.
    fireEvent.click(await screen.findByRole("button", { name: /retry the saved payment request/i }));
    await waitFor(() => expect(posts).toHaveLength(2));
    expect(posts[1]).toMatchObject({ idempotency_key: posts[0].idempotency_key, bank_reference: "TX-1001" });
    await waitFor(() => expect(sessionStorage.getItem(KEY)).toBeNull());
  });

  it("offers no payment to the platform administrator", async () => {
    render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}><AccountPass me={me} pass={{ state: "platform_exempt" }} onRefresh={() => undefined} /></SWRConfig></I18nProvider>);
    expect((await screen.findAllByRole("button", { name: /administrator exempt/i })).every((b) => (b as HTMLButtonElement).disabled)).toBe(true);
  });
});
