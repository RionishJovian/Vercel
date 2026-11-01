import { describe, expect, it } from "vitest";
import { mergeFleet, type FleetData } from "./useFleet";
import type { HubStatus } from "./hubStatus";

const recorded = (state: string): FleetData => ({ total_peers: 1, up: 0, stale: 0, offline: 0, unknown: 1, overall_state: state, peers: [{ device_id: "d1", hardware_name: "hAP ax3", state }] });

describe("fleet health follows the live Magic Hub handshake", () => {
  it("a connected router with no recorded heartbeat is up, so Home and the Router tab agree", () => {
    const m = mergeFleet(recorded("unknown"), { d1: { state: "up" } })!;
    expect(m.peers?.[0].state).toBe("up");
    expect([m.up, m.total_peers, m.overall_state, m.unknown]).toEqual([1, 1, "up", 0]);
  });
  it("a tunnel with no recent handshake is offline", () => {
    const m = mergeFleet(recorded("unknown"), { d1: { state: "down" } })!;
    expect([m.peers?.[0].state, m.offline, m.overall_state]).toEqual(["offline", 1, "offline"]);
  });
  it("when the lookup cannot answer, the recorded state stands unchanged", () => {
    for (const hub of [{} as Record<string, HubStatus>, { d1: { state: "unknown" } } as Record<string, HubStatus>]) {
      const m = mergeFleet(recorded("stale"), hub)!;
      expect(m.peers?.[0].state).toBe("stale");
      expect(m.overall_state).toBe("stale");
    }
    expect(mergeFleet(recorded("unknown"), {})!.overall_state).toBe("unknown");
  });
  it("counts several routers, and the worst state is the overall one", () => {
    const fleet: FleetData = { total_peers: 3, peers: [{ device_id: "a", state: "unknown" }, { device_id: "b", state: "up" }, { device_id: "c", state: "unknown" }] };
    const m = mergeFleet(fleet, { a: { state: "up" }, c: { state: "down" } })!;
    expect([m.up, m.offline, m.unknown, m.overall_state]).toEqual([2, 1, 0, "offline"]);
  });
  it("passes an absent fleet through", () => { expect(mergeFleet(undefined, {})).toBeUndefined(); });
});
