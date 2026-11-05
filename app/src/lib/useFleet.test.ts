import { afterEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { useFleet } from "./useFleet";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("useFleet", () => {
  it("shows the server's status and asks nothing else", async () => {
    const asked: string[] = [];
    const peers = [{ device_id: "d1", state: "up", source: "hub_tunnel" }];
    vi.stubGlobal("fetch", vi.fn(async (url: string) => { asked.push(String(url).replace("/api/v1", "")); return new Response(JSON.stringify({ total_peers: 1, up: 1, overall_state: "up", peers }), { status: 200, headers: { "content-type": "application/json" } }); }));
    const wrapper = ({ children }: { children: ReactNode }) => createElement(SWRConfig, { value: { provider: () => new Map() } }, children);
    const { result } = renderHook(() => useFleet(), { wrapper });
    await waitFor(() => expect(result.current.data?.peers).toEqual(peers));
    expect(asked).toEqual(["/fleet/health"]);
  });
});
