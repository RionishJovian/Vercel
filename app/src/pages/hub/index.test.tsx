import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { I18nProvider } from "../../i18n";
import { Hub } from "./index";

const CONNECTED = { device_id: "aaaaaaaa-0000-4000-8000-000000000001", hardware_name: "Shop router", deployment_layer: "MAIN_GATEWAY", transport: "tunnel", management_ip: "10.77.0.20", management_connected: true };
const NEW = { device_id: "aaaaaaaa-0000-4000-8000-000000000002", hardware_name: "New router", deployment_layer: "MAIN_GATEWAY", transport: "tunnel" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
let routers: unknown[], deleteAnswer: { status: number; body: unknown }, deleted: string[];

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); deleted = []; routers = [CONNECTED];
  deleteAnswer = { status: 200, body: { message: "router deleted", router_cleanup_script: "# remove the setup" } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url).replace("/api/v1", "");
    if (init?.method === "DELETE") { deleted.push(path); return json(deleteAnswer.status, deleteAnswer.body); }
    if (path === "/routers") return json(200, { routers });
    if (path === "/vps/enrollment-capability") return json(200, { available: true, lookup_enabled: true });
    return json(200, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = () => render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), shouldRetryOnError: false }}><Hub me={{ user: { user_role: "OWNER" } } as never} /></SWRConfig></I18nProvider>);
const startDelete = async () => {
  await screen.findByText("Shop router");
  fireEvent.click(screen.getByRole("button", { name: "Delete record" }));
  const dialog = await screen.findByRole("alertdialog");
  fireEvent.click([...dialog.querySelectorAll("button")].find((b) => b.textContent === "Delete record")!);
};

describe("Magic Hub card", () => {
  it("shows a connected router as connected, with no way to connect again", async () => {
    open();
    await screen.findByText("Connected to Magic Hub");
    expect(screen.queryByRole("button", { name: "Connect Magic Hub" })).toBeNull();
  });
  it("offers Connect only to a router not connected yet", async () => {
    routers = [NEW];
    open();
    await screen.findByRole("button", { name: "Connect Magic Hub" });
    expect(screen.queryByText("Connected to Magic Hub")).toBeNull();
  });
  it("hands over the cleanup script after a delete, then refreshes on Done", async () => {
    open();
    await startDelete();
    await screen.findByDisplayValue("# remove the setup");
    expect(deleted).toEqual([`/routers/${CONNECTED.device_id}`]);
    routers = [];
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByText("Shop router")).toBeNull());
  });
  it("refreshes straight away when there is nothing to clean up", async () => {
    deleteAnswer = { status: 200, body: { message: "router deleted" } };
    open();
    await startDelete();
    await waitFor(() => expect(deleted).toHaveLength(1));
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
  });
  it("names the router in the delete question", async () => {
    open();
    await screen.findByText("Shop router");
    fireEvent.click(screen.getByRole("button", { name: "Delete record" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog.textContent).toContain("Shop router");
  });
  it("explains a kept record, then offers to connect again", async () => {
    deleteAnswer = { status: 200, body: { message: "kept", record_kept: true, router_cleanup_script: "# remove the setup" } };
    open();
    await startDelete();
    await screen.findByText("Connection ended. History kept.");
    await screen.findByDisplayValue("# remove the setup");
    routers = [{ ...CONNECTED, management_connected: false, management_ip: undefined }];
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await screen.findByRole("button", { name: "Connect Magic Hub" });
    screen.getByText(/stay on its record/);
    expect(screen.queryByText("Connected to Magic Hub")).toBeNull();
    expect(screen.getByText("Shop router")).toBeTruthy();
  });
  it("explains a kept record when there is nothing to clean up", async () => {
    deleteAnswer = { status: 200, body: { message: "kept", record_kept: true } };
    routers = [{ ...CONNECTED, management_connected: false }];
    open();
    await startDelete();
    await screen.findByText(/stay on its record/);
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
  });
  it("shows why a delete was refused and keeps the card", async () => {
    deleteAnswer = { status: 503, body: { error: "Magic Hub could not end this router's connection, so nothing was deleted. Try again." } };
    open();
    await startDelete();
    await screen.findByText(/nothing was deleted/);
    expect(screen.getByText("Shop router")).toBeTruthy();
  });
});
