import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

const open = (role = "OWNER") => render(<I18nProvider><SWRConfig value={{ provider: () => new Map(), shouldRetryOnError: false }}><Hub me={{ user: { user_role: role } } as never} /></SWRConfig></I18nProvider>);
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
  it("removes the card at once, and sends the session cookie and CSRF header, when nothing needs cleaning up", async () => {
    deleteAnswer = { status: 200, body: { message: "router deleted" } };
    open();
    await startDelete();
    await waitFor(() => expect(screen.queryByText("Shop router")).toBeNull());
    const call = (fetch as unknown as { mock: { calls: Array<[string, RequestInit]> } }).mock.calls.find(([, init]) => init?.method === "DELETE")!;
    expect(call[1].credentials).toBe("include");
    expect((call[1].headers as Record<string, string>)["X-MM-CSRF"]).toBe("1");
  });
  it("shows the server's reason next to the button, and lets the owner try again", async () => {
    deleteAnswer = { status: 409, body: { error: "Recover or cancel this router's management migration before deleting its record" } };
    open();
    await startDelete();
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() => expect(dialog.textContent).toContain("Recover or cancel this router's management migration"));
    const confirm = [...dialog.querySelectorAll("button")].find((b) => b.textContent === "Delete record")!;
    expect((confirm as HTMLButtonElement).disabled).toBe(false);
    expect(screen.getAllByText("Shop router").length).toBeGreaterThan(0);
  });
  it("says so when the server gives no reason", async () => {
    deleteAnswer = { status: 500, body: null };
    open();
    await startDelete();
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() => expect(dialog.querySelector(".note.bad, [class*='bad']")).toBeTruthy());
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
    const result = await screen.findByRole("status", { name: "Record not deleted. Connection ended." });
    expect(result.textContent).toMatch(/stay on its record/);
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByRole("status", { name: "Record not deleted. Connection ended." })).toBeNull();
  });
  it("answers a Platform Admin's second Delete on a router whose connection already ended", async () => {
    deleteAnswer = { status: 200, body: { message: "kept", record_kept: true } };
    routers = [{ ...CONNECTED, management_connected: false, last_seen_at: new Date(Date.now() - 26 * 86_400_000).toISOString() }];
    open("DEVELOPER");
    for (let press = 0; press < 2; press++) {
      await screen.findByText("Shop router");
      if (press === 0) await startDelete();
      else {
        fireEvent.click(screen.getByRole("button", { name: "Delete record" }));
        const dialog = await screen.findByRole("alertdialog");
        fireEvent.click([...dialog.querySelectorAll("button")].find((b) => b.textContent === "Delete record")!);
      }
      await screen.findByRole("status", { name: "Record not deleted. Connection ended." });
      fireEvent.click(screen.getByRole("button", { name: "Done" }));
    }
    expect(deleted).toHaveLength(2);
    const call = (fetch as unknown as { mock: { calls: Array<[string, RequestInit]> } }).mock.calls.find(([, init]) => init?.method === "DELETE")!;
    expect(call[1].credentials).toBe("include");
    expect((call[1].headers as Record<string, string>)["X-MM-CSRF"]).toBe("1");
    expect(screen.getByText("26d").className).toContain("num");
  });
  it("offers permanent delete to the Platform Admin only, and only after the exact name is typed", async () => {
    deleteAnswer = { status: 200, body: { message: "kept", record_kept: true } };
    routers = [{ ...CONNECTED, management_connected: false }];
    const view = open("OWNER");
    await startDelete();
    await screen.findByRole("status", { name: "Record not deleted. Connection ended." });
    expect(screen.queryByRole("group", { name: "Delete this record and its history for good" })).toBeNull();
    view.unmount();

    open("DEVELOPER");
    await startDelete();
    const group = await screen.findByRole("group", { name: "Delete this record and its history for good" });
    const button = [...group.querySelectorAll("button")].find((b) => b.textContent === "Delete permanently") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    const input = group.querySelector("input")!;
    fireEvent.change(input, { target: { value: "shop router" } });
    expect(button.disabled).toBe(true);
    fireEvent.change(input, { target: { value: "Shop router" } });
    expect(button.disabled).toBe(false);

    deleteAnswer = { status: 200, body: { message: "router deleted", forced: true } };
    routers = [];
    fireEvent.click(button);
    await waitFor(() => expect(screen.queryByText("Shop router")).toBeNull());
    const forcedCall = (fetch as unknown as { mock: { calls: Array<[string, RequestInit]> } }).mock.calls.find(([url, init]) => init?.method === "DELETE" && String(url).includes("force=true"))!;
    expect(String(forcedCall[0])).toContain(`/routers/${CONNECTED.device_id}?force=true&confirm=Shop%20router`);
    expect(forcedCall[1].credentials).toBe("include");
    expect((forcedCall[1].headers as Record<string, string>)["X-MM-CSRF"]).toBe("1");
  });
  it("shows why a permanent delete was refused and keeps the card", async () => {
    deleteAnswer = { status: 200, body: { message: "kept", record_kept: true } };
    routers = [{ ...CONNECTED, management_connected: false }];
    open("DEVELOPER");
    await startDelete();
    const group = await screen.findByRole("group", { name: "Delete this record and its history for good" });
    fireEvent.change(group.querySelector("input")!, { target: { value: "Shop router" } });
    deleteAnswer = { status: 409, body: { error: "This record is still linked to other history, so nothing was deleted." } };
    fireEvent.click([...group.querySelectorAll("button")].find((b) => b.textContent === "Delete permanently")!);
    await waitFor(() => expect(group.textContent).toContain("so nothing was deleted"));
    expect(screen.getAllByText("Shop router").length).toBeGreaterThan(0);
  });
  it("clicks through Delete, confirm and permanent delete, with the banner above the button, and drops the card only on a real deletion", async () => {
    deleteAnswer = { status: 200, body: { message: "kept", record_kept: true } };
    routers = [{ ...CONNECTED, management_connected: false }];
    open("DEVELOPER");
    await screen.findByText("Shop router");
    // 1. the card's Delete record button opens the confirmation
    fireEvent.click(screen.getByRole("button", { name: "Delete record" }));
    const dialog = await screen.findByRole("alertdialog");
    // 2. the red Delete record button sends a plain DELETE and answers "kept"
    fireEvent.click([...dialog.querySelectorAll("button")].find((b) => b.textContent === "Delete record")!);
    await screen.findByRole("status", { name: "Record not deleted. Connection ended." });
    expect(deleted).toEqual([`/routers/${CONNECTED.device_id}`]);
    // 3. the permanent delete asks for the exact name, then sends force=true
    const group = screen.getByRole("group", { name: "Delete this record and its history for good" });
    const button = [...group.querySelectorAll("button")].find((b) => b.textContent === "Delete permanently") as HTMLButtonElement;
    fireEvent.change(group.querySelector("input")!, { target: { value: "Shop router" } });
    // An API that is not updated yet ignores force=true and answers "kept" again: say so, keep the card.
    fireEvent.click(button);
    const banner = await within(group).findByRole("alert");
    expect(banner.textContent).toContain("did not apply the permanent delete");
    expect(banner.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getAllByText("Shop router").length).toBeGreaterThan(0);
    expect(button.disabled).toBe(false);
    // The updated API deletes it: the card goes at once.
    deleteAnswer = { status: 200, body: { message: "router deleted", forced: true } };
    routers = [];
    fireEvent.click(button);
    await waitFor(() => expect(screen.queryByText("Shop router")).toBeNull());
    expect(deleted.filter((p) => p.includes("force=true"))).toHaveLength(2);
  });
  it("shows the refusal above the red Delete record button", async () => {
    deleteAnswer = { status: 409, body: { error: "Recover or cancel this router's management migration before deleting its record" } };
    open("OWNER");
    await startDelete();
    const dialog = await screen.findByRole("alertdialog");
    const banner = await within(dialog).findByRole("alert");
    const red = [...dialog.querySelectorAll("button")].find((b) => b.textContent === "Delete record")!;
    expect(banner.compareDocumentPosition(red) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
  it("says so when the session ended instead of doing nothing", async () => {
    deleteAnswer = { status: 401, body: { error: "unauthorized" } };
    open("DEVELOPER");
    await startDelete();
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() => expect(dialog.textContent).toContain("Your session ended"));
  });
  it("keeps the cleanup script when the connection ended but the record did not change", async () => {
    deleteAnswer = { status: 503, body: { error: "Press Delete again to finish.", code: "hub_ended_record_unchanged", router_cleanup_script: "# remove the setup" } };
    open();
    await startDelete();
    await screen.findByText("Connection ended. Record not updated.");
    await screen.findByDisplayValue("# remove the setup");
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await screen.findByText("Press Delete again to finish.");
    screen.getByRole("button", { name: "Delete record" });
  });
  it("shows why a delete was refused and keeps the card", async () => {
    deleteAnswer = { status: 503, body: { error: "Magic Hub could not end this router's connection, so nothing was deleted. Try again." } };
    open();
    await startDelete();
    await screen.findByText(/nothing was deleted/);
    expect(screen.getAllByText("Shop router").length).toBeGreaterThan(0);
  });
});
