import { AREA, assess, inAreaNow, nearby, SCAN_EVERY_MS, STALE_AFTER_HOURS, toSatellites, type OrbitRow, type Satellite } from "./starlink";

const NOW = Date.parse("2026-11-06T06:00:00Z");
const row = (norad: number, epoch: string, raan: number, anomaly: number, incl = 53): OrbitRow => [norad, epoch, 15.06, 0.0001, incl, raan, 90, anomaly, 0.0002, 0.00001, 0];
const hoursAgo = (h: number) => new Date(NOW - h * 3_600_000).toISOString().replace("Z", "");

// A synthetic constellation that covers the sky: every plane, every phase, several inclinations.
function constellation(): Satellite[] {
  const rows: OrbitRow[] = [];
  let id = 1;
  for (const incl of [43, 53, 70, 97.6]) for (let raan = 0; raan < 360; raan += 6) for (let m = 0; m < 360; m += 6) rows.push(row(id++, hoursAgo(12), raan, m, incl));
  return toSatellites(rows);
}

describe("orbit data age", () => {
  it("reads the epoch as UTC whatever the device's time zone", () => {
    expect(toSatellites([row(1, "2026-11-06T00:00:00.000000", 0, 0)])[0].epochMs).toBe(Date.parse("2026-11-06T00:00:00Z"));
  });
  it("draws data within the limit and reports its median age", () => {
    const sats = toSatellites([row(1, hoursAgo(10), 0, 0), row(2, hoursAgo(20), 0, 0), row(3, hoursAgo(300), 0, 0)]);
    expect(assess(new Date(NOW).toISOString(), sats, NOW)).toMatchObject({ fresh: true, ageHours: expect.closeTo(20, 3) });
  });
  it("refuses old elements, an old download, no data and a device clock far behind", () => {
    const old = toSatellites([row(1, hoursAgo(STALE_AFTER_HOURS + 1), 0, 0)]), recent = toSatellites([row(1, hoursAgo(5), 0, 0)]);
    expect(assess(new Date(NOW).toISOString(), old, NOW).fresh).toBe(false);
    expect(assess(new Date(NOW - (STALE_AFTER_HOURS + 1) * 3_600_000).toISOString(), recent, NOW).fresh).toBe(false);
    expect(assess(new Date(NOW).toISOString(), [], NOW).fresh).toBe(false);
    expect(assess("not a date", recent, NOW).fresh).toBe(false);
    expect(assess(new Date(NOW).toISOString(), recent, NOW - 72 * 3_600_000).fresh).toBe(false);
  });
});

describe("satellites over the area", () => {
  const sats = constellation();
  const key = (coords: Float32Array) => { const out: string[] = []; for (let i = 0; i < coords.length; i += 2) out.push(coords[i].toFixed(3) + "," + coords[i + 1].toFixed(3)); return out.sort(); };

  it("only returns positions inside the area", () => {
    const date = new Date(NOW), coords = inAreaNow(nearby(sats, date), date);
    expect(coords.length).toBeGreaterThan(0);
    for (let i = 0; i < coords.length; i += 2) {
      expect(coords[i]).toBeGreaterThanOrEqual(AREA.west); expect(coords[i]).toBeLessThanOrEqual(AREA.east);
      expect(coords[i + 1]).toBeGreaterThanOrEqual(AREA.south); expect(coords[i + 1]).toBeLessThanOrEqual(AREA.north);
    }
  });
  // The search runs once a minute, so anything entering the area before the next search must already be in its list.
  it("misses nothing between catalog searches", () => {
    const start = new Date(NOW), candidates = nearby(sats, start), later = new Date(NOW + SCAN_EVERY_MS);
    expect(candidates.length).toBeLessThan(sats.length / 4);
    expect(key(inAreaNow(candidates, later))).toEqual(key(inAreaNow(sats, later)));
  });
  it("skips objects with orbit data too old to trust", () => {
    const date = new Date(NOW);
    const old = toSatellites(Array.from({ length: 3600 }, (_, i) => row(i + 1, hoursAgo(24 * 15), i % 360, (i * 7) % 360)));
    expect(nearby(old, date)).toEqual([]);
  });
});
