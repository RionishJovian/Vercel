import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { PageBoundary } from "../components/PageBoundary";
import { isChunkLoadError, requestReload, startEditTracking } from "./staleBuild";

let stop: (() => void) | undefined;
afterEach(() => { stop?.(); stop = undefined; cleanup(); vi.restoreAllMocks(); });
const ChunkBroken = (): never => { throw new TypeError("Failed to fetch dynamically imported module: /app/assets/SiteMap-old.js"); };
const OrdinaryBroken = (): never => { throw new Error("invalid application state"); };

describe("safe chunk recovery", () => {
  it("recognizes module failures without relabeling ordinary application or API errors", () => {
    expect(isChunkLoadError(new TypeError("Importing a module script failed."))).toBe(true);
    expect(isChunkLoadError(new TypeError("error loading dynamically imported module"))).toBe(true);
    expect(isChunkLoadError(new Error("invalid application state"))).toBe(false);
    expect(isChunkLoadError(new TypeError("Failed to fetch"))).toBe(false);
  });
  it("does not reload without an explicit request", () => {
    stop = startEditTracking();
    const reload = vi.fn(), confirm = vi.fn();
    expect(reload).not.toHaveBeenCalled();
    expect(requestReload("warning", reload, confirm)).toBe(true);
    expect(reload).toHaveBeenCalledTimes(1);
    expect(confirm).not.toHaveBeenCalled();
  });
  it("keeps typed input when reload is cancelled, and reloads only after explicit confirmation", () => {
    stop = startEditTracking();
    render(<input aria-label="Site name" />);
    fireEvent.input(screen.getByLabelText("Site name"), { target: { value: "Unsaved site" } });
    const reload = vi.fn();
    expect(requestReload("warning", reload, () => false)).toBe(false);
    expect(reload).not.toHaveBeenCalled();
    expect((screen.getByLabelText("Site name") as HTMLInputElement).value).toBe("Unsaved site");
    expect(requestReload("warning", reload, () => true)).toBe(true);
    expect(reload).toHaveBeenCalledTimes(1);
  });
  it("contains a late map chunk failure without unmounting the adjacent form", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    stop = startEditTracking();
    const tree = (broken: boolean) => <I18nProvider><input aria-label="Site name" /><PageBoundary inline>{broken ? <ChunkBroken /> : <div>map placeholder</div>}</PageBoundary></I18nProvider>;
    const view = render(tree(false));
    fireEvent.input(screen.getByLabelText("Site name"), { target: { value: "Keep this draft" } });
    view.rerender(tree(true));
    expect(screen.getByRole("alert")).toBeTruthy();
    expect((screen.getByLabelText("Site name") as HTMLInputElement).value).toBe("Keep this draft");
    const reload = vi.fn();
    expect(requestReload("warning", reload, () => false)).toBe(false);
    expect(reload).not.toHaveBeenCalled();
  });
  it("passes non-chunk errors to the outer page boundary", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(<I18nProvider><PageBoundary><PageBoundary inline><OrdinaryBroken /></PageBoundary></PageBoundary></I18nProvider>);
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.queryByText(/Part of the app could not load/)).toBeNull();
  });
});
