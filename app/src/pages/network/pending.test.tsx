import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";

const router = { device_id: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", hardware_name: "hAP ax2 lab", deployment_layer: "MAIN_GATEWAY" } as never;
let pending: { revert_at: string } | undefined;
let posts: string[];
let failNext: string | undefined;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  posts = []; failNext = undefined;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
    if (init?.method === "POST") {
      posts.push(String(url));
      if (failNext) return json({ error: failNext }, 409);
      pending = undefined;
      return json({ ok: true, result: {} });
    }
    return json({ inventory: { interfaces: [{ name: "ether3", type: "ether" }], available: { vlans: true }, objects: { vlans: [] }, write_enabled: true, address_write_enabled: true, pending_confirm: pending } });
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const soon = () => ({ revert_at: new Date(Date.now() + 240_000).toISOString() });
const mount = () => render(<I18nProvider><NetworkEditor workspace="vlan" canEdit router={router} /></I18nProvider>);

describe("a change waiting to be kept or reverted", () => {
  it("shows after a reload, locks the form and counts down", async () => {
    pending = soon();
    const { container } = mount();
    expect(await screen.findByText(/Waiting for you to keep this change/)).toBeTruthy();
    expect(screen.getByRole("timer").textContent).toMatch(/Time left: [34]:\d\d/);
    expect((container.querySelector("fieldset.nolock") as HTMLFieldSetElement).disabled).toBe(true);
  });

  it("Keep sends confirm, then the panel goes away and says so", async () => {
    pending = soon();
    mount();
    fireEvent.click(await screen.findByRole("button", { name: "Keep these changes" }));
    await waitFor(() => expect(posts).toEqual([expect.stringMatching(/network\/confirm$/)]));
    expect(await screen.findByText(/Change kept/)).toBeTruthy();
    expect(screen.queryByText(/Waiting for you to keep this change/)).toBeNull();
  });

  it("Revert now sends revert", async () => {
    pending = soon();
    mount();
    fireEvent.click(await screen.findByRole("button", { name: "Revert now" }));
    await waitFor(() => expect(posts).toEqual([expect.stringMatching(/network\/revert$/)]));
    expect(await screen.findByText(/Change reverted/)).toBeTruthy();
  });

  it("a refused Keep stays on screen with the reason so the owner can still revert", async () => {
    pending = soon(); failNext = "the time ran out, so the router has returned to its previous settings";
    mount();
    fireEvent.click(await screen.findByRole("button", { name: "Keep these changes" }));
    expect(await screen.findByText(/the time ran out/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Revert now" })).toBeTruthy();
  });

  it("without a pending change nothing extra is shown and the form is usable", async () => {
    pending = undefined;
    const { container } = mount();
    await waitFor(() => expect((container.querySelector("fieldset.nolock") as HTMLFieldSetElement).disabled).toBe(false));
    expect(screen.queryByText(/Waiting for you to keep this change/)).toBeNull();
  });
});
