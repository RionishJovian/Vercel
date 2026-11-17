import { findProductPhoto, productForName, type MikroTikProductPhoto } from "../../lib/mikrotikProducts";
import { portPanels, type PortPanel } from "../../lib/portPanels";

/**
 * Models the Magic Hub "Router network setup" one-paste can prepare, and what the script needs to
 * know about each one. This table is the only place a model is described: the dropdown lists
 * these rows and `newRouterNetwork.ts` generates the RouterOS script from the selected row.
 *
 * Adding a model (owner decision 2026-11-14, widened the same day: any router MikroTik documents
 * well enough, not only ones the owner holds):
 *  1. `boardModel` is exactly what `/system routerboard get model` prints, NOT always the product
 *     code: the retail suffix is dropped (RB5009UG+S+IN prints `RB5009UG+S+`, L009UiGS-RM prints
 *     `L009UiGS`, hAP ax2 `C52iG-5HaxD2HaxD-TC` prints `C52iG-5HaxD2HaxD`). Take it from a real
 *     `/system routerboard print` or an export header (`# model = ...`) posted on
 *     forum.mikrotik.com and link that post in `docs`. The script stops on any other board, so a
 *     wrong string only makes the setup refuse; it can never configure the wrong router.
 *  2. Confirm from MikroTik's default configuration for the model: the WAN port, the factory LAN
 *     bridge, and which radio name is which band (the order differs per model: hAP ax2 has
 *     wifi1 = 5 GHz, wAP ax has wifi1 = 2.4 GHz, the ac models have wlan1 = 2.4 GHz).
 *  3. Add one row below and add the model to the tests in `routerProfiles.test.ts`.
 * `evidence` says how far a row has been proved: "hardware" only after a real router ran the
 * script, otherwise "documented" (MikroTik pages plus forum printouts, not run on that model).
 * The model picker (`modelCatalogue.ts`) lists every catalogue model and offers setup only for the
 * models that have a row here; a row with no `radios` is a wired-only model (no Wi-Fi WAN options).
 * Photos and back-panel diagrams are not repeated here: `productId` is the model's id in
 * `lib/mikrotikProducts.ts` (display name, photo) and the same id in `lib/portPanels.ts` (port
 * diagram), reached through `assetsFor`. Adding a diagram for a model is done there, not here.
 * `wifiDriver` picks the script branch: "wifi" is the RouterOS 7 `/interface wifi` package
 * (wifi-qcom, wifi-mediatek), "wireless" is the legacy `/interface wireless` package that the
 * IPQ4018/4019 `ac` models ship with. A router that runs wifi-qcom-ac instead has no
 * `/interface wireless` menu, so the legacy script cannot even start and nothing is changed.
 */
export type WifiBand = "5ghz" | "2ghz";
export type WifiDriver = "wifi" | "wireless";

export interface RouterRadio {
  /** Factory interface name: `wifiN` under `/interface wifi`, or `wlanN` under `/interface wireless`. */
  name: string;
  band: WifiBand;
}

/** One physical port. Only facts MikroTik documents; ports a model has no row for are simply unknown. */
export interface RouterPort {
  /** Factory interface name, for example `ether1`. */
  name: string;
  kind: "ether" | "sfp";
  speed: "1G" | "2.5G" | "10G";
  poe?: readonly ("in" | "out")[];
}

