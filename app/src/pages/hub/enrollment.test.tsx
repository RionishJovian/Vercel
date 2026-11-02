import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { Enrollment } from "./Enrollment";

const ID = "6f69dede-53d9-478a-bc68-9911e878d440";
const BOOT = "# step 1 bootstrap script";
const SETUP = "# step 2 setup script";
let calls: Array<{ method: string; url: string; body?: Record<string, unknown> }>;
let finishAnswer: { status: number; body: unknown };

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  calls = [];
  finishAnswer = { status: 409, body: { code: "hub_handshake_unverified" } };
  vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    const path = String(url).replace("/api/v1", "");
    calls.push({ method, url: path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (path.endsWith("/bootstrap")) return json(200, { state: "router_key_required", bootstrap_script: BOOT });
    if (path.endsWith("/quick-enroll")) return json(200, { state: "hub_peer_enrolled", router_configuration_required: true, one_paste: true, automatic_management: true, router_script: SETUP, tunnel_address: "10.77.0.16/32", hub_public_key: "HUBKEY", endpoint: "207.148.76.172:51820", allowed_ips: ["10.77.0.1/32"] });
    if (path.endsWith("/finish")) return json(finishAnswer.status, finishAnswer.body);
    return json(404, {});
  }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const open = (onVerified = vi.fn()) => { render(<I18nProvider><Enrollment id={ID} onVerified={onVerified} /></I18nProvider>); return onVerified; };
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole("button", { name }));
const toStep2 = async () => {
  click("Create the step 1 script");
  await screen.findByDisplayValue(BOOT);
  click("I pasted it and saw no error");
  await screen.findByRole("heading", { name: "2. Connect the router" });
};
const toStep3 = async () => {
  await toStep2();
  fireEvent.click(screen.getByRole("checkbox"));
  click("Create the step 2 script");
  await screen.findByDisplayValue(SETUP);
  click("I pasted it and saw no error");
  await screen.findByRole("heading", { name: "3. Check the connection" });
};

describe("Magic Hub connection steps", () => {
  it("starts at step 1 with only that step open and later steps locked", () => {
    open();
    expect(screen.getByRole("heading", { name: "1. Prepare the router" })).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.queryByRole("button", { name: "Check connection" })).toBeNull();
    const rail = screen.getByRole("list", { name: "Connection steps" }).querySelectorAll("button");
    expect([...rail].map((b) => b.disabled)).toEqual([true, true, true]);
    expect(rail[0].getAttribute("aria-current")).toBe("step");
  });

  it("does not offer step 2 until the step 1 script exists and is confirmed pasted", async () => {
    open();
    expect(screen.queryByRole("button", { name: "I pasted it and saw no error" })).toBeNull();
    click("Create the step 1 script");
    await screen.findByDisplayValue(BOOT);
    expect(calls.filter((c) => c.url.endsWith("/quick-enroll"))).toHaveLength(0);
    expect(screen.queryByRole("checkbox")).toBeNull();
    click("I pasted it and saw no error");
    expect(await screen.findByRole("checkbox")).toBeTruthy();
  });

  it("creates the second script only after consent, with the chosen port", async () => {
    open();
    await toStep2();
    fireEvent.submit(screen.getByRole("button", { name: "Create the step 2 script" }).closest("form")!);
    expect(await screen.findByText("Confirm that you authorize connecting this RouterBOARD.")).toBeTruthy();
    expect(calls.some((c) => c.url.endsWith("/quick-enroll"))).toBe(false);
    fireEvent.click(screen.getByRole("checkbox"));
    click("Create the step 2 script");
    await screen.findByDisplayValue(SETUP);
    const post = calls.find((c) => c.url.endsWith("/quick-enroll"))!;
    expect(post.method).toBe("POST");
    expect(post.body).toEqual({ management_rest_port: 443, confirm_enrollment: true });
    expect(screen.getByText(/Paste this script once/)).toBeTruthy();
  });

  it("explains a failed check, keeps the scripts, and offers a way back or a restart", async () => {
    open();
    await toStep3();
    click("Check connection");
    expect(await screen.findByText(/No recent tunnel connection is verified/)).toBeTruthy();
    expect(screen.getByText("The step 2 script finished with no red text and no STOP: line.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Check again" })).toBeTruthy();
    click("Back to step 2");
    expect(await screen.findByDisplayValue(SETUP)).toBeTruthy();
  });

  it("start again clears both scripts and returns to step 1", async () => {
    open();
    await toStep3();
    click("Check connection");
    await screen.findByText(/No recent tunnel connection is verified/);
    click("Start again from step 1");
    expect(await screen.findByRole("heading", { name: "1. Prepare the router" })).toBeTruthy();
    expect(screen.queryByDisplayValue(BOOT)).toBeNull();
    expect(screen.queryByDisplayValue(SETUP)).toBeNull();
  });

  it("reports success only when the server verifies management", async () => {
    finishAnswer = { status: 200, body: { state: "management_authenticated" } };
    const onVerified = open();
    await toStep3();
    click("Check connection");
    await waitFor(() => expect(onVerified).toHaveBeenCalledTimes(1));
    expect(screen.getByText(/Connected\. Router management is ready/)).toBeTruthy();
  });

  it("does not report success for an unverified answer", async () => {
    finishAnswer = { status: 200, body: { state: "something_else" } };
    const onVerified = open();
    await toStep3();
    click("Check connection");
    await screen.findByText(/not verified yet/);
    expect(onVerified).not.toHaveBeenCalled();
  });
});
