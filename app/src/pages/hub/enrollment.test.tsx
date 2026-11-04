import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { Enrollment } from "./Enrollment";

const ID = "6f69dede-53d9-478a-bc68-9911e878d440";
const SETUP = "# complete one-paste setup script";
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let finishAnswer: { status: number; body: unknown }, enrollAnswer: { status: number; body: unknown };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); calls = [];
  finishAnswer = { status: 409, body: { code: "hub_handshake_unverified" } };
  enrollAnswer = { status: 200, body: { state: "hub_peer_enrolled", router_configuration_required: true, one_paste: true, automatic_management: true, router_script: SETUP, tunnel_address: "10.77.0.16/32", hub_public_key: "HUBKEY", endpoint: "207.148.76.172:51820", allowed_ips: ["10.77.0.1/32"] } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET", path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path.endsWith("/quick-enroll")) return json(enrollAnswer.status, enrollAnswer.body);
    if (path.endsWith("/finish")) return json(finishAnswer.status, finishAnswer.body);
    // A one-paste setup must never create a separate router-generated key first.
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = (onVerified = vi.fn()) => { render(<I18nProvider><Enrollment id={ID} onVerified={onVerified} /></I18nProvider>); return onVerified; };
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole("button", { name }));
const generate = async () => { fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script"); await screen.findByDisplayValue(SETUP); };
const toCheck = async () => { await generate(); click("I pasted it and saw no error"); await screen.findByRole("heading", { name: "3. Check the connection" }); };

describe("Magic Hub one-paste connection", () => {
  it("creates one complete script after consent without requesting bootstrap", async () => {
    open(); expect(calls).toEqual([]);
    fireEvent.submit(screen.getByRole("button", { name: "Generate one-paste script" }).closest("form")!);
    await screen.findByText("Confirm that you authorize connecting this RouterBOARD."); expect(calls).toEqual([]);
    fireEvent.change(screen.getByRole("spinbutton"), { target: { value: "8443" } });
    await generate();
    expect(calls).toEqual([{ method: "POST", url: `/vps/peers/${ID}/quick-enroll`, body: { management_rest_port: 8443, confirm_enrollment: true } }]);
    expect(await screen.findByRole("heading", { name: "2. Paste the connection script" })).toBeTruthy();
  });
  it("can check an existing connection immediately without generating a script", async () => {
    finishAnswer = { status: 200, body: { state: "management_authenticated" } };
    const onVerified = open(); click("Check existing connection"); click("Check connection");
    await waitFor(() => expect(onVerified).toHaveBeenCalledTimes(1));
    expect(calls).toEqual([{ method: "POST", url: `/vps/peers/${ID}/finish`, body: {} }]);
    const rail = screen.getByRole("list", { name: "Connection steps" }).querySelectorAll("li");
    expect(rail[0].getAttribute("data-state")).toBe("todo"); expect(rail[1].getAttribute("data-state")).toBe("todo");
  });
  it("routes a saved-management conflict directly to verification", async () => {
    enrollAnswer = { status: 409, body: { code: "hub_management_already_configured" } };
    open(); fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script");
    await screen.findByRole("heading", { name: "3. Check the connection" }); expect(screen.queryByRole("textbox")).toBeNull();
    click("Check connection"); await screen.findByText(/No recent tunnel connection is verified/);
    expect(calls.map((c) => c.url)).toEqual([`/vps/peers/${ID}/quick-enroll`, `/vps/peers/${ID}/finish`]);
  });
  it("keeps a failed check honest and retains the same generated script", async () => {
    open(); await toCheck(); click("Check connection"); await screen.findByText(/No recent tunnel connection is verified/);
    click("Back to script"); expect(await screen.findByDisplayValue(SETUP)).toBeTruthy();
    expect(calls.filter((c) => c.url.endsWith("/quick-enroll"))).toHaveLength(1);
  });
  it("reports success only after verified management", async () => {
    finishAnswer = { status: 200, body: { state: "management_authenticated" } };
    const onVerified = open(); await toCheck(); click("Check connection");
    await waitFor(() => expect(onVerified).toHaveBeenCalledTimes(1));
    expect(screen.getByText(/Connected\. Router management is ready/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Check connection" }).matches(":disabled")).toBe(true);
  });
  it("does not accept an unverified successful HTTP response", async () => {
    finishAnswer = { status: 200, body: { state: "something_else" } };
    const onVerified = open(); click("Check existing connection"); click("Check connection");
    await screen.findByText(/not verified yet/); expect(onVerified).not.toHaveBeenCalled();
  });
  it("requires explicit recovery consent and preserves the existing-tunnel contract", async () => {
    enrollAnswer = { status: 409, body: { code: "hub_router_key_unavailable" } };
    open(); fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script");
    fireEvent.click(await screen.findByRole("checkbox", { name: /Repair installed connection/ }));
    enrollAnswer = { status: 200, body: { state: "hub_peer_enrolled", router_configuration_required: true, one_paste: false, existing_tunnel_required: true, automatic_management: true, router_script: SETUP } };
    click("Generate one-paste script"); await screen.findByDisplayValue(SETUP);
    expect(calls[1].body).toEqual({ management_rest_port: 443, confirm_enrollment: true, resume_existing: true });
    expect(screen.getByText(/needs the original Magic Hub tunnel/)).toBeTruthy(); expect(calls.some((c) => c.url.endsWith("/bootstrap"))).toBe(false);
  });
  it("rejects a repair response that was not explicitly requested", async () => {
    enrollAnswer.body = { state: "hub_peer_enrolled", router_configuration_required: true, one_paste: false, existing_tunnel_required: true, automatic_management: true, router_script: SETUP };
    open(); fireEvent.click(screen.getByRole("checkbox")); click("Generate one-paste script");
    await screen.findByText(/One-paste setup was not confirmed/); expect(screen.queryByDisplayValue(SETUP)).toBeNull();
  });
  it("locks generation and navigation while an enrollment request is outstanding", async () => {
    const normalFetch = globalThis.fetch; let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => { await held; return normalFetch(url, init); }));
    open(); fireEvent.click(screen.getByRole("checkbox"));
    const form = screen.getByRole("button", { name: "Generate one-paste script" }).closest("form")!;
    fireEvent.submit(form); fireEvent.submit(form);
    expect(screen.getByRole("button", { name: "Check existing connection" }).matches(":disabled")).toBe(true);
    expect(screen.getByRole("spinbutton").matches(":disabled")).toBe(true);
    release(); await screen.findByDisplayValue(SETUP); expect(calls).toHaveLength(1);
  });
});
