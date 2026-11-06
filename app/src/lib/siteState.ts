export interface Site { site_id: string; name: string; location?: string | null; timezone?: string | null; latitude?: number | null; longitude?: number | null; notes?: string | null; device_count?: number }
export interface Conn { site_id?: string | null; cloud_status?: string; cloud_last_seen_at?: string }

/** A path is live only when its last report is recent (5 minutes) and not from the future. */
export function connectionIsFresh(c: Conn, now = Date.now()): boolean {
  const seen = Date.parse(c.cloud_last_seen_at ?? "");
  return String(c.cloud_status ?? "").toLowerCase() === "online" && Number.isFinite(seen) && seen <= now + 30000 && now - seen <= 300000;
}
type PathState = "online" | "offline" | "unassigned";
export const stateOf = (site: Site, conns: Conn[]): PathState => { const p = conns.filter((c) => c.site_id === site.site_id); return p.some((c) => connectionIsFresh(c)) ? "online" : p.length ? "offline" : "unassigned"; };

