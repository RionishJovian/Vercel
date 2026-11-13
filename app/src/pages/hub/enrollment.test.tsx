import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { Enrollment } from "./Enrollment";

const ID = "6f69dede-53d9-478a-bc68-9911e878d440";
const SETUP = "{\n:put \"check\"\n# APPLY: tunnel\n:put \"probe\"\n}\n";
const PASTE = "\n" + SETUP;
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let liveAnswer: { status: number; body: unknown }, enrollAnswer: { status: number; body: unknown };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const ENROLLED = { state: "hub_peer_enrolled", router_configuration_required: true, one_paste: true, automatic_management: true, router_script: SETUP, tunnel_address: "10.77.0.20/32", hub_public_key: "HUBKEY", endpoint: "megamagic.tech:51820", allowed_ips: ["10.77.0.1/32"] };

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); calls = [];
  liveAnswer = { status: 200, body: { handshake_status: "no_recent_handshake", connection: { management: "missing" } } };
  enrollAnswer = { status: 200, body: ENROLLED };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path.endsWith("/quick-enroll")) return json(enrollAnswer.status, enrollAnswer.body);
    if (path.endsWith("/live")) return json(liveAnswer.status, liveAnswer.body);
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); });
const open = (onVerified = vi.fn()) => { render(<I18nProvider><Enrollment id={ID} onVerified={onVerified} /></I18nProvider>); return onVerified; };
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole("button", { name }));
const generate = async () => { fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script"); await screen.findByRole("textbox", { name: "Connection script" }); };
const rail = () => [...screen.getByRole("list", { name: "Connection steps" }).querySelectorAll("li")].map((li) => li.getAttribute("data-state"));

describe("one-paste connection", () => {
  it("asks for no port and sends one enrollment request", async () => {
    open();
    expect(screen.queryByRole("spinbutton")).toBeNull();
    await generate();
    expect(calls[0]).toEqual({ method: "POST", url: `/vps/peers/${ID}/quick-enroll`, body: { management_rest_port: 443, confirm_enrollment: true } });
    expect(await screen.findByRole("heading", { name: "2. Paste the connection script" })).toBeTruthy();
    expect(rail()).toEqual(["done", "current", "todo"]);
  });
  it("offers no Check button or repair path after the script is shown", async () => {
    open(); await generate();
    for (const name of [/Check/, /I pasted/, /Repair/, /Back to script/]) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.getByText(/connects by itself/)).toBeTruthy();
  });
  it("keeps a blank line before the block in clipboard and manual copies", async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("Clipboard unavailable"));
    vi.stubGlobal("navigator", Object.assign(Object.create(navigator), { clipboard: { writeText } }));
    open(); await generate();
    const script = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Connection script" });
    click("Copy");
    expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
    expect(writeText).toHaveBeenCalledWith(PASTE);
    click("Copied");
    await screen.findByRole("button", { name: "Selected: copy it manually" });
    expect(script.value.slice(script.selectionStart, script.selectionEnd)).toBe(PASTE);
  });
  it.each(["ethernet", "wifi1", "wifi2"])("previews and includes %s setup without posting Wi-Fi details", async uplink => {
    open();
    fireEvent.change(screen.getByLabelText("Router network setup"), { target: { value: uplink } });
    expect(screen.getByText(/Requires factory defaults/)).toBeTruthy();
    if (uplink !== "ethernet") {
      fireEvent.change(screen.getByLabelText(/Upstream Wi-Fi name/), { target: { value: 'Fixture "$&"' } });
      fireEvent.change(screen.getByLabelText("Upstream Wi-Fi password"), { target: { value: "test-password" } });
    }
    await generate();
    const { value } = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Connection script" });
    expect(value).toMatch(/^\n\{\n/);
    expect(value).toContain(`:local mmUplink "${uplink === "ethernet" ? "ether1" : uplink}"`);
    // Network writes come after every Magic Hub check and before the first Magic Hub write.
    expect(value.indexOf("add-default-route=yes")).toBeLessThan(value.indexOf(':put "probe"'));
    expect(value.indexOf(':put "check"')).toBeLessThan(value.indexOf("/ip dhcp-client set"));
    expect(value.includes("security.passphrase=")).toBe(uplink !== "ethernet");
    expect(calls[0].body).toEqual({ management_rest_port: 443, confirm_enrollment: true });
  });
  it("needs authorization before generating", () => {
    open();
    fireEvent.submit(screen.getByRole("button", { name: "Generate one-paste script" }).closest("form")!);
    expect(calls).toHaveLength(0);
  });
  it("finishes by itself once the server has connected the router", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const onVerified = open(); await generate();
    await screen.findByText("Waiting for your router…");
    liveAnswer = { status: 200, body: { handshake_status: "recent_handshake", connection: { management: "saved" } } };
    await vi.advanceTimersByTimeAsync(5100);
    await waitFor(() => expect(onVerified).toHaveBeenCalledTimes(1));
    expect(screen.getByText(/Connected\. Router management is ready/)).toBeTruthy();
    expect(rail()).toEqual(["done", "done", "current"]);
  });
  it("shows that the router reached the Hub, without claiming it is connected", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const onVerified = open(); await generate();
    liveAnswer = { status: 200, body: { handshake_status: "recent_handshake", connection: { management: "missing" } } };
    await vi.advanceTimersByTimeAsync(5100);
    await screen.findByText(/reached Magic Hub/);
    expect(onVerified).not.toHaveBeenCalled();
  });
  it("offers help only after waiting a while", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    open(); await generate();
    expect(screen.queryByRole("group", { name: /help|Nothing|check/i })).toBeNull();
    await vi.advanceTimersByTimeAsync(125000);
    await screen.findByText(/The complete connection script finished/);
  });
  it("keeps waiting through a failed status read", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const onVerified = open(); await generate();
    liveAnswer = { status: 502, body: {} };
    await vi.advanceTimersByTimeAsync(5100);
    liveAnswer = { status: 200, body: { connection: { management: "saved" } } };
    await vi.advanceTimersByTimeAsync(5100);
    await waitFor(() => expect(onVerified).toHaveBeenCalledTimes(1));
  });
  it("stops watching when closed", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    open(); await generate();
    cleanup(); const before = calls.length;
    await vi.advanceTimersByTimeAsync(20000);
    expect(calls.length).toBe(before);
  });
  it("explains a router that is already connected", async () => {
    enrollAnswer = { status: 409, body: { code: "hub_management_already_configured" } };
    open(); fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script");
    await screen.findByText(/Saved management credentials exist/);
    expect(screen.queryByRole("textbox")).toBeNull();
  });
  it("rejects a response that is not a one-paste script", async () => {
    enrollAnswer = { status: 200, body: { ...ENROLLED, one_paste: false } };
    open(); fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script");
    await screen.findByText(/One-paste setup was not confirmed/); expect(screen.queryByRole("textbox", { name: "Connection script" })).toBeNull();
  });
  it("locks generation while a request is outstanding", async () => {
    const normalFetch = globalThis.fetch; let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => { await held; return normalFetch(url, init); }));
    open(); fireEvent.click(screen.getByRole("checkbox"));
    const form = screen.getByRole("button", { name: "Generate one-paste script" }).closest("form")!;
    fireEvent.submit(form); fireEvent.submit(form);
    release(); await screen.findByRole("textbox", { name: "Connection script" }); expect(calls.filter((c) => c.url.endsWith("/quick-enroll"))).toHaveLength(1);
  });
});
