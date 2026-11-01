/**
 * Official MikroTik product photos, served as shared static files for every account.
 * Photos identify a model, never live device state. The images belong to MikroTik;
 * productUrl and sourceImageUrl keep where each one came from.
 */
export type MikroTikCategory = "router" | "switch" | "ap";
export interface MikroTikProductPhoto {
  id: string; name: string; category: MikroTikCategory; image: string;
  alt: string; productUrl: string; sourceImageUrl: string;
  /** Matches the model as RouterOS or the owner writes it; never a loose family match. */
  match: RegExp;
}
export const mikrotikProducts: readonly MikroTikProductPhoto[] = [
  { id: "rb5009", name: "RB5009UG+S+IN", category: "router", image: "models/rb5009.jpg", alt: "MikroTik RB5009UG+S+IN", match: /\brb5009/, productUrl: "https://mikrotik.com/product/rb5009ug_s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2065_lg.webp" },
  { id: "ccr2004", name: "CCR2004-16G-2S+PC", category: "router", image: "models/ccr2004.jpg", alt: "MikroTik CCR2004-16G-2S+PC", match: /\bccr2004/, productUrl: "https://mikrotik.com/product/ccr2004_16g_2s_pc", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2634_lg.webp" },
  { id: "hap-ax2", name: "hAP ax²", category: "router", image: "models/hap-ax2.jpg", alt: "MikroTik hAP ax²", match: /\bhap ?ax\^?2(?![0-9])/, productUrl: "https://mikrotik.com/product/hap_ax2", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2203_lg.webp" },
  { id: "crs310", name: "CRS310-8G+2S+IN", category: "switch", image: "models/crs310.jpg", alt: "MikroTik CRS310-8G+2S+IN", match: /\bcrs310/, productUrl: "https://mikrotik.com/product/crs310_8g_2s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2280_lg.webp" },
  { id: "cap-ax", name: "cAP ax", category: "ap", image: "models/cap-ax.jpg", alt: "MikroTik cAP ax", match: /\bcap ?ax(?![0-9a-z]| ?lite)/, productUrl: "https://mikrotik.com/product/cap_ax", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2247_lg.webp" },
  { id: "wap-ax", name: "wAP ax", category: "ap", image: "models/wap-ax.jpg", alt: "MikroTik wAP ax", match: /\bwap ?ax(?![0-9a-z]| ?lite)/, productUrl: "https://mikrotik.com/product/wap_ax", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2410_lg.webp" },
];

/** Pass the deployment base, e.g. "/v2/", when assets are served under a subpath. */
export const productPhotoUrl = (product: MikroTikProductPhoto, base = "/"): string => base.replace(/\/$/, "") + "/" + product.image;
export const findProductPhoto = (id: string) => mikrotikProducts.find((product) => product.id === id);

/** The photo for an exact model name, or undefined so the caller shows a generic symbol. */
export function productForName(...names: Array<string | undefined>): MikroTikProductPhoto | undefined {
  const text = names.filter(Boolean).join(" ").toLowerCase().replace(/²/g, "2");
  return mikrotikProducts.find((product) => product.match.test(text));
}
