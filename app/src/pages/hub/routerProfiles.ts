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
 * A model whose Wi-Fi uses the legacy `/interface wireless` driver (RouterOS 6 style) needs a new
 * generator branch first; every row today uses the RouterOS 7 `wifi` package.
 */
export type WifiBand = "5ghz" | "2ghz";

export interface RouterRadio {
  /** Factory interface name under `/interface wifi`, for example `wifi1`. */
  name: string;
  band: WifiBand;
}

export interface RouterProfile {
  id: string;
  /** Shown in the dropdown. */
  name: string;
  /** Plain ASCII name used inside the router script's own messages. */
  scriptName: string;
  /** Exactly what `/system routerboard get model` prints on this model. */
  boardModel: string;
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

const AP_ROUTER_LAN = { bridge: "bridge", address: "192.168.88.1/24", network: "192.168.88.0/24" } as const;

export const routerProfiles: readonly RouterProfile[] = [
  {
    id: "hap-ax2",
    name: "hAP ax²",
    scriptName: "hAP ax2",
    boardModel: "C52iG-5HaxD2HaxD",
    wan: "ether1",
    radios: [{ name: "wifi1", band: "5ghz" }, { name: "wifi2", band: "2ghz" }],
    lan: AP_ROUTER_LAN,
    wifiDriver: "wifi",
    docs: ["https://help.mikrotik.com/docs/spaces/UM/pages/141197388/hAP+ax%C2%B2", "https://manual.mikrotik.com/docs/getting-started/configuration-management/default-configurations/"],
  },
  {
    id: "hap-ax3",
    name: "hAP ax³",
    scriptName: "hAP ax3",
    boardModel: "C53UiG+5HPaxD2HPaxD",
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
