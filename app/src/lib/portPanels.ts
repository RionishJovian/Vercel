import { productForName } from "./mikrotikProducts";

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
  /** Marker diameter, as a percentage of the picture width. */
  size: number;
  ports: readonly PortSpot[];
}

export const portPanels: readonly PortPanel[] = [
  {
    id: "hap-ax3", image: "models/panel/hap-ax3.webp", width: 1000, height: 292, alt: "Back panel of a MikroTik hAP ax³", board: /^c53uig/i, size: 10,
    ports: [{ name: "ether1", x: 35.4, y: 45.7 }, { name: "ether2", x: 45.9, y: 45.7 }, { name: "ether3", x: 55.9, y: 45.7 }, { name: "ether4", x: 65.9, y: 45.7 }, { name: "ether5", x: 75.4, y: 45.7 }],
  },
  {
    id: "hap-ax2", image: "models/panel/hap-ax2.webp", width: 609, height: 1000, alt: "Back panel of a MikroTik hAP ax²", board: /^c52ig/i, size: 9,
    ports: [{ name: "ether5", x: 48, y: 15 }, { name: "ether4", x: 48, y: 26.2 }, { name: "ether3", x: 48, y: 37.2 }, { name: "ether2", x: 48, y: 48.5 }, { name: "ether1", x: 48, y: 59.7 }],
  },
];

/** The diagram for this router, or none. The board name wins; the owner's typed name is the fallback and must match exactly. */
export function panelFor(board?: string, hardwareName?: string): PortPanel | undefined {
  const code = (board ?? "").trim();
  const byBoard = code ? portPanels.find((p) => p.board.test(code)) : undefined;
  if (byBoard) return byBoard;
  const id = productForName(hardwareName)?.id;
  return id ? portPanels.find((p) => p.id === id) : undefined;
}
