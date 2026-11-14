import { describe, expect, it } from "vitest";
import golden from "./__fixtures__/ax2-network-golden.json";
import { isWifiUplink, withNewRouterNetwork, type Uplink } from "./newRouterNetwork";
import { findProfile, routerProfiles, type RouterProfile } from "./routerProfiles";

const BLOCK = "{\n:put \"probe\"\n}\n";
const SSID = 'Fixture "$&"', PASS = "test-password";
const ax2 = findProfile("hap-ax2")!, ax3 = findProfile("hap-ax3")!;
const make = (p: RouterProfile, u: Uplink) => withNewRouterNetwork(BLOCK, p, u, SSID, PASS);
const GOLDEN = golden as Record<Uplink, string>;

describe("hAP ax2 network setup", () => {
  // The golden scripts are what the ax2-only builder produced before models became a table.
  it.each(["ethernet", "5ghz", "2ghz"] as const)("is identical to the script the ax2-only builder produced (%s)", uplink => {
    expect(make(ax2, uplink)).toBe(GOLDEN[uplink]);
  });
});

describe("model check", () => {
  it.each(routerProfiles.map(p => [p.id, p] as const))("%s: the board check is the first command and stops before anything changes", (_id, p) => {
    for (const uplink of ["ethernet", ...p.radios.map(r => r.band)] as const) {
      const script = make(p, uplink);
      const check = `:if ([/system routerboard get model] != "${p.boardModel}") do={ :error "STOP: this network setup requires a factory-configured ${p.scriptName}" }`;
      const at = script.indexOf(check);
      expect(at).toBeGreaterThan(-1);
      // Nothing that reads or changes the router's configuration comes before the check.
      const before = script.slice(0, at).split("\n").filter(l => l.trim() !== "" && !l.startsWith("#") && l.trim() !== "{");
      expect(before).toEqual([]);
      // No state-changing command is allowed before the first model check either.
      expect(script.slice(0, at)).not.toMatch(/\/(interface|ip|system)\b.*\b(set|add|disable|remove)\b/);
      // The Hub block itself is still the one that follows, untouched.
      expect(script.startsWith("{")).toBe(true);
      expect(script.endsWith(BLOCK.slice(1))).toBe(true);
      expect(script.indexOf("Continuing with Magic Hub")).toBeLessThan(script.indexOf(':put "probe"'));
    }
  });
  it("the ax3 script checks the ax3 board and never the ax2 board", () => {
    const script = make(ax3, "ethernet");
    expect(script).toContain('!= "C53UiG+5HPaxD2HPaxD"');
    expect(script).not.toContain("C52iG-5HaxD2HaxD");
    expect(script).toContain("# New hAP ax3 with factory defaults.");
    expect(make(ax2, "ethernet")).not.toContain("C53UiG+5HPaxD2HPaxD");
  });
});

describe("legacy wireless (ac models)", () => {
  const ac2 = findProfile("hap-ac2")!;
  it.each([["5ghz", "wlan2"], ["2ghz", "wlan1"]] as const)("%s uses %s as a station through its own security profile", (band, radio) => {
    const script = make(ac2, band);
    expect(script).toContain(`:local mmUplink "${radio}"`);
    expect(script).toContain(":local mmRadio [/interface wireless find where default-name=$mmUplink and name=$mmUplink]");
    expect(script).toContain('/interface wireless security-profiles add name=mm-wifi-wan mode=dynamic-keys authentication-types=wpa2-psk wpa2-pre-shared-key="test-password"');
    expect(script).toContain('/interface wireless set $mmRadio mode=station ssid="Fixture \\"\\$&\\"" security-profile=mm-wifi-wan disabled=no');
    expect(script).toContain("/interface bridge port disable $mmBridgePort");
    expect(script).toContain("default-route-distance=2");
  });
  it("never uses the RouterOS 7 wifi menu, and never edits the shared default security profile", () => {
    const script = make(ac2, "5ghz");
    expect(script).not.toContain("/interface wifi");
    expect(script).not.toContain("security-profiles set");
    expect(script).not.toContain("configuration.mode");
  });
  it("refuses to run when the Wi-Fi profile name is already taken, before changing anything", () => {
    const script = make(ac2, "5ghz");
    const guard = script.indexOf('[/interface wireless security-profiles find where name="mm-wifi-wan"]');
    expect(guard).toBeGreaterThan(-1);
    expect(guard).toBeLessThan(script.indexOf("/interface bridge port disable"));
    expect(guard).toBeLessThan(script.indexOf("/interface wireless security-profiles add"));
  });
  it("leaves the Ethernet script untouched for an ac model", () => {
    const eth = make(ac2, "ethernet");
    expect(eth).toContain(':local mmUplink "ether1"');
    expect(eth).not.toContain("/interface wireless");
  });
});

