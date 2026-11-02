import { describe, expect, it } from "vitest";
import { speedOk, speedValue } from "./planSpeed";

describe("plan speed", () => {
  it("accepts no limit, or both a download and an upload speed", () => {
    expect(speedOk({})).toBe(true);
    expect(speedOk({ speed_down_mbps: 0, speed_up_mbps: 0 })).toBe(true);
    expect(speedOk({ speed_down_mbps: 20, speed_up_mbps: 5 })).toBe(true);
    expect(speedOk({ speed_down_mbps: 10000, speed_up_mbps: 1 })).toBe(true);
  });
  it("refuses one without the other, out of range and fractional values", () => {
    expect(speedOk({ speed_down_mbps: 20, speed_up_mbps: 0 })).toBe(false);
    expect(speedOk({ speed_down_mbps: 0, speed_up_mbps: 5 })).toBe(false);
    expect(speedOk({ speed_down_mbps: 10001, speed_up_mbps: 5 })).toBe(false);
    expect(speedOk({ speed_down_mbps: 20, speed_up_mbps: -1 })).toBe(false);
    expect(speedOk({ speed_down_mbps: 2.5, speed_up_mbps: 1 })).toBe(false);
  });
  it("reads a number input safely", () => {
    expect(speedValue("")).toBe(0);
    expect(speedValue("abc")).toBe(0);
    expect(speedValue("-5")).toBe(0);
    expect(speedValue("20")).toBe(20);
    expect(speedValue("007")).toBe(7);
  });
});
