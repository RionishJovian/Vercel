/** A signal age as a compact, language-neutral reading: "42s", "5m", "3h", "2d". Returns null for a missing or future-dated time. */
export function ageShort(iso: string | undefined | null, now = Date.now()): string | null {
  const at = Date.parse(iso ?? "");
  if (!Number.isFinite(at)) return null;
  const s = Math.max(0, Math.round((now - at) / 1000));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m";
  if (s < 86400) return Math.floor(s / 3600) + "h";
  return Math.floor(s / 86400) + "d";
}

/** Split "10.8.0.12/32" into the address and its prefix so the prefix can be styled apart. */
export function splitCidr(value: string | undefined | null): { addr: string; cidr: string } | null {
  const v = (value ?? "").trim();
  if (!v) return null;
  const i = v.indexOf("/");
  return i < 0 ? { addr: v, cidr: "" } : { addr: v.slice(0, i), cidr: v.slice(i) };
}
