import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const source = fs.readFileSync(new URL("../web/assets/js/site-map.js", import.meta.url), "utf8").replace(/^import .*;\n/, "").replaceAll("export function", "function");
const controls = Object.fromEntries(["#site-map-status", "#site-map-2d", "#site-map-3d", "#site-map-fit"].map(id => [id, {setAttribute(key, value) {this[key] = value;}}]));
let map, selected, markerCount = 0, lastMarker, mapCount = 0;
class FakeMap {
  constructor(options) { mapCount++; map = this; this.options = options; this.events = {}; this.layers = []; this.controls = []; }
  addControl(c) { this.controls.push(c); }
  on(event, callback) {this.events[event] = callback;}
  getStyle() {return {layers: [{id: "labels", type: "symbol", layout: {"text-field": "name"}}]};}
  addSource(id, options) {this.source = options;}
  addLayer(layer) {this.layers.push(layer);}
  setLayoutProperty(id, property, value) {this.visibility = value;}
  easeTo(options) {this.camera = options;}
  fitBounds() {this.fitted = true;}
  resize() {}
  getCenter() {return {lat: 21.9, lng: 95.9};}
  remove() {this.removed = true;}
}
class FakeMarker {
  constructor(options) { this.element = options.element; this.options = options; this.events = {}; lastMarker = this; markerCount++; }
  setLngLat(coords) { this.coords = coords; return this; }
  addTo() { return this; }
  on(event, callback) {this.events[event] = callback;}
  getLngLat() {return this.coords;}
  remove() {}
}
const context = vm.createContext({maplibregl: {setWorkerUrl() {}, Map: FakeMap, Marker: FakeMarker,
  AttributionControl: class {constructor(options) {this.options = options;}}, NavigationControl: class {}, LngLatBounds: class {extend() {}}},
  localStorage: {getItem() {return "";}}, matchMedia: () => ({matches: false}),
  ResizeObserver: class {observe() {} disconnect() {}},
  document: {createElement() {return {setAttribute() {}, append() {}, addEventListener(e, fn) {this.click = fn;}};}}});
vm.runInContext(source, context);
for (const [lat, lng, valid] of [[null,null,false],[undefined,undefined,false],["","",false],[0,0,true],[16.8,96.1,true],[91,10,false],[10,181,false],["x",10,false]]) {
  assert.equal(context.validCoordinates({latitude:lat,longitude:lng}), valid);
}
const container = {parentElement: {querySelector: id => controls[id]}};
const dispose = context.mountSiteMap(container, [{site_id:"one",name:"<img onerror=alert(1)>",latitude:16.8,longitude:96.1},{site_id:"missing",latitude:null,longitude:null}], [], id => selected = id);
assert.equal(markerCount,1, "missing coordinates must not become 0,0 markers");
map.events.load();
assert.ok(map.controls[0].options.customAttribution.includes("OpenStreetMap"));
assert.equal(map.controls[0].options.compact,false);
controls["#site-map-3d"].onclick();
assert.equal(map.visibility,"visible"); assert.equal(map.camera.pitch,55);
controls["#site-map-2d"].onclick(); assert.equal(map.visibility,"none"); assert.equal(map.camera.pitch,0);
dispose(); assert.equal(map.removed,true);
const disposeMain = context.mountSiteMap(container, [], [], () => {}); map.events.load();
assert.equal(map.options.zoom,4.8); assert.deepEqual(Array.from(map.options.center),[95.9,21.9]);
assert.ok(controls["#site-map-status"].textContent.includes("Myanmar map ready"));
const input = value => ({value, addEventListener() {}, removeEventListener() {}});
const latitude = input(""), longitude = input(""), status = {}, center = {}, clear = {};
const beforePicking = mapCount;
const stopPicking = disposeMain.beginPicking(latitude, longitude, status, center, clear);
assert.equal(mapCount, beforePicking, "editing must reuse the main map, not construct another map");
assert.equal(latitude.value, "", "entering edit mode must not silently select a location");
map.events.click({lngLat:{lat:16.8409,lng:96.1735}});
assert.equal(latitude.value, "16.840900");
assert.equal(lastMarker.options.draggable, true);
lastMarker.coords = {lat:17.2,lng:95.8}; lastMarker.events.dragend();
assert.equal(latitude.value, "17.200000");
clear.onclick(); assert.equal(latitude.value, "");
stopPicking(); assert.equal(center.onclick, null);
map.events.click({lngLat:{lat:20,lng:96}});
assert.equal(latitude.value, "", "cancelled picking must ignore map clicks");
disposeMain(); assert.equal(map.removed, true);
const removePicker = context.mountLocationPicker(container, latitude, longitude, status, center, clear);
assert.equal(latitude.value, "", "default viewport must not silently choose a location");
map.events.click({lngLat:{lat:16.8409,lng:96.1735}});
assert.equal(latitude.value,"16.840900"); assert.equal(longitude.value,"96.173500");
assert.equal(lastMarker.options.draggable,true);
lastMarker.coords = {lat:17.2,lng:95.8}; lastMarker.events.dragend();
assert.equal(latitude.value,"17.200000"); assert.equal(longitude.value,"95.800000");
center.onclick(); assert.equal(latitude.value,"21.900000");
clear.onclick(); assert.equal(latitude.value,""); assert.equal(longitude.value,"");
removePicker(); assert.equal(map.removed,true); assert.equal(center.onclick,null);
console.log("Map tests passed: Myanmar default, click/drag pin, clear, coordinates, attribution, 2D/3D, cleanup.");