describe("wired models", () => {
  it.each(["rb5009", "rb4011", "l009", "hex-s"])("%s only offers the Ethernet script and refuses Wi-Fi", id => {
    const p = findProfile(id)!;
    expect(make(p, "ethernet")).toContain(':local mmUplink "ether1"');
    expect(() => make(p, "5ghz")).toThrow(/no 5ghz radio/);
    expect(() => make(p, "2ghz")).toThrow(/no 2ghz radio/);
  });
});

describe("generated from the profile", () => {
  it("uses the profile's WAN port and radios", () => {
    const profile: RouterProfile = { ...ax3, id: "test-model", wan: "ether3", radios: [{ name: "wifi7", band: "5ghz" }, { name: "wifi8", band: "2ghz" }], lan: { bridge: "bridge-lan", address: "10.9.0.1/24", network: "10.9.0.0/24" } };
    const ethernet = make(profile, "ethernet");
    expect(ethernet).toContain(':local mmUplink "ether3"');
    expect(ethernet).toContain("interface=bridge-lan and address=\"10.9.0.1/24\"");
    expect(ethernet).toContain("in 10.9.0.0/24");
    expect(ethernet).not.toContain("192.168.88");
    const wifi = make(profile, "2ghz");
    expect(wifi).toContain(':local mmUplink "wifi8"');
    expect(wifi).toContain("bridge=bridge-lan");
    expect(wifi).not.toContain("ether3");
  });
  it.each(routerProfiles.map(p => [p.id, p] as const))("%s: each Wi-Fi band becomes the WAN station radio; ethernet uses the WAN port", (_id, p) => {
    for (const r of p.radios) {
      const script = make(p, r.band);
      expect(script).toContain(`:local mmUplink "${r.name}"`);
      expect(script).toContain(p.wifiDriver === "wifi" ? "/interface wifi set $mmRadio configuration.mode=station" : "/interface wireless set $mmRadio mode=station");
      expect(script).toContain("default-route-distance=2");
      // Every check runs before the first change, so a refused check leaves the router untouched.
      expect(script.indexOf("STOP: an internet route is already active")).toBeLessThan(script.indexOf("/interface bridge port disable"));
      expect(script.indexOf("STOP: Wi-Fi interface already configured or missing")).toBeLessThan(script.indexOf("/interface bridge port disable"));
    }
    const eth = make(p, "ethernet");
    expect(eth).toContain(`:local mmUplink "${p.wan}"`);
    expect(eth).not.toContain("mode=station");
    expect(eth).toContain("default-route-distance=1");
  });
  it("refuses a band the model does not have", () => {
    const single: RouterProfile = { ...ax3, radios: [{ name: "wifi1", band: "2ghz" }] };
    expect(() => make(single, "5ghz")).toThrow(/no 5ghz radio/);
    expect(() => make(single, "2ghz")).not.toThrow();
  });
  it("quotes the Wi-Fi name and password and rejects unusable ones", () => {
    expect(make(ax3, "5ghz")).toContain('ssid="Fixture \\"\\$&\\""');
    expect(() => withNewRouterNetwork(BLOCK, ax3, "5ghz", "", PASS)).toThrow();
    expect(() => withNewRouterNetwork(BLOCK, ax3, "5ghz", SSID, "short")).toThrow();
    expect(() => withNewRouterNetwork("not a block", ax3, "ethernet", "", "")).toThrow(/script block/);
  });
  it("tells Wi-Fi uplinks from Ethernet", () => {
    expect(isWifiUplink("ethernet")).toBe(false);
    expect(isWifiUplink("5ghz") && isWifiUplink("2ghz")).toBe(true);
  });
});
