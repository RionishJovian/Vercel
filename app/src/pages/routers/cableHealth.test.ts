import { describe, expect, it } from "vitest";
import { cableHints, needsAttention, rateMbps } from "./cableHealth";

const up = { running: true, link_rate: "1Gbps", full_duplex: true, negotiation: "done", max_rate_mbps: 1000, partner_max_mbps: 1000 };
const ids = (f: Parameters<typeof cableHints>[0]) => cableHints(f).map((h) => h.id);

describe("rateMbps", () => {
  it("reads RouterOS rates and treats anything else as unknown", () => {
    expect(rateMbps("1Gbps")).toBe(1000);
    expect(rateMbps("100Mbps")).toBe(100);
    expect(rateMbps("2.5Gbps")).toBe(2500);
    expect(rateMbps("10Gbps")).toBe(10000);
    for (const bad of [undefined, "", "fast", "0Mbps", "1gbps", "1Gbps "]) expect(rateMbps(bad as string)).toBe(0);
  });
});

describe("cable hints", () => {
  it("says nothing about a healthy link at its top speed", () => {
    expect(ids(up)).toEqual([]);
  });
  it("never suspects an empty or disabled port", () => {
    expect(ids({ ...up, running: false, negotiation: "failed", phy_errors: "50", phy_errors_recent: 9 })).toEqual([]);
    expect(ids({ ...up, disabled: true, phy_errors_recent: 9 })).toEqual([]);
  });
  it("blames the cable when both ends support more than the link runs at", () => {
    const f = { ...up, link_rate: "100Mbps", partner_max_mbps: 1000 };
    expect(ids(f)).toEqual(["slowCable"]);
    expect(needsAttention(cableHints(f))).toBe(true);
  });
  it("does not blame the cable when the connected device is the limit", () => {
    const f = { ...up, link_rate: "100Mbps", partner_max_mbps: 100 };
    expect(ids(f)).toEqual(["partnerLimit"]);
    expect(needsAttention(cableHints(f))).toBe(false);
  });
  it("is honest when it cannot tell what the far end supports, or the speed is set by hand", () => {
    expect(ids({ ...up, link_rate: "100Mbps", partner_max_mbps: 0 })).toEqual(["belowMax"]);
    expect(ids({ ...up, link_rate: "100Mbps", negotiation: "disabled" })).toEqual(["fixedSpeed"]);
    expect(ids({ ...up, link_rate: "100Mbps", max_rate_mbps: 0 })).toEqual([]);
  });
  it("flags a negotiation that did not finish and half duplex", () => {
    expect(ids({ ...up, negotiation: "failed" })).toEqual(["neg"]);
    expect(ids({ ...up, negotiation: "incomplete" })).toEqual(["neg"]);
    expect(ids({ ...up, full_duplex: false })).toEqual(["half"]);
  });
  it("separates errors still increasing from old ones, and from no trend yet", () => {
    expect(ids({ ...up, phy_errors: "12", phy_errors_recent: 3 })).toEqual(["errNow"]);
    expect(ids({ ...up, phy_errors: "12", phy_errors_recent: 0 })).toEqual(["errPast"]);
    expect(ids({ ...up, phy_errors: "12" })).toEqual(["errWatching"]);
    expect(ids({ ...up, phy_errors: "0", phy_errors_recent: 0 })).toEqual([]);
    expect(ids({ ...up })).toEqual([]);
    expect(ids({ ...up, phy_errors: "abc" })).toEqual([]);
  });
  it("can show several findings together", () => {
    expect(ids({ ...up, link_rate: "100Mbps", partner_max_mbps: 1000, phy_errors: "7", phy_errors_recent: 2 })).toEqual(["slowCable", "errNow"]);
  });
});
