import { useEffect, useRef } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

export interface MapSite { site_id: string; name: string; latitude?: number | null; longitude?: number | null }
export type PathState = "online" | "offline" | "unassigned";
interface Props { sites: MapSite[]; states: Record<string, PathState>; selected: string; onSelect: (id: string) => void; pick?: { lat: string; lng: string; onPick: (lat: number, lng: number) => void } }

export const validCoordinates = (s: { latitude?: unknown; longitude?: unknown }) =>
  s.latitude !== null && s.latitude !== undefined && s.latitude !== "" && s.longitude !== null && s.longitude !== undefined && s.longitude !== "" &&
  Number.isFinite(Number(s.latitude)) && Number.isFinite(Number(s.longitude)) && Math.abs(Number(s.latitude)) <= 85.051129 && Math.abs(Number(s.longitude)) <= 180;

/** Pins show each site's management-path state. Tiles come from OpenFreeMap; site names and router details never leave Mega Magic. */
export default function SiteMap({ sites, states, selected, onSelect, pick }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const map = useRef<maplibregl.Map | null>(null);
  const markers = useRef<maplibregl.Marker[]>([]);
  const draft = useRef<maplibregl.Marker | null>(null);
  const handlers = useRef({ onSelect, pick });
  handlers.current = { onSelect, pick };
  const positioned = sites.filter(validCoordinates);

  useEffect(() => {
    if (!box.current) return;
    const first = positioned.find((s) => s.site_id === selected);
    const m = new maplibregl.Map({ container: box.current, style: "https://tiles.openfreemap.org/styles/liberty", center: first ? [Number(first.longitude), Number(first.latitude)] : [95.9, 21.9], zoom: first ? 13 : 4.8, attributionControl: false, cooperativeGestures: true });
    m.addControl(new maplibregl.AttributionControl({ compact: false }));
    m.addControl(new maplibregl.NavigationControl({ visualizePitch: false }), "top-right");
    m.on("click", (e) => { const p = handlers.current.pick; if (!p) return; p.onPick(e.lngLat.lat, e.lngLat.lng); });
    map.current = m;
    return () => { markers.current.forEach((x) => x.remove()); draft.current?.remove(); draft.current = null; m.remove(); map.current = null; };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Pins: rebuilt when sites, states or the selection change.
  useEffect(() => {
    const m = map.current; if (!m) return;
    markers.current.forEach((x) => x.remove()); markers.current = [];
    for (const s of positioned) {
      const b = document.createElement("button");
      b.type = "button"; b.className = `site-pin ${states[s.site_id] ?? "unassigned"}${s.site_id === selected ? " is-selected" : ""}`; b.setAttribute("aria-label", s.name); b.title = s.name;
      const dot = document.createElement("span"), label = document.createElement("strong"); label.textContent = s.name; b.append(dot, label);
      b.addEventListener("click", (ev) => { ev.stopPropagation(); if (!handlers.current.pick) handlers.current.onSelect(s.site_id); });
      markers.current.push(new maplibregl.Marker({ element: b, anchor: "bottom" }).setLngLat([Number(s.longitude), Number(s.latitude)]).addTo(m));
    }
    const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches, duration = reduced ? 0 : 500;
    const sel = positioned.find((s) => s.site_id === selected);
    if (sel) m.easeTo({ center: [Number(sel.longitude), Number(sel.latitude)], zoom: Math.max(m.getZoom(), 12), duration });
    else if (positioned.length === 1) m.easeTo({ center: [Number(positioned[0].longitude), Number(positioned[0].latitude)], zoom: 13, duration });
    else if (positioned.length > 1) { const bounds = new maplibregl.LngLatBounds(); positioned.forEach((s) => bounds.extend([Number(s.longitude), Number(s.latitude)])); m.fitBounds(bounds, { padding: 65, maxZoom: 13, duration }); }
  }, [positioned.map((s) => `${s.site_id}:${s.latitude}:${s.longitude}`).join("|"), JSON.stringify(states), selected]); // eslint-disable-line react-hooks/exhaustive-deps

  // A draggable pin while a site's location is being chosen.
  useEffect(() => {
    const m = map.current; if (!m) return;
    const lat = Number(pick?.lat), lng = Number(pick?.lng);
    if (!pick || !pick.lat || !pick.lng || !Number.isFinite(lat) || !Number.isFinite(lng)) { draft.current?.remove(); draft.current = null; return; }
    if (!draft.current) {
      draft.current = new maplibregl.Marker({ draggable: true, color: "#146b52" }).setLngLat([lng, lat]).addTo(m);
      draft.current.on("dragend", () => { const p = draft.current!.getLngLat(); handlers.current.pick?.onPick(p.lat, p.lng); });
    } else draft.current.setLngLat([lng, lat]);
  }, [pick?.lat, pick?.lng, !!pick]); // eslint-disable-line react-hooks/exhaustive-deps

  return <div ref={box} className="site-map" role="region" aria-label="Site map" />;
}