export interface RouterProfile {
  id: string;
  /** Id of the same model in `mikrotikProducts` (and `portPanels` when a diagram exists). */
  productId: string;
  /** Plain ASCII name used inside the router script's own messages. */
  scriptName: string;
  /** Exactly what `/system routerboard get model` prints on this model. */
  boardModel: string;
  /** Every physical port, in factory order. */
  ports: readonly RouterPort[];
  /** Factory WAN Ethernet port (not in the LAN bridge, DHCP client, WAN interface list). */
  wan: string;
  /** Built-in Wi-Fi radios. Empty for a wired-only model: the picker then offers Ethernet WAN only. */
  radios: readonly RouterRadio[];
  /** Factory LAN: the bridge, the address on it and that address's subnet. */
  lan: { bridge: string; address: string; network: string };
  /** Which Wi-Fi package the radios belong to; selects the script branch. */
  wifiDriver: WifiDriver;
  /** "hardware": a real router of this model ran the script. "documented": MikroTik pages and forum printouts only. */
  evidence: "hardware" | "documented";
  /** MikroTik pages (and forum posts with a real printout) the layout above was checked against. */
  docs: readonly string[];
}

const ether = (n: number, speed: RouterPort["speed"] = "1G", poe?: RouterPort["poe"]): RouterPort =>
  poe ? { name: "ether" + n, kind: "ether", speed, poe } : { name: "ether" + n, kind: "ether", speed };
const sfp = (name: string, speed: RouterPort["speed"]): RouterPort => ({ name, kind: "sfp", speed });
/** ether<from>..ether<to>, all the same speed. */
const range = (from: number, to: number, speed: RouterPort["speed"] = "1G"): RouterPort[] =>
  Array.from({ length: to - from + 1 }, (_, i) => ether(from + i, speed));

const AP_ROUTER_LAN = { bridge: "bridge", address: "192.168.88.1/24", network: "192.168.88.0/24" } as const;
/** "AP Router" default configuration: first Ethernet port is the WAN, everything else is in `bridge`. */
const DEFAULTS = "https://manual.mikrotik.com/docs/getting-started/configuration-management/default-configurations/";
const forum = (path: string) => "https://forum.mikrotik.com/t/" + path;
/** The legacy `ac` layout every IPQ4018/4019 model shares: wlan1 is the 2.4 GHz radio, wlan2 the 5 GHz one. */
const LEGACY_RADIOS: readonly RouterRadio[] = [{ name: "wlan1", band: "2ghz" }, { name: "wlan2", band: "5ghz" }];

