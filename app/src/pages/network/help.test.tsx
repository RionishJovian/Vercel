import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { NetworkEditor } from "./NetworkEditor";
import { Network } from "./index";
import { descriptions, schema } from "./schema";

const router = { device_id: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", hardware_name: "hAP ax2 lab", deployment_layer: "MAIN_GATEWAY" } as never;
const me = { user: { user_role: "OWNER" } } as never;
const network = (sub: string) => render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), shouldRetryOnError: false }}><Network me={me} sub={sub} /></SWRConfig></I18nProvider>);

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ routers: [] }), { status: 200, headers: { "content-type": "application/json" } })));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("explanations stay behind help", () => {
  it("task buttons carry a name only, and the description opens from ?", async () => {
    const { container } = render(<I18nProvider><NetworkEditor workspace="qos" canEdit router={router} /></I18nProvider>);
    await waitFor(() => expect(container.querySelectorAll(".task").length).toBe(2));
    expect(document.body.textContent).not.toContain(descriptions.fairshare);
    fireEvent.click(screen.getByRole("button", { name: "Help" }));
    expect(document.body.textContent).toContain(descriptions.fairshare);
  });
  it("a page with one task shows its name once and opens on the form", async () => {
    const { container } = network("vlan");
    await screen.findByRole("heading", { level: 1, name: "VLANs" });
    await waitFor(() => expect(container.querySelector('input[name="vlan-id"]')).toBeTruthy());
    expect(container.querySelector(".task")).toBeNull();
    expect(screen.getAllByRole("heading").filter((h) => h.textContent === schema.vlan.title)).toHaveLength(1);
    expect(document.body.textContent).not.toContain(descriptions.vlan);
  });
});

describe("network page titles", () => {
  it("Internet connections has the same title with and without its address", async () => {
    const first = network("");
    await screen.findByRole("heading", { level: 1, name: "Internet connections" });
    first.unmount();
    network("wan");
    await screen.findByRole("heading", { level: 1, name: "Internet connections" });
  });
  it("New network setup keeps the section pills, with its own lit", async () => {
    const { container } = network("new");
    await screen.findByRole("heading", { level: 1, name: "New network setup" });
    expect(container.querySelector('a[href="/app/network/new"]')?.getAttribute("aria-current")).toBe("page");
    expect(container.querySelector('a[href="/app/network/wan"]')).toBeTruthy();
  });
});
