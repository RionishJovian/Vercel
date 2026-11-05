/** Seconds as the router writes durations: 1h4m23s. */
export function fmtDuration(total: number): string {
  const s = Math.max(0, Math.floor(total));
  const parts: Array<[number, string]> = [[Math.floor(s / 86400), "d"], [Math.floor((s % 86400) / 3600), "h"], [Math.floor((s % 3600) / 60), "m"], [s % 60, "s"]];
  return parts.filter(([n]) => n > 0).map(([n, u]) => n + u).join("") || "0s";
}

/** RouterOS scripts may report uptime as 2d03:04:05 or 00:05:12, others as 1h4m23s. Show one form. */
export function fmtUptime(raw: string | undefined): string {
  if (!raw) return "—";
  const seconds = raw.includes(":") ? parseSeconds(raw) : null;
  return seconds === null ? raw : fmtDuration(seconds);
}

export function fmtBytes(raw: string | undefined): string {
  const n = Number(raw);
  if (raw === undefined || raw === "" || !Number.isFinite(n)) return "—";
  if (n < 1000) return `${n} B`;
  if (n < 1_000_000) return `${Math.round(n / 1000)} KB`;
  if (n < 1_000_000_000) { const v = n / 1_000_000; return `${v < 10 ? v.toFixed(1) : Math.round(v)} MB`; }
  return `${(n / 1_000_000_000).toFixed(1)} GB`;
}

/** Seconds in a RouterOS duration, in either form; null when there is none. */
export function parseSeconds(raw: string | undefined): number | null {
  const text = (raw ?? "").trim();
  if (!text) return null;
  const clock = /^(?:(\d+)w)?(?:(\d+)d)?\s*(\d+):(\d\d):(\d\d)$/.exec(text);
  if (clock) return Number(clock[1] ?? 0) * 604800 + Number(clock[2] ?? 0) * 86400 + Number(clock[3]) * 3600 + Number(clock[4]) * 60 + Number(clock[5]);
  const compact = /^(?:(\d+)w)?(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(text);
  if (!compact) return null;
  return Number(compact[1] ?? 0) * 604800 + Number(compact[2] ?? 0) * 86400 + Number(compact[3] ?? 0) * 3600 + Number(compact[4] ?? 0) * 60 + Number(compact[5] ?? 0);
}

/** At or above this, a session counts as idle. */
export const IDLE_SECONDS = 15 * 60;
/** At or below this, a session counts as near expiry. */
export const NEAR_EXPIRY_SECONDS = 15 * 60;
