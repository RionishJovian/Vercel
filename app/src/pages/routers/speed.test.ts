import { describe, expect, it } from "vitest";
import { rateText, stillMeasuring } from "./speed";

describe("port speed text", () => {
  it("formats real speeds, including zero", () => {
    expect(rateText(0, "Measuring…")).toBe("0 kbps");
    expect(rateText(5026, "Measuring…")).toBe("5 kbps");
    expect(rateText(2_500_000, null)).toBe("2.5 Mbps");
  });
  it("shows the measuring label only when asked, otherwise a dash", () => {
    expect(rateText(undefined, "Measuring…")).toBe("Measuring…");
    expect(rateText(undefined, null)).toBe("—");
  });
  it("measures only during the first readings and only while no port has a speed", () => {
    const none = [{ rx_bps: undefined }, {}];
    expect(stillMeasuring(none, 1)).toBe(true);
    expect(stillMeasuring(none, 2)).toBe(true);
    expect(stillMeasuring(none, 3)).toBe(false);
    expect(stillMeasuring([{ rx_bps: 0, tx_bps: 0 }], 1)).toBe(false);
    expect(stillMeasuring([{ tx_bps: 7 }], 1)).toBe(false);
    expect(stillMeasuring([], 1)).toBe(false);
  });
});
