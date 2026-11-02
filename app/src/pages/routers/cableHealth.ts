/**
 * Plain-language hints about a wired port, from what the router itself reports. A hint is only given when the
 * data supports it: no reading means no hint, never "all good". A port with no link or a disabled port is never
 * treated as faulty, since an empty port is normal.
 */
export interface CableFacts {
  running?: boolean; disabled?: boolean; link_rate?: string; full_duplex?: boolean; negotiation?: string;
  max_rate_mbps?: number; partner_max_mbps?: number;
  /** The router's lifetime error counter arrives as a string (it can exceed what a JSON number holds exactly). */
  phy_errors?: string | number; phy_errors_recent?: number;
}
export type HintId = "neg" | "half" | "slowCable" | "partnerLimit" | "belowMax" | "fixedSpeed" | "errNow" | "errPast" | "errWatching";
export interface Hint { id: HintId; tone: "warn" | "info" }

/** "1Gbps" -> 1000, "100Mbps" -> 100, "2.5Gbps" -> 2500; anything else is 0 (unknown). */
export function rateMbps(rate?: string): number {
  const m = /^([0-9]+(?:\.[0-9]+)?)([MG])bps$/.exec(rate ?? "");
  if (!m) return 0;
  const n = parseFloat(m[1]) * (m[2] === "G" ? 1000 : 1);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

export function cableHints(f: CableFacts): Hint[] {
  if (f.disabled || !f.running) return [];
  const out: Hint[] = [];
  if (f.negotiation === "failed" || f.negotiation === "incomplete") out.push({ id: "neg", tone: "warn" });
  if (f.full_duplex === false) out.push({ id: "half", tone: "warn" });
  const rate = rateMbps(f.link_rate), top = f.max_rate_mbps ?? 0, partner = f.partner_max_mbps ?? 0;
  if (rate > 0 && top > 0 && rate < top) {
    if (f.negotiation === "disabled") out.push({ id: "fixedSpeed", tone: "info" });
    else if (partner > rate) out.push({ id: "slowCable", tone: "warn" });
    else if (partner > 0) out.push({ id: "partnerLimit", tone: "info" });
    else out.push({ id: "belowMax", tone: "info" });
  }
  const total = Number(f.phy_errors ?? 0);
  if (typeof f.phy_errors_recent === "number" && f.phy_errors_recent > 0) out.push({ id: "errNow", tone: "warn" });
  else if (Number.isFinite(total) && total > 0) out.push({ id: typeof f.phy_errors_recent === "number" ? "errPast" : "errWatching", tone: "info" });
  return out;
}

export const needsAttention = (hints: readonly Hint[]) => hints.some((h) => h.tone === "warn");
