import * as maplibregl from "/assets/vendor/maplibre/maplibre-gl.js";

maplibregl.setWorkerUrl("/assets/vendor/maplibre/maplibre-gl-worker.js");

export function validCoordinates(site) {
  return site.latitude !== null && site.latitude !== undefined && site.latitude !== "" &&
    site.longitude !== null && site.longitude !== undefined && site.longitude !== "" &&
    Number.isFinite(Number(site.latitude)) && Number.isFinite(Number(site.longitude)) &&
    Math.abs(Number(site.latitude)) <= 85.051129 && Math.abs(Number(site.longitude)) <= 180;
}

export function mountSiteMap(container, sites, connections, onSelect) {
  const shell = container.parentElement;
  const status = shell.querySelector("#site-map-status");
  const positioned = sites.filter(validCoordinates);
  const selected = positioned.find(s => s.site_id === localStorage.getItem("mm_selected_site"));
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const map = new maplibregl.Map({
    container, style: "https://tiles.openfreemap.org/styles/liberty",
    center: selected ? [Number(selected.longitude), Number(selected.latitude)] : [95.9, 21.9],
    zoom: selected ? 13 : 4.8, maxPitch: 65, attributionControl: false,
    cooperativeGestures: true,
  });
  // Always visible, including mobile and both view modes. Provider attribution
  // is also obtained from the tile source; never remove the data credits.
  map.addControl(new maplibregl.AttributionControl({ compact: false,
    customAttribution: '<a href="https://openfreemap.org/" target="_blank" rel="noopener">OpenFreeMap</a> · © <a href="https://www.openmaptiles.org/" target="_blank" rel="noopener">OpenMapTiles</a> · <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a>' }), "bottom-left");
  map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }), "top-right");
  const markers = [];
  let picking = null, draftMarker = null;
  const writePin = point => {
    if (!picking) return;
    picking.latitude.value = point.lat.toFixed(6);
    picking.longitude.value = point.lng.toFixed(6);
    picking.status.textContent = "Pin selected. Drag to adjust; save the site to keep it.";
  };
  const draftPin = (point, write = true) => {
    if (!draftMarker) {
      draftMarker = new maplibregl.Marker({draggable:true,color:"#146b52"}).setLngLat(point).addTo(map);
      draftMarker.on("dragend", () => writePin(draftMarker.getLngLat()));
    } else draftMarker.setLngLat(point);
    if (write) writePin(point);
  };
  for (const site of positioned) {
    const paths = connections.filter(c => c.site_id === site.site_id);
    const fresh = c => {const seen=Date.parse(c.cloud_last_seen_at),now=Date.now();return String(c.cloud_status||"").toLowerCase()==="online" && Number.isFinite(seen) && seen<=now+30000 && now-seen<=300000;};
    const state = paths.some(fresh) ? "online" : paths.length ? "offline" : "unassigned";
    const button = document.createElement("button");
    button.type = "button";
    button.className = `site-global-pin ${state}${selected?.site_id === site.site_id ? " is-selected" : ""}`;
    button.setAttribute("aria-label", `Select ${site.name}`);
    button.title = site.name;
    const dot = document.createElement("span");
    const label = document.createElement("strong");
    label.textContent = site.name;
    button.append(dot, label);
    button.addEventListener("click", () => {if (!picking) onSelect(site.site_id);});
    markers.push(new maplibregl.Marker({ element: button, anchor: "bottom" }).setLngLat([Number(site.longitude), Number(site.latitude)]).addTo(map));
  }
  let loaded = false, disposed = false, mode3d = false;
  const fit = () => {
    if (!positioned.length) { map.easeTo({center: [95.9, 21.9], zoom: 4.8, duration: reduced ? 0 : 500}); return; }
    if (positioned.length === 1) { map.easeTo({center: [Number(positioned[0].longitude), Number(positioned[0].latitude)], zoom: 13, duration: reduced ? 0 : 500}); return; }
    const bounds = new maplibregl.LngLatBounds();
    for (const s of positioned) bounds.extend([Number(s.longitude), Number(s.latitude)]);
    map.fitBounds(bounds, { padding: 65, maxZoom: 13, duration: reduced ? 0 : 500 });
  };
  const setMode = three => {
    mode3d = three;
    shell.querySelector("#site-map-2d").setAttribute("aria-pressed", String(!three));
    shell.querySelector("#site-map-3d").setAttribute("aria-pressed", String(three));
    if (loaded) map.setLayoutProperty("mm-3d-buildings", "visibility", three ? "visible" : "none");
    map.easeTo({ pitch: three ? 55 : 0, bearing: three ? -15 : 0, duration: reduced ? 0 : 600 });
    status.textContent = three ? "3D enabled — zoom in to see buildings where mapped." : positioned.length ? "Select a location marker to open its details." : "Myanmar map ready. Add a site and pin its location.";
  };
  shell.querySelector("#site-map-2d").onclick = () => setMode(false);
  shell.querySelector("#site-map-3d").onclick = () => setMode(true);
  shell.querySelector("#site-map-fit").onclick = fit;
  map.on("load", () => {
    if (disposed) return;
    const labelLayer = map.getStyle().layers.find(l => l.type === "symbol" && l.layout?.["text-field"]);
    map.addSource("mm-buildings", {type: "vector", url: "https://tiles.openfreemap.org/planet"});
    map.addLayer({id: "mm-3d-buildings", type: "fill-extrusion", source: "mm-buildings", "source-layer": "building", minzoom: 15,
      layout: {visibility: mode3d ? "visible" : "none"}, filter: ["!=", ["get", "hide_3d"], true],
      paint: {"fill-extrusion-color": "#8fb69d", "fill-extrusion-height": ["coalesce", ["get", "render_height"], 3],
        "fill-extrusion-base": ["coalesce", ["get", "render_min_height"], 0], "fill-extrusion-opacity": 0.85}}, labelLayer?.id);
    loaded = true;
    setMode(mode3d);
  });
  map.on("error", () => { if (!disposed) status.textContent = "Some map data could not load. Location controls below remain available."; });
  map.on("click", event => {if (picking) draftPin(event.lngLat);});
  const observer = new ResizeObserver(() => { if (!disposed) map.resize(); });
  observer.observe(container);
  const stopPicking = () => {
    if (picking) {
      picking.latitude.removeEventListener("change", picking.sync);
      picking.longitude.removeEventListener("change", picking.sync);
      picking.center.onclick = null; picking.clear.onclick = null;
    }
    picking = null; draftMarker?.remove(); draftMarker = null;
    if (!disposed) status.textContent = positioned.length ? "Select a location marker to open its details." : "Myanmar map ready. Add a site and pin its location.";
  };
  const dispose = () => { disposed = true; stopPicking(); observer.disconnect(); markers.forEach(m => m.remove()); map.remove(); };
  dispose.beginPicking = (latitude, longitude, pinStatus, center, clear) => {
    stopPicking();
    const sync = () => {
      if (validCoordinates({latitude:latitude.value,longitude:longitude.value})) draftPin({lat:Number(latitude.value),lng:Number(longitude.value)},false);
      else {draftMarker?.remove();draftMarker=null;}
    };
    picking = {latitude,longitude,status:pinStatus,center,clear,sync};
    latitude.addEventListener("change", sync); longitude.addEventListener("change", sync);
    center.onclick = () => draftPin(map.getCenter());
    clear.onclick = () => {draftMarker?.remove();draftMarker=null;latitude.value="";longitude.value="";pinStatus.textContent="Pin cleared. Click the main map to select a location.";};
    sync();
    if (draftMarker) map.easeTo({center:[Number(longitude.value),Number(latitude.value)],zoom:14,duration:reduced?0:400});
    status.textContent="Location editing — click this map or drag the draft pin. Save or cancel below.";
    return stopPicking;
  };
  return dispose;
}

