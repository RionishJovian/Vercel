import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Map as MapLibre } from "maplibre-gl";
import { I18nProvider } from "../i18n";
import StarlinkLayer from "./StarlinkLayer";

class FakeWorker {
  static all: FakeWorker[] = [];
  posts: unknown[] = []; terminated = false; onmessage: ((e: { data: unknown }) => void) | null = null; onerror: (() => void) | null = null;
  constructor() { FakeWorker.all.push(this); }
  postMessage(m: unknown) { this.posts.push(m); }
  terminate() { this.terminated = true; }
}

function fakeMap() {
  const sources: Record<string, { setData: ReturnType<typeof vi.fn> }> = {};
  const map = {
    getCenter: () => ({ lng: 96, lat: 17 }), getZoom: () => 13, fitBounds: vi.fn(), easeTo: vi.fn(),
    addSource: vi.fn((id: string) => { sources[id] = { setData: vi.fn() }; }), addLayer: vi.fn(), addImage: vi.fn(), removeImage: vi.fn(),
    removeLayer: vi.fn(), removeSource: vi.fn((id: string) => { delete sources[id]; }), getSource: (id: string) => sources[id],
  };
  return { map, sources };
}
const drawn = (s: { setData: ReturnType<typeof vi.fn> }) => s.setData.mock.lastCall?.[0]?.features ?? [];

function setup(reduced = false) {
  localStorage.setItem("mm2_lang", "en");
  vi.stubGlobal("Worker", FakeWorker);
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
  vi.stubGlobal("matchMedia", () => ({ matches: reduced }));
  const { map, sources } = fakeMap(), onActive = vi.fn();
  render(<I18nProvider><StarlinkLayer map={map as unknown as MapLibre} onActive={onActive} /></I18nProvider>);
  return { map, sources, onActive };
}
const toggle = () => fireEvent.click(screen.getByRole("button", { name: /Starlink/ }));
const send = (w: FakeWorker, data: unknown) => act(() => w.onmessage?.({ data }));

afterEach(() => { cleanup(); FakeWorker.all = []; vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("StarlinkLayer", () => {
  it("is off by default and starts nothing", () => {
    const { map } = setup();
    expect(screen.getByRole("button", { name: /Starlink/ }).getAttribute("aria-pressed")).toBe("false");
    expect(FakeWorker.all).toHaveLength(0);
    expect(map.addSource).not.toHaveBeenCalled();
  });
  it("draws calculated positions with the count, age, honest label and source", () => {
    const { sources, map } = setup();
    toggle();
    expect(map.fitBounds).toHaveBeenCalledOnce();
    send(FakeWorker.all[0], { state: "ready", ageHours: 22.6, coords: new Float32Array([96, 21, 96, 20, 97, 21, 97, 21]) });
    expect(screen.getByText("2 Starlink satellites calculated over the Myanmar area.")).toBeTruthy();
    expect(screen.getByText("Orbit data about 23 hours old.")).toBeTruthy();
    expect(screen.getByText(/not live signals from SpaceX/).textContent).toMatch(/don't show Starlink coverage/);
    expect((screen.getByRole("link", { name: /CelesTrak/ }) as HTMLAnchorElement).href).toBe("https://celestrak.org/");
    const icons = drawn(sources["starlink-icons"]);
    expect(icons.map((f: { geometry: { coordinates: number[] } }) => f.geometry.coordinates)).toEqual([[96, 21], [97, 21]]);
    expect(icons[0].properties.heading).toBeCloseTo(0);
    // The second satellite has no earlier position, so it gets no trail. The trail runs towards the satellite, for the fade.
    expect(drawn(sources["starlink-trails"]).map((f: { geometry: { coordinates: number[][] } }) => f.geometry.coordinates)).toEqual([[[96, 20], [96, 21]]]);
  });
  it.each([["stale", /out of date/], ["unavailable", /isn't available/]])("draws nothing when the data is %s, and says so", (state, text) => {
    const { sources } = setup();
    toggle();
    send(FakeWorker.all[0], { state: "ready", ageHours: 5, coords: new Float32Array([96, 21, 96, 20]) });
    send(FakeWorker.all[0], { state });
    expect(screen.getByRole("status").textContent).toMatch(text);
    expect(drawn(sources["starlink-icons"])).toEqual([]);
    expect(drawn(sources["starlink-trails"])).toEqual([]);
    expect(drawn(sources["starlink-area"])).toEqual([]);
    expect(screen.queryByText(/calculated over/)).toBeNull();
  });
  it("draws nothing if the worker itself fails", () => {
    const { sources } = setup();
    toggle();
    act(() => FakeWorker.all[0].onerror?.());
    expect(screen.getByRole("status").textContent).toMatch(/isn't available/);
    expect(drawn(sources["starlink-icons"])).toEqual([]);
  });
  it("pauses while the page is hidden and resumes after", () => {
    setup();
    toggle();
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    act(() => { document.dispatchEvent(new Event("visibilitychange")); });
    hidden.mockReturnValue(false);
    act(() => { document.dispatchEvent(new Event("visibilitychange")); });
    hidden.mockRestore();
    expect(FakeWorker.all[0].posts).toEqual([{ type: "start", intervalMs: 1000 }, { type: "stop" }, { type: "start", intervalMs: 1000 }]);
  });
  it("updates rarely under reduced motion", () => {
    setup(true);
    toggle();
    expect(FakeWorker.all[0].posts).toEqual([{ type: "start", intervalMs: 30000 }]);
  });
  it("cleans up and returns the map to where it was when turned off", () => {
    const { map, onActive } = setup();
    toggle();
    expect(onActive).toHaveBeenLastCalledWith(true);
    toggle();
    expect(FakeWorker.all[0].terminated).toBe(true);
    expect(map.removeSource).toHaveBeenCalledTimes(3);
    expect(map.removeImage).toHaveBeenCalledOnce();
    expect(onActive).toHaveBeenLastCalledWith(false);
    expect(map.easeTo).toHaveBeenCalledWith(expect.objectContaining({ zoom: 13 }));
  });
});
