// Calculates where Starlink satellites should be from public orbit data (SGP4).
// These are predictions, not signals from SpaceX, and say nothing about any site's internet.
import { degreesLat, degreesLong, eciToGeodetic, gstime, json2satrec, propagate, type OMMJsonObject, type SatRec } from "satellite.js";

/** Myanmar, for framing the map. */
export const MYANMAR_BOUNDS: [[number, number], [number, number]] = [[92, 9.5], [101.3, 28.6]];
/** Myanmar plus a margin of about 2.5 degrees: only satellites in here are drawn or counted. */
export const AREA = { west: 89.5, east: 103.5, south: 7, north: 31 };

/** The full catalog is only searched this often; between searches just the nearby satellites are calculated. */
export const SCAN_EVERY_MS = 60_000;
/** Ground speed peaks near 0.072 degrees of latitude a second, so 60 s is about 4.3 degrees. Longitude stretches with latitude, up to about 1.35x at this area's edge. */
export const SCAN_MARGIN_DEG = 6;
/** Orbit data older than this is reported as out of date and nothing is drawn. */
export const STALE_AFTER_HOURS = 72;
/** A single object not updated for this long has probably decayed or stopped being tracked. */
const MAX_OBJECT_AGE_DAYS = 14;
/** Below this the object is re-entering, and SGP4 is no longer meaningful. */
const MIN_ALTITUDE_KM = 150;

const HOUR = 3_600_000;

export type OrbitRow = [norad: number, epoch: string, meanMotion: number, ecc: number, incl: number, raan: number, argp: number, anomaly: number, bstar: number, ndot: number, nddot: number];
export interface OrbitPayload { source: string; source_url: string; fetched_at: string; elements: OrbitRow[] }
export interface Satellite { satrec: SatRec; epochMs: number }

// CelesTrak gives EPOCH in UTC with no zone mark, which Date would read as local time.
const epochMs = (epoch: string) => Date.parse(epoch + "Z");

export function toSatellites(rows: OrbitRow[]): Satellite[] {
  const out: Satellite[] = [];
  for (const [norad, epoch, meanMotion, ecc, incl, raan, argp, anomaly, bstar, ndot, nddot] of rows) {
    const ms = epochMs(epoch);
    if (!Number.isFinite(ms)) continue;
    const satrec = json2satrec({
      OBJECT_NAME: String(norad), OBJECT_ID: String(norad), NORAD_CAT_ID: norad, EPOCH: epoch, MEAN_MOTION: meanMotion, ECCENTRICITY: ecc, INCLINATION: incl,
      RA_OF_ASC_NODE: raan, ARG_OF_PERICENTER: argp, MEAN_ANOMALY: anomaly, BSTAR: bstar, MEAN_MOTION_DOT: ndot, MEAN_MOTION_DDOT: nddot,
      EPHEMERIS_TYPE: 0, CLASSIFICATION_TYPE: "U", ELEMENT_SET_NO: 999, REV_AT_EPOCH: 0,
    } as OMMJsonObject);
    if (!satrec.error) out.push({ satrec, epochMs: ms });
  }
  return out;
}

/** How old the orbit data is (the median object), and whether it is still fit to draw. */
export function assess(fetchedAt: string, satellites: Satellite[], now: number): { fresh: boolean; ageHours: number } {
  const fetched = Date.parse(fetchedAt);
  if (!satellites.length || !Number.isFinite(fetched)) return { fresh: false, ageHours: Infinity };
  const epochs = satellites.map((s) => s.epochMs).sort((a, b) => a - b);
  const ageHours = (now - epochs[epochs.length >> 1]) / HOUR;
  // A negative age means this device's clock is well behind the data: the positions would be wrong too.
  const fresh = ageHours > -24 && ageHours <= STALE_AFTER_HOURS && (now - fetched) / HOUR <= STALE_AFTER_HOURS;
  return { fresh, ageHours: Math.max(0, ageHours) };
}

function position(satrec: SatRec, date: Date, gmst: ReturnType<typeof gstime>): [lng: number, lat: number] | null {
  const state = propagate(satrec, date);
  if (!state || typeof state.position !== "object" || !state.position) return null;
  const geo = eciToGeodetic(state.position, gmst);
  if (!Number.isFinite(geo.height) || geo.height < MIN_ALTITUDE_KM) return null;
  return [degreesLong(geo.longitude), degreesLat(geo.latitude)];
}

const inArea = (lng: number, lat: number, margin: number) => lat >= AREA.south - margin && lat <= AREA.north + margin && lng >= AREA.west - margin && lng <= AREA.east + margin;

/** The slow search: every satellite that is or could soon be in the area. */
export function nearby(satellites: Satellite[], date: Date): Satellite[] {
  const gmst = gstime(date), oldest = date.getTime() - MAX_OBJECT_AGE_DAYS * 24 * HOUR;
  return satellites.filter((s) => {
    if (s.epochMs < oldest) return false;
    const p = position(s.satrec, date, gmst);
    return p !== null && inArea(p[0], p[1], SCAN_MARGIN_DEG);
  });
}

/** The quick step: positions now, as lng,lat pairs, for those actually in the area. */
export function inAreaNow(candidates: Satellite[], date: Date): Float32Array {
  const gmst = gstime(date), coords = new Float32Array(candidates.length * 2);
  let n = 0;
  for (const s of candidates) {
    const p = position(s.satrec, date, gmst);
    if (p && inArea(p[0], p[1], 0)) { coords[n++] = p[0]; coords[n++] = p[1]; }
  }
  return coords.slice(0, n);
}

export type OrbitState = "loading" | "ready" | "stale" | "unavailable";
export interface OrbitFrame { state: OrbitState; ageHours?: number; coords?: Float32Array }
