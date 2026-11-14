import { findProductPhoto, productForName, type MikroTikProductPhoto } from "../../lib/mikrotikProducts";
import { portPanels, type PortPanel } from "../../lib/portPanels";

/**
 * Models the Magic Hub "Router network setup" one-paste can prepare, and what the script needs to
 * know about each one. This table is the only place a model is described: the dropdown lists
 * these rows and `newRouterNetwork.ts` generates the RouterOS script from the selected row.
 *
 * Adding a model (owner decision 2026-11-14: only built-in Wi-Fi models, picked from a list):
 *  1. Take `boardModel` from `/system routerboard get model` on a real router of that model, or
 *     from MikroTik's own documentation. Never guess it: the script stops on any other board.
 *  2. Confirm from MikroTik's default configuration for the model: the WAN port, the factory LAN
 *     bridge, which `wifiN` radio is 5 GHz and which is 2.4 GHz.
 *  3. Add one row below and add the model to the tests in `routerProfiles.test.ts`.
 * Photos and back-panel diagrams are not repeated here: `productId` is the model's id in
 * `lib/mikrotikProducts.ts` (display name, photo) and the same id in `lib/portPanels.ts` (port
 * diagram), reached through `assetsFor`. Adding a diagram for a model is done there, not here.
 * A model whose Wi-Fi uses the legacy `/interface wireless` driver (RouterOS 6 style) needs a new
 * generator branch first; every row today uses the RouterOS 7 `wifi` package.
 */
export type WifiBand = "5ghz" | "2ghz";

export interface RouterRadio {
  /** Factory interface name under `/interface wifi`, for example `wifi1`. */
  name: string;
  band: WifiBand;
}

/** One physical port. Only facts MikroTik documents; ports a model has no row for are simply unknown. */
export interface RouterPort {
  /** Factory interface name, for example `ether1`. */
  name: string;
  kind: "ether" | "sfp";
  speed: "1G" | "2.5G";
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
  radios: readonly RouterRadio[];
  /** Factory LAN: the bridge, the address on it and that address's subnet. */
  lan: { bridge: string; address: string; network: string };
  /** `wifi` is the RouterOS 7 package (`/interface wifi`). */
  wifiDriver: "wifi";
  /** MikroTik pages the layout above was checked against. */
  docs: readonly string[];
}

const ether = (n: number, speed: RouterPort["speed"]): RouterPort => ({ name: "ether" + n, kind: "ether", speed });
const AP_ROUTER_LAN = { bridge: "bridge", address: "192.168.88.1/24", network: "192.168.88.0/24" } as const;

export const routerProfiles: readonly RouterProfile[] = [
  {
    id: "hap-ax2",
    productId: "hap-ax2",
    scriptName: "hAP ax2",
    boardModel: "C52iG-5HaxD2HaxD",
    // Five 1G ports; MikroTik lists PoE-out on Ether 1 (PoE-in is not recorded here until confirmed).
    ports: [{ name: "ether1", kind: "ether", speed: "1G", poe: ["out"] }, ...[2, 3, 4, 5].map(n => ether(n, "1G"))],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    docs: ["https://help.mikrotik.com/docs/spaces/UM/pages/141197388/hAP+ax%C2%B2", "https://manual.mikrotik.com/docs/getting-started/configuration-management/default-configurations/"],
  },
  {
    id: "hap-ax3",
    productId: "hap-ax3",
    scriptName: "hAP ax3",
    boardModel: "C53UiG+5HPaxD2HPaxD",
    // ether1 is the 2.5G port with PoE-in/out; ether2-5 are 1G.
    ports: [{ name: "ether1", kind: "ether", speed: "2.5G", poe: ["in", "out"] }, ...[2, 3, 4, 5].map(n => ether(n, "1G"))],
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    docs: ["https://help.mikrotik.com/docs/spaces/QG/pages/110362626/Quick+Guide+-+hAP+ax%C2%B3", "https://help.mikrotik.com/docs/spaces/UM/pages/150601820/hAP+ax%C2%B3", "https://manual.mikrotik.com/docs/getting-started/configuration-management/default-configurations/"],
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
