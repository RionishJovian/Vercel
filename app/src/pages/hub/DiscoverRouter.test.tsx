import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { DiscoverRouter } from "./DiscoverRouter";

let fetchSpy: ReturnType<typeof vi.fn>;
beforeEach(() => { localStorage.setItem("mm2_lang", "en"); fetchSpy = vi.fn(); vi.stubGlobal("fetch", fetchSpy); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const open = (onContinue = vi.fn()) => { render(<I18nProvider><DiscoverRouter onContinue={onContinue} /></I18nProvider>); return onContinue; };
const expand = () => { fireEvent.click(screen.getByRole("button", { name: "Show" })); fireEvent.click(screen.getByRole("checkbox")); };

describe("DiscoverRouter", () => {
  it("starts closed and asks for nothing", () => {
    open();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.getByRole("button", { name: "Show" }).getAttribute("aria-expanded")).toBe("false");
  });
  it("suggests the default address as not verified and never claims a router is there", () => {
    open(); expand();
    screen.getByText(/Suggested address/); screen.getByText(/Not verified/);
    expect(screen.getByRole("status").textContent).not.toMatch(/Connected|Online|Managed|Found/i);
    expect(screen.getByRole("status").textContent).toMatch(/Not verified/);
    expect(screen.getByRole("link", { name: "Open the router page" }).getAttribute("href")).toBe("http://192.168.88.1");
  });
  it("opens the router page in a new tab without leaking the opener", () => {
    open(); expand();
    const a = screen.getByRole("link", { name: "Open the router page" });
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("rel")).toContain("noopener");
  });
  it("labels a typed address as entered by the owner, and refuses a public one", () => {
    open(); expand();
    const input = screen.getByLabelText("Router address");
    fireEvent.change(input, { target: { value: "192.168.1.1" } });
    screen.getByText(/Entered by you/);
    fireEvent.change(input, { target: { value: "8.8.8.8" } });
    expect(screen.queryByRole("link", { name: "Open the router page" })).toBeNull();
    screen.getByText(/Public addresses are not accepted/);
  });
  it("makes no network request at all", () => {
    open(); expand();
    fireEvent.change(screen.getByLabelText("Router address"), { target: { value: "192.168.1.1" } });
    expect(fetchSpy).not.toHaveBeenCalled();
  });
  it("continues into the existing add-router form and says automatic scanning is not available", () => {
    const onContinue = open(); expand();
    fireEvent.click(screen.getByRole("button", { name: "Continue to Magic Hub" }));
    expect(onContinue).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText("Cannot open it?"));
    screen.getByText(/not available yet/);
  });
});
