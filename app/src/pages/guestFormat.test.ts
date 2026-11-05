import { describe, expect, it } from "vitest";
import { pageWindow } from "../components/DenseTable";
import { fmtBytes, fmtDuration, fmtUptime, parseSeconds } from "./guestFormat";

describe("guest formatting", () => {
  it("writes durations the way the router does", () => {
    expect(fmtDuration(3863)).toBe("1h4m23s");
    expect(fmtDuration(0)).toBe("0s");
    expect(fmtDuration(90061)).toBe("1d1h1m1s");
  });
  it("normalises the router's clock form and leaves other forms alone", () => {
    expect(fmtUptime("00:05:12")).toBe("5m12s");
    expect(fmtUptime("2d03:04:05")).toBe("2d3h4m5s");
    expect(fmtUptime("1h4m23s")).toBe("1h4m23s");
    expect(fmtUptime(undefined)).toBe("—");
  });
  it("reads either duration form, and nothing from an empty one", () => {
    expect(parseSeconds("00:06:00")).toBe(360);
    expect(parseSeconds("1h4m23s")).toBe(3863);
    expect(parseSeconds("1d")).toBe(86400);
    expect(parseSeconds("2d03:04:05")).toBe(183845);
    expect(parseSeconds("")).toBeNull();
    expect(parseSeconds(undefined)).toBeNull();
    expect(parseSeconds("soon")).toBeNull();
  });
  it("sizes bytes", () => {
    expect(fmtBytes("321000")).toBe("321 KB");
    expect(fmtBytes("7000000")).toBe("7.0 MB");
    expect(fmtBytes("1100000000")).toBe("1.1 GB");
    expect(fmtBytes(undefined)).toBe("—");
    expect(fmtBytes("abc")).toBe("—");
  });
});

describe("pageWindow", () => {
  it("lists every page up to seven", () => { expect(pageWindow(3, 6)).toEqual([1, 2, 3, 4, 5, 6]); });
  it("marks gaps around the current page", () => {
    expect(pageWindow(10, 20)).toEqual([1, null, 9, 10, 11, null, 20]);
    expect(pageWindow(1, 20)).toEqual([1, 2, null, 20]);
  });
});
