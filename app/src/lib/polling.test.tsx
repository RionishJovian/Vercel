import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import useSWR, { SWRConfig } from "swr";
import { ApiError } from "./api";
import { keepPolling, useVisibleInterval } from "./polling";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); });
beforeEach(() => { vi.stubGlobal("document", Object.assign(document, {})); });

function Reading({ read }: { read: () => Promise<string> }) {
  const { data, error } = useSWR("reading", read, keepPolling(20));
  return <p>{error ? "failed" : data ?? "loading"}</p>;
}
const open = (read: () => Promise<string>) => render(<SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0, shouldRetryOnError: false }}><Reading read={read} /></SWRConfig>);

describe("keepPolling", () => {
  it("recovers by itself after a failed read, even with retries off everywhere else", async () => {
    let calls = 0;
    open(async () => { calls++; if (calls === 1) throw new Error("router busy"); return "reading " + calls; });
    await screen.findByText("failed");
    await waitFor(() => expect(screen.getByText(/^reading/)).toBeTruthy());
    const seen = calls;
    await waitFor(() => expect(calls).toBeGreaterThan(seen));
  });
  it("stops when the session has ended", async () => {
    let calls = 0;
    open(async () => { calls++; throw new ApiError("signed out", 401); });
    await screen.findByText("failed");
    await new Promise((r) => setTimeout(r, 90));
    expect(calls).toBe(1);
  });
});

describe("useVisibleInterval", () => {
  const hidden = (value: boolean) => Object.defineProperty(document, "visibilityState", { configurable: true, get: () => (value ? "hidden" : "visible") });
  afterEach(() => hidden(false));
  it("runs on a timer only while visible, and at once on coming back", () => {
    vi.useFakeTimers();
    const run = vi.fn();
    renderHook(() => useVisibleInterval(run, 1000));
    act(() => { vi.advanceTimersByTime(2100); });
    expect(run).toHaveBeenCalledTimes(2);
    hidden(true);
    act(() => { vi.advanceTimersByTime(3000); });
    expect(run).toHaveBeenCalledTimes(2);
    hidden(false);
    act(() => { document.dispatchEvent(new Event("visibilitychange")); });
    expect(run).toHaveBeenCalledTimes(3);
  });
  it("does nothing when turned off", () => {
    vi.useFakeTimers();
    const run = vi.fn();
    renderHook(() => useVisibleInterval(run, 1000, false));
    act(() => { vi.advanceTimersByTime(3000); });
    expect(run).not.toHaveBeenCalled();
  });
});
