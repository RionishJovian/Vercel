import { describe, expect, it } from "vitest";
import { discoveryEvidenceHealth } from "./discoveryEvidence";

const now = Date.parse("2026-11-08T12:00:00Z");

describe("read-only discovery evidence quality", () => {
  it("does not confuse an unreadable gateway with a network containing no devices", () => {
    const got = discoveryEvidenceHealth({
      sources: [{ name: "arp", available: false, count: 0 }, { name: "neighbors", available: false, count: 0 }],
      observed_at: "2026-11-08T11:59:00Z",
    }, now);
    expect(got).toEqual({ readable: 0, total: 2, noneReadable: true, freshness: "fresh" });
  });
  it("marks old observations stale even when sources were readable", () => {
    const got = discoveryEvidenceHealth({
      sources: [{ name: "arp", available: true, count: 10 }, { name: "neighbors", available: false, count: 0 }],
      observed_at: "2026-11-08T11:50:00Z",
    }, now);
    expect(got).toEqual({ readable: 1, total: 2, noneReadable: false, freshness: "stale" });
  });
  it("does not invent freshness from missing, malformed or far-future timestamps", () => {
    for (const observed_at of [undefined, "not-a-timestamp", "2026-11-09T12:00:00Z"]) {
      expect(discoveryEvidenceHealth({ sources: [{ name: "arp", available: true, count: 0 }], observed_at }, now).freshness).toBe("unknown");
    }
  });
  it("accepts a slightly skewed clock but never infers device online state", () => {
    const got = discoveryEvidenceHealth({
      sources: [{ name: "arp", available: true, count: 0 }],
      observed_at: "2026-11-08T12:00:30Z",
    }, now);
    expect(got.freshness).toBe("fresh");
    expect(got.noneReadable).toBe(false);
  });
});
