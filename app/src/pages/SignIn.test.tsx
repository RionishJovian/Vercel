import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { ThemeProvider } from "../lib/theme";
import { SignIn } from "./SignIn";

let reply: () => Response | Promise<Response>;
let bodies: unknown[];
beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.stubGlobal("matchMedia", () => ({ matches: true, addEventListener: () => {}, removeEventListener: () => {} }));
  bodies = [];
  vi.stubGlobal("fetch", vi.fn(async (_url: string, init?: RequestInit) => { bodies.push(JSON.parse(String(init?.body))); return reply(); }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); history.replaceState({}, "", "/"); });

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const submit = (onDone = vi.fn()) => {
  const { container } = render(<ThemeProvider><I18nProvider><SignIn onDone={onDone} /></I18nProvider></ThemeProvider>);
  fireEvent.change(screen.getByLabelText("Email"), { target: { value: "  owner@example.com " } });
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "a-long-password" } });
  fireEvent.submit(container.querySelector("form")!);
  return onDone;
};

describe("Sign in", () => {
  it("sends the trimmed email and continues on success", async () => {
    reply = () => json(200, {});
    const done = submit();
    await waitFor(() => expect(done).toHaveBeenCalled());
    expect(bodies[0]).toEqual({ email: "owner@example.com", password: "a-long-password" });
  });
  it("says to check the details after a wrong password", async () => {
    reply = () => json(401, { error: "invalid credentials" });
    const done = submit();
    expect((await screen.findByRole("alert")).textContent).toContain("Check your email and password");
    expect(done).not.toHaveBeenCalled();
  });
  it("tells a locked-out user to wait, not to recheck their password", async () => {
    reply = () => json(429, { error: "too many failed sign-in attempts" });
    submit();
    const alert = (await screen.findByRole("alert")).textContent;
    expect(alert).toContain("Wait 15 minutes");
    expect(alert).not.toContain("Check your email");
  });
  it("reports an unreachable server separately and allows a retry", async () => {
    reply = () => Promise.reject(new TypeError("offline"));
    submit();
    expect((await screen.findByRole("alert")).textContent).toContain("Could not reach the server");
    expect((screen.getByRole("button", { name: "Sign in" }) as HTMLButtonElement).disabled).toBe(false);
  });
  it("accepts older logins without a domain suffix (no browser email validation)", async () => {
    reply = () => json(200, {});
    const onDone = vi.fn();
    const { container } = render(<ThemeProvider><I18nProvider><SignIn onDone={onDone} /></I18nProvider></ThemeProvider>);
    const email = screen.getByLabelText("Email") as HTMLInputElement;
    expect(email.type).toBe("text");
    expect(email.autocomplete).toBe("username");
    fireEvent.change(email, { target: { value: "kyaw@mikromagic" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "pw" } });
    fireEvent.submit(container.querySelector("form")!);
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(bodies[0]).toEqual({ email: "kyaw@mikromagic", password: "pw" });
  });
  it("explains a suspended business separately", async () => {
    reply = () => json(403, { error: "this business account is suspended" });
    submit();
    expect((await screen.findByRole("alert")).textContent).toContain("suspended");
  });
  it("can show the password without losing autofill hints", () => {
    render(<ThemeProvider><I18nProvider><SignIn onDone={vi.fn()} /></I18nProvider></ThemeProvider>);
    const pw = screen.getByLabelText("Password") as HTMLInputElement;
    expect(pw.autocomplete).toBe("current-password");
    fireEvent.click(screen.getByRole("button", { name: "Show password" }));
    expect(pw.type).toBe("text");
    fireEvent.click(screen.getByRole("button", { name: "Hide password" }));
    expect(pw.type).toBe("password");
  });
});


describe("sign-in credential navigation safety", () => {
  const open = () => render(<ThemeProvider><I18nProvider><SignIn onDone={vi.fn()} /></I18nProvider></ThemeProvider>);
  it("declares a POST fallback and only mounts the active sign-in inputs", () => {
    const { container } = open();
    expect(container.querySelector("form")?.getAttribute("method")).toBe("post");
    expect(container.querySelectorAll("form")).toHaveLength(1);
    expect(container.querySelectorAll('input[name="password"]')).toHaveLength(1);
    expect(container.querySelector('input[autocomplete="new-password"]')).toBeNull();
  });
  it.each(["?email=synthetic%40example.test&password=synthetic-only", "?%70assword=synthetic-only", "?EMAIL=synthetic%40example.test"]) ("strips credential query keys and keeps them out of the create-account return link: %s", (query) => {
    history.replaceState({}, "", "/app/revenue" + query + "#old");
    const { container } = open();
    expect(location.search).toBe("");
    expect(location.hash).toBe("");
    const link = container.querySelector('a[href^="/signin?"]') as HTMLAnchorElement;
    expect(new URL(link.href).searchParams.get("next")).toBe("/app/revenue");
  });
  it("preserves a non-credential return query", () => {
    history.replaceState({}, "", "/app/revenue?period=day");
    const { container } = open();
    expect(location.search).toBe("?period=day");
    const link = container.querySelector('a[href^="/signin?"]') as HTMLAnchorElement;
    expect(new URL(link.href).searchParams.get("next")).toBe("/app/revenue?period=day");
  });
});
