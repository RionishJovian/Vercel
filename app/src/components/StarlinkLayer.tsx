import { useEffect, useState } from "react";
import type { GeoJSONSource, Map as MapLibre } from "maplibre-gl";
import { useI18n } from "../i18n";
import { AREA, heading, MYANMAR_BOUNDS, type OrbitFrame } from "../lib/starlink";

const ICONS = "starlink-icons", TRAILS = "starlink-trails", OUTLINE = "starlink-area", ICON = "starlink-satellite";
const INK = "15, 43, 61", ICON_PX = 16, ICON_SCALE = 2;
type Shapes = Extract<Parameters<GeoJSONSource["setData"]>[0], { type: "FeatureCollection" }>;
const EMPTY: Shapes = { type: "FeatureCollection", features: [] };
const OUTLINE_DATA: Shapes = { type: "FeatureCollection", features: [{ type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: [[AREA.west, AREA.south], [AREA.east, AREA.south], [AREA.east, AREA.north], [AREA.west, AREA.north], [AREA.west, AREA.south]] } }] };

function iconsData(c: Float32Array): Shapes {
  const features: Shapes["features"] = [];
  for (let i = 0; i < c.length; i += 4) features.push({ type: "Feature", properties: { heading: heading(c[i + 2], c[i + 3], c[i], c[i + 1]) }, geometry: { type: "Point", coordinates: [c[i], c[i + 1]] } });
  return { type: "FeatureCollection", features };
}

// Drawn from where the satellite was to where it is, so the gradient fades away behind it.
function trailsData(c: Float32Array): Shapes {
  const features: Shapes["features"] = [];
  for (let i = 0; i < c.length; i += 4) if (c[i] !== c[i + 2] || c[i + 1] !== c[i + 3]) features.push({ type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: [[c[i + 2], c[i + 3]], [c[i], c[i + 1]]] } });
  return { type: "FeatureCollection", features };
}

/** A small satellite pointing north: a body between two solar panels, ringed in white so it reads on land and sea. */
function drawIcon(): ImageData | null {
  const size = ICON_PX * ICON_SCALE, canvas = document.createElement("canvas");
  canvas.width = canvas.height = size;
  const g = canvas.getContext("2d");
  if (!g) return null;
  g.scale(size / 16, size / 16);
  const shape = () => { g.beginPath(); g.rect(6, 4.5, 4, 7); g.rect(1, 6, 4, 4); g.rect(11, 6, 4, 4); g.rect(5, 7.4, 6, 1.2); };
  shape(); g.strokeStyle = "#ffffff"; g.lineWidth = 2; g.lineJoin = "round"; g.stroke();
  shape(); g.fillStyle = `rgb(${INK})`; g.fill();
  return g.getImageData(0, 0, size, size);
}

function addLayers(m: MapLibre) {
  const icon = drawIcon();
  if (icon) m.addImage(ICON, icon, { pixelRatio: ICON_SCALE });
  m.addSource(OUTLINE, { type: "geojson", data: EMPTY });
  m.addSource(TRAILS, { type: "geojson", data: EMPTY, lineMetrics: true });
  m.addSource(ICONS, { type: "geojson", data: EMPTY, buffer: 16 });
  m.addLayer({ id: OUTLINE, type: "line", source: OUTLINE, paint: { "line-color": `rgb(${INK})`, "line-opacity": 0.45, "line-width": 1, "line-dasharray": [3, 3] } });
  m.addLayer({ id: TRAILS, type: "line", source: TRAILS, layout: { "line-cap": "round" }, paint: { "line-width": 1.5, "line-gradient": ["interpolate", ["linear"], ["line-progress"], 0, `rgba(${INK}, 0)`, 1, `rgba(${INK}, 0.8)`] } });
  m.addLayer({ id: ICONS, type: "symbol", source: ICONS, layout: { "icon-image": ICON, "icon-size": ["interpolate", ["linear"], ["zoom"], 3, 0.62, 8, 1.25], "icon-rotate": ["get", "heading"], "icon-rotation-alignment": "map", "icon-allow-overlap": true, "icon-ignore-placement": true } });
}

function removeLayers(m: MapLibre) {
  // The map may already be gone when the whole page is leaving.
  try { for (const id of [ICONS, TRAILS, OUTLINE]) { m.removeLayer(id); m.removeSource(id); } m.removeImage(ICON); } catch { /* nothing left to remove */ }
}

/** Draws only what the orbit data supports, and nothing when that data is old or missing. */
export default function StarlinkLayer({ map, onActive, defaultOn = false, compact = false }: { map: MapLibre; onActive: (on: boolean) => void; defaultOn?: boolean; compact?: boolean }) {
  const { t } = useI18n();
  const [on, setOn] = useState(defaultOn);
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
      (map.getSource(ICONS) as GeoJSONSource | undefined)?.setData(ready ? iconsData(frame.coords!) : EMPTY);
      (map.getSource(TRAILS) as GeoJSONSource | undefined)?.setData(ready ? trailsData(frame.coords!) : EMPTY);
      (map.getSource(OUTLINE) as GeoJSONSource | undefined)?.setData(ready ? OUTLINE_DATA : EMPTY);
      setView({ state: frame.state, ageHours: frame.ageHours, count: ready ? frame.coords!.length / 4 : 0 });
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
      {on && compact && (view.state === "stale" || view.state === "unavailable") && <p className="note small" role="status">{t(view.state === "stale" ? "starlink.stale" : "starlink.unavailable")}</p>}
      {on && !compact && (
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
