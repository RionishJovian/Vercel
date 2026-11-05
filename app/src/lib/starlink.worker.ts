/// <reference lib="webworker" />
// Downloads and calculates off the main thread so the map stays smooth on phones. Only runs while the page is showing.
import { assess, inAreaNow, nearby, SCAN_EVERY_MS, toSatellites, type OrbitFrame, type OrbitPayload, type Satellite } from "./starlink";

const CHECK_READY_MS = 3_600_000, CHECK_FAILED_MS = 300_000;
const URL_ORBITS = "/api/v1/map/starlink", URL_REFRESH = "/api/v1/auth/refresh";

let satellites: Satellite[] = [], candidates: Satellite[] = [], fetchedAt = "", etag = "";
let lastScan = 0, lastLoad = 0, loading = false, timer: ReturnType<typeof setInterval> | undefined;

const post = (frame: OrbitFrame) => self.postMessage(frame, frame.coords ? [frame.coords.buffer as ArrayBuffer] : []);
const orbits = () => fetch(URL_ORBITS, { headers: { Accept: "application/json", "X-MM-CSRF": "1" } });

// Same as the app's API client: an expired session renews once before giving up.
async function fetchOrbits(): Promise<Response> {
  const res = await orbits();
  if (res.status !== 401) return res;
  const renewed = await fetch(URL_REFRESH, { method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json", "X-MM-CSRF": "1" }, body: "{}" });
  return renewed.ok ? orbits() : res;
}

async function load() {
  loading = true; lastLoad = Date.now();
  try {
    const res = await fetchOrbits();
    if (!res.ok) throw new Error("http-" + res.status);
    const tag = res.headers.get("etag") ?? "";
    if (tag && tag === etag) return;
    const data = await res.json() as OrbitPayload;
    if (!Array.isArray(data.elements)) throw new Error("bad-payload");
    satellites = toSatellites(data.elements); fetchedAt = data.fetched_at; etag = tag; lastScan = 0;
  } catch { /* Whatever was loaded stays until it goes out of date; tick() reports the rest. */ }
  finally { loading = false; }
}

function tick() {
  const now = Date.now();
  const verdict = assess(fetchedAt, satellites, now);
  if (!loading && now - lastLoad > (verdict.fresh ? CHECK_READY_MS : CHECK_FAILED_MS)) { void load().then(tick); }
  if (!satellites.length) { post({ state: loading ? "loading" : "unavailable" }); return; }
  if (!verdict.fresh) { post({ state: "stale" }); return; }
  const date = new Date(now);
  if (now - lastScan >= SCAN_EVERY_MS) { candidates = nearby(satellites, date); lastScan = now; }
  post({ state: "ready", ageHours: verdict.ageHours, coords: inAreaNow(candidates, date) });
}

self.onmessage = ({ data }: MessageEvent<{ type: "start"; intervalMs: number } | { type: "stop" }>) => {
  clearInterval(timer); timer = undefined;
  if (data.type === "start") { timer = setInterval(tick, data.intervalMs); tick(); }
};
