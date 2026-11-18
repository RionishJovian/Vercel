import { describe, expect, it } from "vitest";
import { isWifiUplink, MARK, withNewRouterNetwork, type Uplink } from "./newRouterNetwork";
import { findProfile, routerProfiles, type RouterProfile } from "./routerProfiles";

// A Magic Hub block as the server sends it: read-only checks, then the apply boundary, then the writes.
const BLOCK = "{\n# PREFLIGHT: checks\n:put \"check\"\n# APPLY: tunnel\n:put \"probe\"\n}\n";
const SSID = 'Fixture "$&"', PASS = "test-password";
const ax2 = findProfile("hap-ax2")!, ax3 = findProfile("hap-ax3")!;
const make = (p: RouterProfile, u: Uplink) => withNewRouterNetwork(BLOCK, p, u, SSID, PASS);

describe("hAP ax2 network setup", () => {
  it.each(["ethernet", "5ghz", "2ghz"] as const)("checks first, changes only at the Magic Hub boundary (%s)", uplink => {
    const script = make(ax2, uplink);
    const board = script.indexOf("/system routerboard get model");
    const hubCheck = script.indexOf(':put "check"');
    const firstWrite = script.search(/\/interface (bridge port set|wifi set)|\/ip dhcp-client (add|set)/);
    const boundary = script.indexOf("# APPLY: tunnel");
    expect(board).toBeGreaterThan(-1);
    expect(board).toBeLessThan(hubCheck);          // the model check comes before everything else
    expect(hubCheck).toBeLessThan(firstWrite);     // network changes come after every Magic Hub check
    expect(firstWrite).toBeLessThan(boundary);     // and before the first Magic Hub write
    expect(script.endsWith('# APPLY: tunnel\n:put "probe"\n}\n')).toBe(true);
  });
  it("needs the Magic Hub boundary to place the changes", () => {
    expect(() => withNewRouterNetwork("{\n:put \"x\"\n}", ax2, "ethernet", "", "")).toThrow(/boundary/);
  });
  it("only a Magic Hub tunnel from an earlier attempt may already exist, and a hotspot may not", () => {
    const script = make(ax2, "ethernet");
    expect(script).toContain('[:find [/interface wireguard get $mmWg comment] "mega-magic:mmh-"] != 0');
    expect(script).toContain("[:len [/ip hotspot find]] != 0");
  });
});

describe("time sync", () => {
  it.each(["ethernet", "5ghz", "2ghz"] as const)("turns the time client on after the WAN is proven, only if it is off (%s)", uplink => {
    const script = make(ax2, uplink);
    const routeOk = script.indexOf('Continuing with Magic Hub."');
    const ntp = script.indexOf("/system ntp client set enabled=yes servers=pool.ntp.org");
    const tz = script.indexOf("time-zone-name=Asia/Yangon");
    expect(routeOk).toBeGreaterThan(-1);
    expect(ntp).toBeGreaterThan(routeOk);              // after the default route is proven
    expect(tz).toBeGreaterThan(routeOk);
    expect(ntp).toBeLessThan(script.indexOf("# APPLY: tunnel")); // and before the Hub's own writes
    expect(script).toContain(":if (![/system ntp client get enabled])");       // an existing NTP setup is left alone
    // A factory router reports its zone as "manual" (seen on a real hAP ax2, 7.25.1) or "UTC"; a zone someone chose is left alone.
    expect(script).toContain(':if ([/system clock get time-zone-name] = "UTC" || [/system clock get time-zone-name] = "manual")');
    expect(script).toContain("on-error="); // a failure here must not stop the Magic Hub setup
  });
});

describe("Wi-Fi WAN can resume a cut-off paste", () => {
  it.each([["hap-ax2", "5ghz"], ["hap-ac2", "5ghz"]] as const)("%s %s marks what it creates and only continues its own work", (id, band) => {
    const script = make(findProfile(id)!, band);
    expect(script).toContain(`:local mmMark "${MARK}"`);
    expect(script).toContain(":if (!$mmPortOurs) do={ /interface bridge port set $mmBridgePort disabled=yes comment=$mmMark }");
    expect(script).toContain(":if ([:len $mmMember] = 0) do={ /interface list member add list=WAN interface=$mmUplink comment=$mmMark }");
    expect(script).toContain(":if ([:len $mmClient] = 0) do={ /ip dhcp-client add");
    expect(script).toContain("comment=$mmMark }\n:set mmClient");
    // Anything that is neither factory state nor marked stops the script before a change.
    for (const stop of ["bridge port was changed outside this setup", "interface list was changed outside this setup", "DHCP client was changed outside this setup"]) {
      expect(script.indexOf(stop)).toBeGreaterThan(-1);
      expect(script.indexOf(stop)).toBeLessThan(script.indexOf("# APPLY (Wi-Fi WAN)"));
    }
  });
  it("a re-paste of the Ethernet script has nothing to resume and stays a single enable", () => {
    const eth = make(ax2, "ethernet");
    expect(eth).not.toContain("$mmMark");
    expect(eth).toContain("/ip dhcp-client set $mmClient disabled=no");
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
      // The Hub block keeps its own checks first and its own writes last, untouched.
      expect(script.startsWith("{")).toBe(true);
      expect(script.indexOf(':put "check"')).toBeGreaterThan(at);
      expect(script.endsWith('# APPLY: tunnel\n:put "probe"\n}\n')).toBe(true);
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
    expect(script).toContain("/interface bridge port set $mmBridgePort disabled=yes comment=$mmMark");
    expect(script).toContain("default-route-distance=2");
  });
  it("never uses the RouterOS 7 wifi menu, and never edits the shared default security profile", () => {
    const script = make(ac2, "5ghz");
    expect(script).not.toContain("/interface wifi");
    expect(script).not.toMatch(/security-profiles set (default|\[find default)/);
    expect(script).toContain("security-profiles set $mmProfile wpa2-pre-shared-key=");
    expect(script).not.toContain("configuration.mode");
  });
  it("refuses a Wi-Fi profile named mm-wifi-wan that it did not create, before changing anything", () => {
    const script = make(ac2, "5ghz");
    const guard = script.indexOf('[/interface wireless security-profiles find where name="mm-wifi-wan"]');
    expect(guard).toBeGreaterThan(-1);
    expect(script).toContain("Wi-Fi security profile mm-wifi-wan was created outside this setup");
    expect(guard).toBeLessThan(script.indexOf("/interface bridge port set"));
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
      expect(script.indexOf("STOP: an internet route is already active")).toBeLessThan(script.indexOf("/interface bridge port set"));
      expect(script.indexOf("STOP: Wi-Fi interface missing or already configured")).toBeLessThan(script.indexOf("/interface bridge port set"));
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
