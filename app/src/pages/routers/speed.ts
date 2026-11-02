/** Port speed text. A speed needs two readings, so the first ones have none yet: say so instead of a bare dash. */
export function rateText(v: number | undefined, measuring: string | null): string {
  if (typeof v === "number") return v >= 1e6 ? (v / 1e6).toFixed(1) + " Mbps" : Math.round(v / 1e3) + " kbps";
  return measuring ?? "—";
}

/** Only the first readings are "measuring"; if no port ever gets a speed, the dash is the honest answer. */
export function stillMeasuring(ifaces: Array<{ rx_bps?: number; tx_bps?: number }>, reads: number): boolean {
  return reads < 3 && ifaces.length > 0 && !ifaces.some((i) => typeof i.rx_bps === "number" || typeof i.tx_bps === "number");
}
