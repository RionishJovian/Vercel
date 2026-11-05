/** Seconds as the router writes durations: 1h4m23s. */
export function fmtDuration(total: number): string {
  const s = Math.max(0, Math.floor(total));
  const parts: Array<[number, string]> = [[Math.floor(s / 86400), "d"], [Math.floor((s % 86400) / 3600), "h"], [Math.floor((s % 3600) / 60), "m"], [s % 60, "s"]];
  return parts.filter(([n]) => n > 0).map(([n, u]) => n + u).join("") || "0s";
}

/** RouterOS scripts may report uptime as 2d03:04:05 or 00:05:12, others as 1h4m23s. Show one form. */
export function fmtUptime(raw: string | undefined): string {
  if (!raw) return "—";
  const m = /^(?:(\d+)w)?(?:(\d+)d)?\s*(\d+):(\d\d):(\d\d)$/.exec(raw.trim());
  if (!m) return raw;
  return fmtDuration(Number(m[1] ?? 0) * 604800 + Number(m[2] ?? 0) * 86400 + Number(m[3]) * 3600 + Number(m[4]) * 60 + Number(m[5]));
}

export function fmtBytes(raw: string | undefined): string {
  const n = Number(raw);
  if (raw === undefined || raw === "" || !Number.isFinite(n)) return "—";
  if (n < 1000) return `${n} B`;
  if (n < 1_000_000) return `${Math.round(n / 1000)} KB`;
  if (n < 1_000_000_000) { const v = n / 1_000_000; return `${v < 10 ? v.toFixed(1) : Math.round(v)} MB`; }
  return `${(n / 1_000_000_000).toFixed(1)} GB`;
}

/** At or below this, a session counts as near expiry. */
export const NEAR_EXPIRY_SECONDS = 15 * 60;
