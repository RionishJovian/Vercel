import { mikrotikProducts, type MikroTikCategory, type MikroTikProductPhoto } from "../../lib/mikrotikProducts";
import { portPanels, type PortPanel } from "../../lib/portPanels";
import { routerProfiles, type RouterProfile, type WifiBand } from "./routerProfiles";
import type { Uplink } from "./newRouterNetwork";

/**
 * The Magic Hub model picker lists every model in `lib/mikrotikProducts.ts`. Whether "new router"
 * network setup is offered for a model is decided here from facts, never by a hand-kept whitelist:
 *
 *  - A model is `ready` only when `routerProfiles.ts` has a row for it. That row carries the board
 *    code the script checks first, so a profile is the proof that someone confirmed the code.
 *  - Every other model is listed with the reason it is not ready, and only "Keep existing network
 *    settings" is offered for it.
 *  - A ready model offers Ethernet WAN, plus a Wi-Fi WAN option for each band it has a radio on.
 *    A wired-only profile (empty `radios`) therefore shows no Wi-Fi options at all.
 *
 * Reasons come from MikroTik's default-configurations page
 * (https://manual.mikrotik.com/docs/getting-started/configuration-management/default-configurations/):
 * switches have no WAN and CCR and RB1100 ship an "IP only" default with no WAN DHCP client. Every
 * other model uses the "AP Router" default (first Ethernet port is the WAN, the rest bridged on
 * 192.168.88.1/24) and is ready once a row in `routerProfiles.ts` records its printed board string.
 * The one model left without a row is a catalogue model whose printed board string is not
 * documented apart from another model's, so it stays "board-unverified" instead of guessing.
 * (The LTE default, 192.168.188.1/24 with an LTE WAN, belongs to the LTE Chateau models, not to
 * the Chateau PRO ax in the catalogue, which has no modem.)
 */
export type SetupBlock = "switch" | "ip-only" | "board-unverified";

/** Factory defaults that do not fit the Ethernet or Wi-Fi WAN script, by catalogue id. */
const IP_ONLY = new Set(["ccr2004", "ccr2116", "ccr2216", "rb1100"]);

export interface ModelEntry {
  product: MikroTikProductPhoto;
  panel?: PortPanel;
  /** Present when the model can be prepared by the setup script. */
  profile?: RouterProfile;
  /** Present when it cannot; never both. */
  blocked?: SetupBlock;
}

export function setupBlockFor(product: MikroTikProductPhoto, hasProfile: boolean): SetupBlock | undefined {
  if (hasProfile) return undefined;
  if (product.category === "switch") return "switch";
  if (IP_ONLY.has(product.id)) return "ip-only";
  return "board-unverified";
}

export const modelCatalogue: readonly ModelEntry[] = mikrotikProducts.map(product => {
  const profile = routerProfiles.find(p => p.productId === product.id);
  return { product, panel: portPanels.find(p => p.id === product.id), profile, blocked: setupBlockFor(product, profile !== undefined) };
});

export const findModel = (id: string): ModelEntry | undefined => modelCatalogue.find(m => m.product.id === id);
export const findModelByProfile = (profileId: string): ModelEntry | undefined => modelCatalogue.find(m => m.profile?.id === profileId);

/** The setup choices a model supports besides "keep existing": Ethernet, then a band per radio. Empty when blocked. */
export function uplinksFor(entry: ModelEntry | undefined): Uplink[] {
  const profile = entry?.profile;
  if (!profile) return [];
  const bands: WifiBand[] = (["5ghz", "2ghz"] as const).filter(band => profile.radios.some(r => r.band === band));
  return ["ethernet", ...bands];
}

const GROUPS: readonly MikroTikCategory[] = ["router", "ap", "switch"];

/** Catalogue order is kept inside each group; ready models come first so the usable ones are easy to find. */
export function modelGroups(): { category: MikroTikCategory; models: ModelEntry[] }[] {
  return GROUPS.map(category => ({
    category,
    models: modelCatalogue.filter(m => m.product.category === category).sort((a, b) => Number(!!b.profile) - Number(!!a.profile)),
  })).filter(g => g.models.length > 0);
}
