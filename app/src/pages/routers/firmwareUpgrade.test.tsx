import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { FirmwareUpgrade } from "./FirmwareUpgrade";

const ID = "7af55780-3e68-40cc-b4ae-e464b18c8961";
const REVIEW = { confirmation: "tok", current: "7.6", target: "7.24.5", model: "hAP ax3", steps: ["Save a backup", "Write the firmware", "Restart"], warnings: ["The router restarts.", "This cannot be undone from the app."] };
const FW = { current_firmware: "7.6", upgrade_firmware: "7.24.5", firmware_upgrade_available: true };
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let mode: string | null, telemetryFirmware: string, applyStatus: number;

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const flush = async (ms = 0) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); };

beforeEach(() => {
  vi.useFakeTimers();
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  calls = []; mode = "true"; telemetryFirmware = "7.6"; applyStatus = 200;
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path.endsWith("/firmware/status")) return mode === null ? json(403, { error: "forbidden" }) : json(200, { mode });
    if (path.endsWith("/firmware/preview")) return json(200, { review: REVIEW });
    if (path.endsWith("/firmware/apply")) return applyStatus === 200 ? json(200, { result: { restarting: true } }) : json(applyStatus, { error: "The router refused the firmware upgrade; it was not restarted" });
    if (path.endsWith("/telemetry")) return json(200, { telemetry: { snapshot: { firmware: { current_firmware: telemetryFirmware } } } });
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); });

const open = async (fw: typeof FW | null = FW, onDone = vi.fn()) => {
  render(<I18nProvider><FirmwareUpgrade id={ID} fw={fw ?? undefined} onDone={onDone} /></I18nProvider>);
  await flush();
  return onDone;
};
const click = async (name: string | RegExp) => { fireEvent.click(screen.getByRole("button", { name })); await flush(); };
const applyCalls = () => calls.filter((c) => c.url.endsWith("/firmware/apply"));

describe("Firmware upgrade", () => {
  it("shows nothing to someone the server does not allow, or when no upgrade is available", async () => {
    mode = null;
    await open();
    expect(screen.queryByRole("heading", { name: "Firmware upgrade" })).toBeNull();
    cleanup(); mode = "true";
    await open({ ...FW, firmware_upgrade_available: false });
    expect(screen.queryByRole("heading", { name: "Firmware upgrade" })).toBeNull();
    cleanup();
    await open(null);
    expect(screen.queryByRole("heading", { name: "Firmware upgrade" })).toBeNull();
  });

  it("says upgrades are off and offers no button while the server holds them", async () => {
    mode = "held";
    await open();
    expect(screen.getByText("Firmware upgrades are switched off for now.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Upgrade firmware" })).toBeNull();
  });

  it("preview-only mode shows the review but never offers to apply", async () => {
    mode = "preview";
    await open();
    await click("Upgrade firmware");
    expect(screen.getByText("7.6 → 7.24.5")).toBeTruthy();
    expect(screen.getByText("This cannot be undone from the app.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Upgrade and restart" })).toBeNull();
    expect(applyCalls()).toHaveLength(0);
  });

  it("needs the consent checkbox before anything is applied, and sends the reviewed confirmation", async () => {
    await open();
    await click("Upgrade firmware");
    const go = screen.getByRole("button", { name: "Upgrade and restart" }) as HTMLButtonElement;
    expect(go.disabled).toBe(true);
    fireEvent.click(go);
    await flush();
    expect(applyCalls()).toHaveLength(0);
    fireEvent.click(screen.getByRole("checkbox"));
    await click("Upgrade and restart");
    expect(applyCalls()).toHaveLength(1);
    expect(applyCalls()[0].body).toEqual({ confirmation: "tok" });
    expect(screen.getByText(/Restarting\. The router is offline/)).toBeTruthy();
  });

  it("reports success only when the router itself reports the new firmware", async () => {
    const onDone = await open();
    await click("Upgrade firmware");
    fireEvent.click(screen.getByRole("checkbox"));
    await click("Upgrade and restart");
    await flush(5000);
    expect(screen.queryByText(/Done\./)).toBeNull();
    expect(onDone).not.toHaveBeenCalled();
    telemetryFirmware = "7.24.5";
    await flush(5000);
    expect(screen.getByText("Done. The router reports firmware 7.24.5")).toBeTruthy();
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it("gives up after ten minutes and says so instead of claiming success", async () => {
    const onDone = await open();
    await click("Upgrade firmware");
    fireEvent.click(screen.getByRole("checkbox"));
    await click("Upgrade and restart");
    await flush(10 * 60 * 1000 + 6000);
    expect(screen.getByText(/has not reported the new firmware after ten minutes/)).toBeTruthy();
    expect(screen.queryByText(/Done\./)).toBeNull();
    expect(onDone).not.toHaveBeenCalled();
  });

  it("shows the server's reason when the upgrade is refused and does not pretend it restarted", async () => {
    applyStatus = 502;
    await open();
    await click("Upgrade firmware");
    fireEvent.click(screen.getByRole("checkbox"));
    await click("Upgrade and restart");
    expect(screen.getByText("The router refused the firmware upgrade; it was not restarted")).toBeTruthy();
    expect(screen.queryByText(/Restarting\./)).toBeNull();
  });
});