export function mountLocationPicker(container, latitude, longitude, status, centerButton, clearButton) {
  const initial = {latitude: latitude.value, longitude: longitude.value};
  const hasPin = validCoordinates(initial);
  const map = new maplibregl.Map({
    container, style: "https://tiles.openfreemap.org/styles/liberty",
    center: hasPin ? [Number(initial.longitude), Number(initial.latitude)] : [95.9, 21.9],
    zoom: hasPin ? 14 : 4.8,
    attributionControl: false,
  });
  map.addControl(new maplibregl.AttributionControl({compact: false}), "bottom-left");
  map.addControl(new maplibregl.NavigationControl(), "top-right");
  let marker = null, disposed = false;
  const write = point => {
    latitude.value = point.lat.toFixed(6);
    longitude.value = point.lng.toFixed(6);
    status.textContent = "Location pinned. Drag the pin to adjust; save the site to keep it.";
  };
  const pin = (point, update = true) => {
    if (!marker) {
      marker = new maplibregl.Marker({draggable: true, color: "#146b52"}).setLngLat(point).addTo(map);
      marker.on("dragend", () => write(marker.getLngLat()));
    } else marker.setLngLat(point);
    if (update) write(point);
  };
  if (hasPin) pin({lng: Number(initial.longitude), lat: Number(initial.latitude)}, false);
  map.on("click", event => pin(event.lngLat));
  const sync = () => {
    if (validCoordinates({latitude: latitude.value, longitude: longitude.value})) {
      pin({lng: Number(longitude.value), lat: Number(latitude.value)}, false);
    } else { marker?.remove(); marker = null; }
  };
  latitude.addEventListener("change", sync);
  longitude.addEventListener("change", sync);
  centerButton.onclick = () => pin(map.getCenter());
  clearButton.onclick = () => {
    marker?.remove(); marker = null; latitude.value = ""; longitude.value = "";
    status.textContent = "Location cleared. Click the map to place a pin.";
  };
  map.on("error", () => { if (!disposed) status.textContent = "Map unavailable. You can still enter coordinates below."; });
  const observer = new ResizeObserver(() => { if (!disposed) map.resize(); });
  observer.observe(container);
  return () => {
    disposed = true; observer.disconnect(); marker?.remove(); map.remove();
    latitude.removeEventListener("change", sync); longitude.removeEventListener("change", sync);
    centerButton.onclick = null; clearButton.onclick = null;
  };
}
