import { describe, expect, it } from "vitest";
import { setupStatus } from "./status";

const gw = (id: string, connected = false) => ({ device_id: id, management_connected: connected });
const none = new Map<string | undefined, string>();

describe("setupStatus", () => {
  it("starts at the site step when there is no site", () => {
    expect(setupStatus({ sites: 0, gateways: [], selected: "", states: none }).steps).toEqual([{ key: "site", state: "current" }, { key: "connect", state: "todo" }, { key: "hotspot", state: "todo" }, { key: "guest", state: "todo" }, { key: "install", state: "todo" }]);
  });
  it("asks to connect once a site exists, even with no router record", () => {
    expect(setupStatus({ sites: 1, gateways: [], selected: "", states: none }).steps.map((s) => s.state)).toEqual(["done", "current", "todo", "todo", "todo"]);
  });
  it("marks connect done only when management access is verified", () => {
    expect(setupStatus({ sites: 1, gateways: [gw("a", false)], selected: "", states: new Map([["a", "up"]]) }).steps[1].state).toBe("current");
    expect(setupStatus({ sites: 1, gateways: [gw("a", true)], selected: "", states: none }).steps[1].state).toBe("done");
  });
  it("keeps connected and live separate", () => {
    const s = setupStatus({ sites: 1, gateways: [gw("a", true)], selected: "a", states: new Map([["a", "offline"]]) });
    expect(s.steps[1].state).toBe("done");
    expect(s.live).toBe(false);
    expect(setupStatus({ sites: 1, gateways: [gw("a", true)], selected: "a", states: new Map([["a", "up"]]) }).live).toBe(true);
  });
  it("uses the remembered router only while it is still a gateway, else the single one", () => {
    expect(setupStatus({ sites: 1, gateways: [gw("a"), gw("b", true)], selected: "b", states: none }).router?.device_id).toBe("b");
    expect(setupStatus({ sites: 1, gateways: [gw("a")], selected: "gone", states: none }).router?.device_id).toBe("a");
    expect(setupStatus({ sites: 1, gateways: [gw("a"), gw("b")], selected: "gone", states: none }).router).toBeUndefined();
  });
  it("marks Hotspot done only from router-reported profiles", () => {
    const base = { sites: 1, gateways: [gw("a", true)], selected: "a", states: none };
    expect(setupStatus(base).steps[2].state).toBe("current");
    expect(setupStatus({ ...base, hotspotProfiles: 0 }).steps[2].state).toBe("current");
    expect(setupStatus({ ...base, hotspotProfiles: 1 }).steps[2].state).toBe("done");
    expect(setupStatus({ ...base, gateways: [gw("a", false)], hotspotProfiles: 1 }).steps[2].state).toBe("todo");
  });
  const ready = { sites: 1, gateways: [gw("a", true)], selected: "a", states: none, hotspotProfiles: 1 };
  it("needs a saved guest page, not the zero time", () => {
    expect(setupStatus(ready).steps[3].state).toBe("current");
    expect(setupStatus({ ...ready, portalSavedAt: "0001-01-01T00:00:00Z" }).steps[3].state).toBe("current");
    expect(setupStatus({ ...ready, portalSavedAt: "2026-11-06T01:00:00Z" }).steps[3].state).toBe("done");
    expect(setupStatus({ ...ready, hotspotProfiles: 0, portalSavedAt: "2026-11-06T01:00:00Z" }).steps[3].state).toBe("todo");
  });
  it("is installed only when a verified install is newer than the last save", () => {
    const saved = { ...ready, portalSavedAt: "2026-11-06T01:00:00Z" };
    expect(setupStatus(saved).steps[4].state).toBe("current");
    expect(setupStatus({ ...saved, portalInstalledAt: "2026-11-06T00:00:00Z" }).steps[4].state).toBe("current");
    expect(setupStatus({ ...saved, portalInstalledAt: "2026-11-06T02:00:00Z" }).steps[4].state).toBe("done");
    expect(setupStatus(ready).steps[4].state).toBe("todo");
  });
});
