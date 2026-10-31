import { connectionIsFresh } from "./Sites";

const now = Date.parse("2026-10-31T12:00:00Z");
const at = (ms: number) => new Date(now + ms).toISOString();

describe("connectionIsFresh", () => {
  it("accepts an online report from the last 5 minutes", () => { expect(connectionIsFresh({ cloud_status: "online", cloud_last_seen_at: at(-60_000) }, now)).toBe(true); });
  it("rejects a stale report", () => { expect(connectionIsFresh({ cloud_status: "online", cloud_last_seen_at: at(-301_000) }, now)).toBe(false); });
  it("rejects a report from the future", () => { expect(connectionIsFresh({ cloud_status: "online", cloud_last_seen_at: at(60_000) }, now)).toBe(false); });
  it("rejects a non-online status or missing time", () => {
    expect(connectionIsFresh({ cloud_status: "offline", cloud_last_seen_at: at(-1000) }, now)).toBe(false);
    expect(connectionIsFresh({ cloud_status: "online" }, now)).toBe(false);
  });
});
