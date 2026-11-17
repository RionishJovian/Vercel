import { ageShort, splitCidr } from "./age";

describe("ageShort", () => {
  const now = Date.parse("2026-11-13T12:00:00Z");
  it("reads seconds, minutes, hours and days", () => {
    expect(ageShort("2026-11-13T11:59:18Z", now)).toBe("42s");
    expect(ageShort("2026-11-13T11:55:00Z", now)).toBe("5m");
    expect(ageShort("2026-11-13T09:00:00Z", now)).toBe("3h");
    expect(ageShort("2026-11-11T12:00:00Z", now)).toBe("2d");
  });
  it("returns null for missing or invalid times and never goes negative", () => {
    expect(ageShort(undefined, now)).toBeNull();
    expect(ageShort("nope", now)).toBeNull();
    expect(ageShort("2026-11-13T12:00:30Z", now)).toBe("0s");
  });
});
describe("splitCidr", () => {
  it("separates the /32 prefix", () => {
    expect(splitCidr("10.8.0.12/32")).toEqual({ addr: "10.8.0.12", cidr: "/32" });
    expect(splitCidr("10.8.0.12")).toEqual({ addr: "10.8.0.12", cidr: "" });
    expect(splitCidr("")).toBeNull();
  });
});
