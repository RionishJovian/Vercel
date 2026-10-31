import { pace } from "./Home";

describe("pace", () => {
  it("is null when there is nothing to compare", () => { expect(pace(500, 0)).toBeNull(); });
  it("puts an average day at half the dial", () => { expect(pace(100, 700)).toEqual({ ratio: 1, fraction: 0.5 }); });
  it("caps at a full dial for a very good day", () => { expect(pace(1000, 700)?.fraction).toBe(1); });
});