export const routerProfiles: readonly RouterProfile[] = [
  {
    id: "hap-ax2",
    productId: "hap-ax2",
    scriptName: "hAP ax2",
    boardModel: "C52iG-5HaxD2HaxD",
    // Five 1G ports; MikroTik lists PoE-out on Ether 1 (PoE-in is not recorded here until confirmed).
    ports: [ether(1, "1G", ["out"]), ...range(2, 5)],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "hardware",
    docs: ["https://help.mikrotik.com/docs/spaces/UM/pages/141197388/hAP+ax%C2%B2", DEFAULTS],
  },
  {
    id: "hap-ax3",
    productId: "hap-ax3",
    scriptName: "hAP ax3",
    boardModel: "C53UiG+5HPaxD2HPaxD",
    // ether1 is the 2.5G port with PoE-in/out; ether2-5 are 1G.
    ports: [ether(1, "2.5G", ["in", "out"]), ...range(2, 5)],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://help.mikrotik.com/docs/spaces/QG/pages/110362626/Quick+Guide+-+hAP+ax%C2%B3", "https://help.mikrotik.com/docs/spaces/UM/pages/150601820/hAP+ax%C2%B3", DEFAULTS],
  },
  {
    id: "hap-ax-lite",
    productId: "hap-ax-lite",
    scriptName: "hAP ax lite",
    boardModel: "L41G-2axD",
    // Four 1G ports and a single 2.4 GHz radio, so the model has no 5 GHz Wi-Fi WAN option.
    ports: range(1, 4),
    wan: "ether1",
    radios: [{ name: "wifi1", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/hap-ax-lite/", "https://mikrotik.com/product/hap_ax_lite", DEFAULTS],
  },
  {
    id: "hap-ax-s",
    productId: "hap-ax-s",
    scriptName: "hAP ax S",
    boardModel: "E62iUGS-2axD5axT",
    // Five 1G ports (PoE-out on ether5) and a 2.5G SFP. MediaTek radios: wifi1 is 2.4 GHz, wifi2 is 5 GHz.
    ports: [...range(1, 4), ether(5, "1G", ["out"]), sfp("sfp1", "2.5G")],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "2ghz" }, { name: "wifi2", band: "5ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/hap-ax-s/", forum("speed-cap-ax-vs-hap-ax-s/269648")],
  },
  {
    id: "chateau-pro-ax",
    productId: "chateau-pro-ax",
    scriptName: "Chateau PRO ax",
    boardModel: "H53UiG-5HaxQ2HaxQ",
    // No LTE modem: MikroTik's quickstart says to plug the ISP cable into the first Ethernet port and
    // open 192.168.88.1, i.e. the ordinary AP Router default (the LTE default belongs to the LTE Chateaus).
    ports: [...range(1, 4), ether(5, "1G", ["out"])],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/chateau-pro-ax/", forum("wifi-qcom-ac/269409"), DEFAULTS],
  },
  {
    id: "hap-ac3",
    productId: "hap-ac3",
    scriptName: "hAP ac3",
    boardModel: "RBD53iG-5HacD2HnD",
    ports: [ether(1, "1G", ["in"]), ...range(2, 5)],
    wan: "ether1",
    radios: LEGACY_RADIOS,
    lan: AP_ROUTER_LAN,
    wifiDriver: "wireless",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/hap-ac3/", "https://mikrotik.com/product/hap_ac3", forum("hap-ax3-interface-wireless-over-to-interface-wifi/182122")],
  },
  {
    id: "hap-ac2",
    productId: "hap-ac2",
    scriptName: "hAP ac2",
    boardModel: "RBD52G-5HacD2HnD",
    ports: range(1, 5),
    wan: "ether1",
    radios: LEGACY_RADIOS,
    lan: AP_ROUTER_LAN,
    wifiDriver: "wireless",
    evidence: "documented",
    docs: ["https://mikrotik.com/product/hap_ac2", forum("upgrading-rooterboot-factory-software/163694"), DEFAULTS],
  },
  {
    id: "rb5009",
    productId: "rb5009",
    scriptName: "RB5009UG+S+IN",
    // The retail "IN" suffix is not part of the printed model (forum printouts show `RB5009UG+S+`).
    boardModel: "RB5009UG+S+",
    ports: [ether(1, "2.5G", ["in"]), ...range(2, 8), sfp("sfp-sfpplus1", "10G")],
    wan: "ether1",
    radios: [],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://mikrotik.com/product/rb5009ug_s_in", forum("max-mtu-question/165604/71"), DEFAULTS],
  },
  {
    id: "rb4011",
    productId: "rb4011",
    scriptName: "RB4011iGS+RM",
    // The Wi-Fi variant (RB4011iGS+5HacQ2HnD-IN) is a different product with its own board string.
    boardModel: "RB4011iGS+",
    ports: [...range(1, 9), ether(10, "1G", ["out"]), sfp("sfp-sfpplus1", "10G")],
    wan: "ether1",
    radios: [],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://mikrotik.com/product/rb4011igs_rm", forum("new-capsman-client-works-on-hap-ac2-but-not-on-wap-ac/173530"), DEFAULTS],
  },
  {
    id: "l009",
    productId: "l009",
    scriptName: "L009UiGS-RM",
    // The 2.4 GHz variant (L009UiGS-2HaxD-IN) is a different product with its own board string.
    boardModel: "L009UiGS",
    ports: [ether(1, "1G", ["in"]), ...range(2, 7), ether(8, "1G", ["out"]), sfp("sfp1", "2.5G")],
    wan: "ether1",
    radios: [],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://mikrotik.com/product/l009uigs_rm", forum("strange-dhcp-problem-with-capsman-wifi-qcom-ac-involved/264228"), DEFAULTS],
  },
  {
    id: "hex-s",
    productId: "hex-s",
    scriptName: "hEX S",
    boardModel: "RB760iGS",
    ports: [...range(1, 4), ether(5, "1G", ["out"]), sfp("sfp1", "1G")],
    wan: "ether1",
    radios: [],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://mikrotik.com/product/hex_s", forum("how-can-you-view-the-mac-address-table-of-a-hw-offloaded-bridge-for-example-on-rb760igs-with-v7-19-6/266024"), DEFAULTS],
  },
  {
    id: "cap-ax",
    productId: "cap-ax",
    scriptName: "cAP ax",
    boardModel: "cAPGi-5HaxD2HaxD",
    // Default: ether1 is the DHCP-client WAN, ether2 (PoE-out) is bridged with the radios. wifi1 is 5 GHz here.
    ports: [ether(1), ether(2, "1G", ["out"])],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/cap-ax/", forum("speed-cap-ax-vs-hap-ax-s/269648"), DEFAULTS],
  },
  {
    id: "wap-ax",
    productId: "wap-ax",
    scriptName: "wAP ax",
    boardModel: "wAPG-5HaxD2HaxD",
    // wifi1 is 2.4 GHz and wifi2 is 5 GHz on this model (the reverse of the hAP/cAP ax boards).
    ports: [ether(1, "1G", ["in"]), ether(2)],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "2ghz" }, { name: "wifi2", band: "5ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/wap-ax/", forum("is-it-me-or-is-it-the-wap-ax/184004"), DEFAULTS],
  },
  {
    id: "cap-ac",
    productId: "cap-ac",
    scriptName: "cAP ac",
    boardModel: "RBcAPGi-5acD2nD",
    ports: [ether(1, "1G", ["in"]), ether(2, "1G", ["out"])],
    wan: "ether1",
    radios: LEGACY_RADIOS,
    lan: AP_ROUTER_LAN,
    wifiDriver: "wireless",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/cap-ac/", "https://mikrotik.com/product/cap_ac", forum("hotspot-and-cap-ac-with-rb4011/137221")],
  },
  {
    id: "wap-ac",
    productId: "wap-ac",
    scriptName: "wAP ac",
    boardModel: "RBwAPG-5HacD2HnD",
    ports: [ether(1), ether(2)],
    wan: "ether1",
    radios: LEGACY_RADIOS,
    lan: AP_ROUTER_LAN,
    wifiDriver: "wireless",
    evidence: "documented",
    docs: ["https://manual.mikrotik.com/hardware/wap-ac/", forum("unreliable-2-4ghz-on-wap-ac/271933")],
  },
];

