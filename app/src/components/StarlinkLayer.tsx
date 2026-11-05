import { useEffect, useState } from "react";
import type { GeoJSONSource, Map as MapLibre } from "maplibre-gl";
import { useI18n } from "../i18n";
import { AREA, MYANMAR_BOUNDS, type OrbitFrame } from "../lib/starlink";

const DOTS = "starlink-dots", OUTLINE = "starlink-area";
type Shapes = Extract<Parameters<GeoJSONSource["setData"]>[0], { type: "FeatureCollection" }>;
const EMPTY: Shapes = { type: "FeatureCollection", features: [] };
const OUTLINE_DATA: Shapes = { type: "FeatureCollection", features: [{ type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: [[AREA.west, AREA.south], [AREA.east, AREA.south], [AREA.east, AREA.north], [AREA.west, AREA.north], [AREA.west, AREA.south]] } }] };

function dotsData(coords: Float32Array): Shapes {
  const points: number[][] = [];
  for (let i = 0; i < coords.length; i += 2) points.push([coords[i], coords[i + 1]]);
  return { type: "FeatureCollection", features: [{ type: "Feature", properties: {}, geometry: { type: "MultiPoint", coordinates: points } }] };
}

function addLayers(m: MapLibre) {
  m.addSource(OUTLINE, { type: "geojson", data: EMPTY });
  m.addSource(DOTS, { type: "geojson", data: EMPTY, buffer: 8 });
  m.addLayer({ id: OUTLINE, type: "line", source: OUTLINE, paint: { "line-color": "#0f2b3d", "line-opacity": 0.45, "line-width": 1, "line-dasharray": [3, 3] } });
  m.addLayer({ id: DOTS, type: "circle", source: DOTS, paint: { "circle-radius": ["interpolate", ["linear"], ["zoom"], 3, 2.5, 8, 4], "circle-color": "#0f2b3d", "circle-stroke-color": "#ffffff", "circle-stroke-width": 1 } });
}

function removeLayers(m: MapLibre) {
  // The map may already be gone when the whole page is leaving.
  try { for (const id of [DOTS, OUTLINE]) { m.removeLayer(id); m.removeSource(id); } } catch { /* nothing left to remove */ }
}

/** Off until asked for. Draws only what the orbit data supports, and nothing when that data is old or missing. */
export default function StarlinkLayer({ map, onActive }: { map: MapLibre; onActive: (on: boolean) => void }) {
  const { t } = useI18n();
  const [on, setOn] = useState(false);
  const [view, setView] = useState<{ state: OrbitFrame["state"]; ageHours?: number; count: number }>({ state: "loading", count: 0 });

  useEffect(() => {
    if (!on) return;
    const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches, duration = reduced ? 0 : 500;
    const before = { center: map.getCenter(), zoom: map.getZoom() };
    onActive(true);
    addLayers(map);
    map.fitBounds(MYANMAR_BOUNDS, { padding: 12, duration });
    setView({ state: "loading", count: 0 });

    const worker = new Worker(new URL("../lib/starlink.worker.ts", import.meta.url), { type: "module" });
    const draw = (frame: OrbitFrame) => {
      const ready = frame.state === "ready" && !!frame.coords;
      (map.getSource(DOTS) as GeoJSONSource | undefined)?.setData(ready ? dotsData(frame.coords!) : EMPTY);
      (map.getSource(OUTLINE) as GeoJSONSource | undefined)?.setData(ready ? OUTLINE_DATA : EMPTY);
      setView({ state: frame.state, ageHours: frame.ageHours, count: ready ? frame.coords!.length / 2 : 0 });
    };
    worker.onmessage = (e: MessageEvent<OrbitFrame>) => draw(e.data);
    worker.onerror = () => draw({ state: "unavailable" });
    // Reduced motion gets occasional still updates instead of continuous movement.
    const run = () => worker.postMessage(document.hidden ? { type: "stop" } : { type: "start", intervalMs: reduced ? 30_000 : 1_000 });
    document.addEventListener("visibilitychange", run);
    run();

    return () => {
      document.removeEventListener("visibilitychange", run);
      worker.terminate();
      removeLayers(map);
      onActive(false);
      try { map.easeTo({ ...before, duration }); } catch { /* the map is already gone */ }
    };
  }, [on, map, onActive]);

  const age = view.ageHours === undefined ? "" : view.ageHours < 1 ? t("starlink.ageFresh") : t("starlink.age").replace("{h}", String(Math.round(view.ageHours)));
  return (
    <>
      <button type="button" className="pill starlink-toggle" aria-pressed={on} onClick={() => setOn((x) => !x)}>
        <span className="starlink-mark" aria-hidden="true" />{t("starlink.toggle")}
      </button>
      {on && (
        <div className="note starlink-panel small">
          <p role="status">
            {view.state === "loading" && t("starlink.loading")}
            {view.state === "stale" && t("starlink.stale")}
            {view.state === "unavailable" && t("starlink.unavailable")}
          </p>
          {view.state === "ready" && <p><b className="num">{t("starlink.count").replace("{n}", String(view.count))}</b> <span className="soft">{age}</span></p>}
          <p className="soft">{t("starlink.note")}</p>
          <p className="soft"><a href="https://celestrak.org/" target="_blank" rel="noopener noreferrer">{t("starlink.credit")}</a></p>
        </div>
      )}
    </>
  );
}
