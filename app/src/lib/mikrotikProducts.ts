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
  { id: "hap-ax3", name: "hAP ax³", category: "router", image: "models/hap-ax3.jpg", alt: "MikroTik hAP ax³", match: /\bhap ?ax3(?![0-9])/, productUrl: "https://mikrotik.com/product/hap_ax3", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2211_lg.webp" },
  { id: "hap-ax-lite", name: "hAP ax lite", category: "router", image: "models/hap-ax-lite.jpg", alt: "MikroTik hAP ax lite", match: /\bhap ?ax ?lite/, productUrl: "https://mikrotik.com/product/hap_ax_lite", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2225_lg.webp" },
  { id: "hap-ax-s", name: "hAP ax S", category: "router", image: "models/hap-ax-s.jpg", alt: "MikroTik hAP ax S", match: /\bhap ?ax ?s(?![a-z0-9])/, productUrl: "https://mikrotik.com/product/hap_ax_s", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2502_lg.webp" },
  { id: "hap-ac3", name: "hAP ac³", category: "router", image: "models/hap-ac3.jpg", alt: "MikroTik hAP ac³", match: /\bhap ?ac3(?![0-9])/, productUrl: "https://mikrotik.com/product/hap_ac3", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1975_lg.webp" },
  { id: "hap-ac2", name: "hAP ac²", category: "router", image: "models/hap-ac2.jpg", alt: "MikroTik hAP ac²", match: /\bhap ?ac2(?![0-9])/, productUrl: "https://mikrotik.com/product/hap_ac2", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1468_lg.webp" },
  { id: "hex-s", name: "hEX S", category: "router", image: "models/hex-s.jpg", alt: "MikroTik hEX S", match: /\bhex ?s(?![a-z0-9])/, productUrl: "https://mikrotik.com/product/hex_s", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1539_lg.webp" },
  { id: "rb4011", name: "RB4011iGS+RM", category: "router", image: "models/rb4011.jpg", alt: "MikroTik RB4011iGS+RM", match: /\brb4011/, productUrl: "https://mikrotik.com/product/rb4011igs_rm", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1633_lg.webp" },
  { id: "rb1100", name: "RB1100AHx4", category: "router", image: "models/rb1100.jpg", alt: "MikroTik RB1100AHx4", match: /\brb1100/, productUrl: "https://mikrotik.com/product/rb1100ahx4", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1344_lg.webp" },
  { id: "ccr2116", name: "CCR2116-12G-4S+", category: "router", image: "models/ccr2116.jpg", alt: "MikroTik CCR2116-12G-4S+", match: /\bccr2116/, productUrl: "https://mikrotik.com/product/ccr2116_12g_4splus", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2625_lg.webp" },
  { id: "ccr2216", name: "CCR2216-1G-12XS-2XQ", category: "router", image: "models/ccr2216.jpg", alt: "MikroTik CCR2216-1G-12XS-2XQ", match: /\bccr2216/, productUrl: "https://mikrotik.com/product/ccr2216_1g_12xs_2xq", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2122_lg.webp" },
  { id: "chateau-pro-ax", name: "Chateau PRO ax", category: "router", image: "models/chateau-pro-ax.jpg", alt: "MikroTik Chateau PRO ax", match: /\bchateau ?pro ?ax/, productUrl: "https://mikrotik.com/product/chateau_pro_ax", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2414_lg.webp" },
  { id: "l009", name: "L009UiGS-RM", category: "router", image: "models/l009.jpg", alt: "MikroTik L009UiGS-RM", match: /\bl009/, productUrl: "https://mikrotik.com/product/l009uigs_rm", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2267_lg.webp" },
  { id: "cap-ac", name: "cAP ac", category: "ap", image: "models/cap-ac.jpg", alt: "MikroTik cAP ac", match: /\bcap ?ac(?![0-9a-z])/, productUrl: "https://mikrotik.com/product/cap_ac", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1447_lg.webp" },
  { id: "cap-xl-ac", name: "cAP XL ac", category: "ap", image: "models/cap-xl-ac.jpg", alt: "MikroTik cAP XL ac", match: /\bcap ?xl ?ac/, productUrl: "https://mikrotik.com/product/cap_xl_ac", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/2099_lg.webp" },
  { id: "crs326", name: "CRS326-24G-2S+IN", category: "switch", image: "models/crs326.jpg", alt: "MikroTik CRS326-24G-2S+IN", match: /\bcrs326-?24g/, productUrl: "https://mikrotik.com/product/crs326_24g_2s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1938_lg.webp" },
  { id: "crs328", name: "CRS328-24P-4S+RM", category: "switch", image: "models/crs328.jpg", alt: "MikroTik CRS328-24P-4S+RM", match: /\bcrs328-?24p/, productUrl: "https://mikrotik.com/product/crs328_24p_4s_rm", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1493_lg.webp" },
  { id: "crs305", name: "CRS305-1G-4S+IN", category: "switch", image: "models/crs305.jpg", alt: "MikroTik CRS305-1G-4S+IN", match: /\bcrs305/, productUrl: "https://mikrotik.com/product/crs305_1g_4s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1659_lg.webp" },
  { id: "crs309", name: "CRS309-1G-8S+IN", category: "switch", image: "models/crs309.jpg", alt: "MikroTik CRS309-1G-8S+IN", match: /\bcrs309/, productUrl: "https://mikrotik.com/product/crs309_1g_8s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1730_lg.webp" },
  { id: "crs112", name: "CRS112-8P-4S-IN", category: "switch", image: "models/crs112.jpg", alt: "MikroTik CRS112-8P-4S-IN", match: /\bcrs112/, productUrl: "https://mikrotik.com/product/crs112_8p_4s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1466_lg.webp" },
  { id: "css610", name: "CSS610-8G-2S+IN", category: "switch", image: "models/css610.jpg", alt: "MikroTik CSS610-8G-2S+IN", match: /\bcss610-?8g/, productUrl: "https://mikrotik.com/product/css610_8g_2s_in", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1980_lg.webp" },
  { id: "wap-ac", name: "wAP ac", category: "ap", image: "models/wap-ac.jpg", alt: "MikroTik wAP ac", match: /\bwap ?ac(?![0-9a-z])/, productUrl: "https://mikrotik.com/product/wap_ac", sourceImageUrl: "https://cdn.mikrotik.com/web-assets/rb_images/1988_lg.webp" },
];

/** Pass the deployment base, e.g. "/v2/", when assets are served under a subpath. */
export const productPhotoUrl = (product: MikroTikProductPhoto, base = "/"): string => base.replace(/\/$/, "") + "/" + product.image;
export const findProductPhoto = (id: string) => mikrotikProducts.find((product) => product.id === id);

/** The photo for an exact model name, or undefined so the caller shows a generic symbol. */
export function productForName(...names: Array<string | undefined>): MikroTikProductPhoto | undefined {
  const text = names.filter(Boolean).join(" ").toLowerCase().replace(/²/g, "2").replace(/³/g, "3");
  return mikrotikProducts.find((product) => product.match.test(text));
}