export const findProfile = (id: string): RouterProfile | undefined => routerProfiles.find(p => p.id === id);

/** The radio of the given band, or undefined when the model has none. */
export const radioFor = (profile: RouterProfile, band: WifiBand): RouterRadio | undefined => profile.radios.find(r => r.band === band);

/** The model's display name, from the shared product catalogue. */
export const profileName = (profile: RouterProfile): string => findProductPhoto(profile.productId)?.name ?? profile.scriptName;

/**
 * The one lookup from a router record to its setup profile. The board string wins and must match
 * exactly; the owner's typed model name is the fallback and uses the catalogue's strict matching.
 */
export function profileFor(board?: string, hardwareName?: string): RouterProfile | undefined {
  const code = (board ?? "").trim();
  const byBoard = code ? routerProfiles.find(p => p.boardModel === code) : undefined;
  if (byBoard) return byBoard;
  const id = productForName(hardwareName)?.id;
  return id ? routerProfiles.find(p => p.productId === id) : undefined;
}

/** Photo and back-panel diagram for a profile, when the shared catalogues have them. Never builds new ones. */
export const assetsFor = (profile: RouterProfile): { photo?: MikroTikProductPhoto; panel?: PortPanel } => ({
  photo: findProductPhoto(profile.productId),
  panel: portPanels.find(p => p.id === profile.productId),
});
