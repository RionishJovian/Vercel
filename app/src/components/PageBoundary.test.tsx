import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { PageBoundary } from "./PageBoundary";

const Broken = (): never => { throw new TypeError("Cannot read properties of undefined (reading 'pending_confirm')"); };
const page = (key: string, child: React.ReactNode) => <I18nProvider><nav>tabs</nav><PageBoundary key={key}>{child}</PageBoundary></I18nProvider>;

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: (k: string) => { delete store[k]; } });
  vi.spyOn(console, "error").mockImplementation(() => {}); // React reports the caught error here.
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("PageBoundary", () => {
  it("shows a message inside the page and leaves the tabs in place", () => {
    render(page("a", <Broken />));
    expect(screen.getByRole("alert").textContent).toContain("This page could not be shown");
    expect(screen.getByRole("button", { name: "Reload" })).toBeTruthy();
    expect(screen.getByText("tabs")).toBeTruthy();
    expect(document.body.textContent).not.toContain("pending_confirm");
  });
  it("recovers when another page opens", () => {
    const view = render(page("a", <Broken />));
    view.rerender(page("b", <p>working page</p>));
    expect(screen.getByText("working page")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
