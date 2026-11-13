/**
 * Back-panel pictures with the position of each Ethernet port, so the router page can light the real
 * ports with their live state. Positions are percentages of the picture. The pictures are MikroTik's
 * own artwork; a model without an entry here simply has no diagram, never a borrowed one.
 */
export interface PortSpot { name: string; x: number; y: number }
export interface PortPanel {
  id: string; image: string; width: number; height: number; alt: string;
  /** RouterOS board name, e.g. "C53UiG+5HPaxD2HPaxD": the most reliable way to identify the model. */
  board: RegExp;
  /** Exact model aliases; never match a family or a customer label containing a model. */
  names: readonly string[];
  /** Dense diagrams retain a usable separation between adjacent controls. */
  minWidth?: number;
  /** Marker diameter, as a percentage of the picture width. */
  size: number;
  ports: readonly PortSpot[];
}

export const portPanels: readonly PortPanel[] = [
  {
    id: "hap-ax3", image: "models/panel/hap-ax3.webp", width: 1000, height: 292, alt: "Back panel of a MikroTik hAP ax³", board: /^c53uig\+5hpaxd2hpaxd$/i, names: ["hAP ax3"], size: 10,
    ports: [{ name: "ether1", x: 35.4, y: 45.7 }, { name: "ether2", x: 45.9, y: 45.7 }, { name: "ether3", x: 55.9, y: 45.7 }, { name: "ether4", x: 65.9, y: 45.7 }, { name: "ether5", x: 75.4, y: 45.7 }],
  },
  {
    id: "hap-ax2", image: "models/panel/hap-ax2.webp", width: 609, height: 1000, alt: "Back panel of a MikroTik hAP ax²", board: /^c52ig-5haxd2haxd(?:-tc)?$/i, names: ["hAP ax2"], size: 9,
    ports: [{ name: "ether5", x: 48, y: 15 }, { name: "ether4", x: 48, y: 26.2 }, { name: "ether3", x: 48, y: 37.2 }, { name: "ether2", x: 48, y: 48.5 }, { name: "ether1", x: 48, y: 59.7 }],
  },
  {
    id: "hap-ax-lite", image: "models/panel/hap-ax-lite.webp", width: 527, height: 1200, alt: "Ethernet ports of a MikroTik hAP ax lite",
    board: /^l41g-2axd$/i, names: ["hAP ax lite"], size: 15,
    ports: [{ name: "ether1", x: 50, y: 56 }, { name: "ether2", x: 50, y: 45 }, { name: "ether3", x: 50, y: 34 }, { name: "ether4", x: 50, y: 23 }],
  },
  {
    id: "hap-ac3", image: "models/panel/hap-ac3.webp", width: 1200, height: 171, alt: "Ethernet ports of a MikroTik hAP ac³",
    board: /^(?:rb)?d53ig-5hacd2hnd$/i, names: ["hAP ac3"], size: 4, minWidth: 560,
    ports: [{ name: "ether1", x: 32, y: 27 }, { name: "ether2", x: 37, y: 27 }, { name: "ether3", x: 42, y: 27 }, { name: "ether4", x: 47, y: 27 }, { name: "ether5", x: 52, y: 27 }],
  },
  {
    id: "hex-s", image: "models/panel/hex-s.webp", width: 1200, height: 233, alt: "Ethernet ports of a MikroTik hEX S (RB760iGS)",
    board: /^rb760igs$/i, names: ["hEX S (RB760iGS)", "RB760iGS"], size: 9,
    ports: [{ name: "ether1", x: 39, y: 40 }, { name: "ether2", x: 51, y: 40 }, { name: "ether3", x: 63, y: 40 }, { name: "ether4", x: 75, y: 40 }, { name: "ether5", x: 87, y: 40 }],
  },
  {
    id: "l009", image: "models/panel/l009.webp", width: 1177, height: 135, alt: "Ethernet ports of a MikroTik L009UiGS-RM",
    board: /^l009uigs(?:-rm)?$/i, names: ["L009UiGS-RM"], size: 5.5, minWidth: 560,
    ports: [{ name: "ether1", x: 36, y: 34 }, { name: "ether2", x: 43.3, y: 34 }, { name: "ether3", x: 50.6, y: 34 }, { name: "ether4", x: 57.9, y: 34 }, { name: "ether5", x: 65.2, y: 34 }, { name: "ether6", x: 72.5, y: 34 }, { name: "ether7", x: 79.8, y: 34 }, { name: "ether8", x: 87.1, y: 34 }],
  },
  {
    id: "rb4011", image: "models/panel/rb4011.webp", width: 1200, height: 134, alt: "Ethernet ports of a MikroTik RB4011iGS+RM",
    board: /^rb4011igs\+rm$/i, names: ["RB4011iGS+RM"], size: 5, minWidth: 560,
    ports: [{ name: "ether1", x: 23, y: 62 }, { name: "ether2", x: 29, y: 62 }, { name: "ether3", x: 35, y: 62 }, { name: "ether4", x: 41, y: 62 }, { name: "ether5", x: 47, y: 62 }, { name: "ether6", x: 67.5, y: 62 }, { name: "ether7", x: 73.5, y: 62 }, { name: "ether8", x: 79.5, y: 62 }, { name: "ether9", x: 85.5, y: 62 }, { name: "ether10", x: 91.5, y: 62 }],
  },
  {
    id: "ccr2004", image: "models/panel/ccr2004.webp", width: 1200, height: 212, alt: "Ethernet ports of a MikroTik CCR2004-16G-2S+PC",
    board: /^ccr2004-16g-2s\+pc$/i, names: ["CCR2004-16G-2S+PC"], size: 4.8, minWidth: 640,
    ports: [{ name: "ether1", x: 28, y: 67 }, { name: "ether2", x: 28, y: 34 }, { name: "ether3", x: 34, y: 67 }, { name: "ether4", x: 34, y: 34 }, { name: "ether5", x: 40, y: 67 }, { name: "ether6", x: 40, y: 34 }, { name: "ether7", x: 46, y: 67 }, { name: "ether8", x: 46, y: 34 }, { name: "ether9", x: 54.5, y: 67 }, { name: "ether10", x: 54.5, y: 34 }, { name: "ether11", x: 60.5, y: 67 }, { name: "ether12", x: 60.5, y: 34 }, { name: "ether13", x: 66.5, y: 67 }, { name: "ether14", x: 66.5, y: 34 }, { name: "ether15", x: 72.5, y: 67 }, { name: "ether16", x: 72.5, y: 34 }],
  },
  {
    id: "ccr2116", image: "models/panel/ccr2116.webp", width: 1200, height: 142, alt: "Ethernet ports of a MikroTik CCR2116-12G-4S+",
    board: /^ccr2116-12g-4s\+$/i, names: ["CCR2116-12G-4S+"], size: 3, minWidth: 960,
    ports: [{ name: "ether1", x: 12.2, y: 68 }, { name: "ether2", x: 16, y: 68 }, { name: "ether3", x: 19.8, y: 68 }, { name: "ether4", x: 23.6, y: 68 }, { name: "ether5", x: 28.1, y: 68 }, { name: "ether6", x: 31.9, y: 68 }, { name: "ether7", x: 35.7, y: 68 }, { name: "ether8", x: 39.5, y: 68 }, { name: "ether9", x: 44.1, y: 68 }, { name: "ether10", x: 47.9, y: 68 }, { name: "ether11", x: 51.7, y: 68 }, { name: "ether12", x: 55.5, y: 68 }, { name: "ether13", x: 60.2, y: 68 }],
  },
  {
    id: "ccr2216", image: "models/panel/ccr2216.webp", width: 1200, height: 130, alt: "Ethernet ports of a MikroTik CCR2216-1G-12XS-2XQ",
    board: /^ccr2216-1g-12xs-2xq$/i, names: ["CCR2216-1G-12XS-2XQ"], size: 3.5, minWidth: 560,
    ports: [{ name: "ether1", x: 48, y: 34 }],
  },
  {
    id: "crs112", image: "models/panel/crs112.webp", width: 1200, height: 250, alt: "Ethernet ports of a MikroTik CRS112-8P-4S-IN",
    board: /^crs112-8p-4s-in$/i, names: ["CRS112-8P-4S-IN"], size: 5.5, minWidth: 560,
    ports: [{ name: "ether1", x: 37, y: 64 }, { name: "ether2", x: 37, y: 33 }, { name: "ether3", x: 44, y: 64 }, { name: "ether4", x: 44, y: 33 }, { name: "ether5", x: 51, y: 64 }, { name: "ether6", x: 51, y: 33 }, { name: "ether7", x: 58, y: 64 }, { name: "ether8", x: 58, y: 33 }],
  },
  {
    id: "crs305", image: "models/panel/crs305.webp", width: 1200, height: 249, alt: "Ethernet ports of a MikroTik CRS305-1G-4S+IN",
    board: /^crs305-1g-4s\+in$/i, names: ["CRS305-1G-4S+IN"], size: 10,
    ports: [{ name: "ether1", x: 14, y: 40 }],
  },
  {
    id: "crs309", image: "models/panel/crs309.webp", width: 1200, height: 125, alt: "Ethernet ports of a MikroTik CRS309-1G-8S+IN",
    board: /^crs309-1g-8s\+in$/i, names: ["CRS309-1G-8S+IN"], size: 4, minWidth: 560,
    ports: [{ name: "ether1", x: 71, y: 49 }],
  },
  {
    id: "crs310", image: "models/panel/crs310.webp", width: 1072, height: 258, alt: "Ethernet ports of a MikroTik CRS310-8G+2S+IN",
    board: /^crs310-8g\+2s\+in$/i, names: ["CRS310-8G+2S+IN"], size: 5.5, minWidth: 560,
    ports: [{ name: "ether1", x: 46.5, y: 69 }, { name: "ether2", x: 46.5, y: 39 }, { name: "ether3", x: 53.4, y: 69 }, { name: "ether4", x: 53.4, y: 39 }, { name: "ether5", x: 60.3, y: 69 }, { name: "ether6", x: 60.3, y: 39 }, { name: "ether7", x: 67.2, y: 69 }, { name: "ether8", x: 67.2, y: 39 }],
  },
  {
    id: "crs328", image: "models/panel/crs328.webp", width: 1200, height: 119, alt: "Ethernet ports of a MikroTik CRS328-24P-4S+RM",
    board: /^crs328-24p-4s\+rm$/i, names: ["CRS328-24P-4S+RM"], size: 2.6, minWidth: 1120,
    ports: [{ name: "ether1", x: 17, y: 68 }, { name: "ether2", x: 17, y: 34 }, { name: "ether3", x: 20.2, y: 68 }, { name: "ether4", x: 20.2, y: 34 }, { name: "ether5", x: 23.4, y: 68 }, { name: "ether6", x: 23.4, y: 34 }, { name: "ether7", x: 26.5, y: 68 }, { name: "ether8", x: 26.5, y: 34 }, { name: "ether9", x: 32.4, y: 68 }, { name: "ether10", x: 32.4, y: 34 }, { name: "ether11", x: 35.5, y: 68 }, { name: "ether12", x: 35.5, y: 34 }, { name: "ether13", x: 38.6, y: 68 }, { name: "ether14", x: 38.6, y: 34 }, { name: "ether15", x: 41.8, y: 68 }, { name: "ether16", x: 41.8, y: 34 }, { name: "ether17", x: 48, y: 68 }, { name: "ether18", x: 48, y: 34 }, { name: "ether19", x: 51.1, y: 68 }, { name: "ether20", x: 51.1, y: 34 }, { name: "ether21", x: 54.3, y: 68 }, { name: "ether22", x: 54.3, y: 34 }, { name: "ether23", x: 57.5, y: 68 }, { name: "ether24", x: 57.5, y: 34 }],
  },
];

const normalizedName = (name: string) => name.trim().toLowerCase().replace(/²/g, "2").replace(/³/g, "3").replace(/\^/g, "");
const exactPanel = (name: string) => portPanels.find((p) => p.board.test(name.trim()) || p.names.some((alias) => normalizedName(alias) === normalizedName(name)));

/** Reported identity wins, including an unsupported identity. Only use the saved name when no board was reported. */
export function panelFor(board?: string, hardwareName?: string): PortPanel | undefined {
  if (board?.trim()) return exactPanel(board);
  return hardwareName?.trim() ? exactPanel(hardwareName) : undefined;
}
