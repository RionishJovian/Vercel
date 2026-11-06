import { describe, expect, it } from "vitest";
import { setupStatus } from "./status";

const gw = (id: string, connected = false) => ({ device_id: id, management_connected: connected });
const none = new Map<string | undefined, string>();

describe("setupStatus", () => {
  it("starts at the site step when there is no site", () => {
    expect(setupStatus({ sites: 0, gateways: [], selected: "", states: none }).steps).toEqual([{ key: "site", state: "current" }, { key: "connect", state: "todo" }]);
  });
  it("asks to connect once a site exists, even with no router record", () => {
    expect(setupStatus({ sites: 1, gateways: [], selected: "", states: none }).steps.map((s) => s.state)).toEqual(["done", "current"]);
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
});
