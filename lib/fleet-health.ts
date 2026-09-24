/**
 * Pure Fleet health helpers. No I/O — safe to import from the page and tests.
 */

export type FleetInsight = {
  id: string;
  severity: string;
  title: string;
  subtitle?: string;
  router?: string;
  /** Stable router_connections.id when known — required for Apply fix. */
  router_id?: string;
  suggestion?: string;
  fix_command?: string;
};

/**
 * Shared live RouterOS totals for Fleet and Home. A session is counted only
 * when RouterOS reports it as active; voucher lifecycle data is not a proxy
 * for a currently connected guest.
 */
export function fleetLiveTotals(
  routers: ReadonlyArray<{ online: boolean; active_sessions?: number | null }>,
) {
  const online = routers.filter((router) => router.online).length;
  return {
    online,
    offline: Math.max(0, routers.length - online),
    activeSessions: routers.reduce((total, router) => total + (router.active_sessions ?? 0), 0),
  };
}

/** Relative age for insight / scan timestamps (User useful). */
export function relativeAge(
  isoOrMs: string | number | Date | null | undefined,
  now = Date.now(),
): string {
  if (isoOrMs == null) return "—";
  const t = typeof isoOrMs === "number" ? isoOrMs : new Date(isoOrMs).getTime();
  if (!Number.isFinite(t)) return "—";
  const sec = Math.max(0, Math.round((now - t) / 1000));
  if (sec < 45) return "just now";
  if (sec < 3600) return `${Math.max(1, Math.round(sec / 60))}m`;
  if (sec < 86400) return `${Math.max(1, Math.round(sec / 3600))}h`;
  return `${Math.max(1, Math.round(sec / 86400))}d`;
}

export function fmtBytes(n?: number): string {
  if (n == null || !Number.isFinite(n)) return "—";
  if (n <= 0) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${u[i]}`;
}

/** Compact RouterOS uptime (`2d10h38m45s` → `2d 10h`) so metric chips do not clip. */
export function fmtUptime(raw?: string | null): string {
  if (raw == null) return "—";
  const trimmed = raw.trim();
  if (!trimmed) return "—";
  const parts = [...trimmed.matchAll(/(\d+)([wdhms])/gi)].map(
    (m) => `${m[1]}${m[2].toLowerCase()}`,
  );
  if (!parts.length) return trimmed;
  return parts.slice(0, 2).join(" ");
}

/** Idle label stays put during the 30s poll so the control does not read as "Refreshinghealth". */
export function fleetRefreshLabel(userRefreshPending: boolean): string {
  return userRefreshPending ? "Refreshing…" : "Refresh health";
}

export function parseCpuLoad(v: unknown): number | undefined {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string") {
    const trimmed = v.replace(/%/g, "").trim();
    if (!trimmed) return undefined;
    const n = Number(trimmed);
    return Number.isFinite(n) ? n : undefined;
  }
  return undefined;
}

export function memoryUsedPercent(total?: number, free?: number): number | undefined {
  if (total == null || free == null) return undefined;
  if (!Number.isFinite(total) || total <= 0 || !Number.isFinite(free) || free < 0) return undefined;
  return Math.round(((total - free) / total) * 100);
}

export function toBytes(v?: string): number | undefined {
  if (v == null || v === "") return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

export function fleetInsights(payload: unknown): FleetInsight[] {
  if (!payload || typeof payload !== "object" || !("insights" in payload)) return [];
  const values = (payload as { insights?: unknown }).insights;
  if (!Array.isArray(values)) return [];
  return values.flatMap((value) => {
    if (!value || typeof value !== "object") return [];
    const rec = value as Record<string, unknown>;
    if (
      typeof rec.id !== "string" ||
      typeof rec.severity !== "string" ||
      typeof rec.title !== "string"
    )
      return [];
    return [
      {
        id: rec.id,
        severity: rec.severity,
        title: rec.title,
        subtitle: typeof rec.subtitle === "string" ? rec.subtitle : undefined,
        router: typeof rec.router === "string" ? rec.router : undefined,
        router_id: typeof rec.router_id === "string" ? rec.router_id : undefined,
        suggestion: typeof rec.suggestion === "string" ? rec.suggestion : undefined,
        fix_command: typeof rec.fix_command === "string" ? rec.fix_command : undefined,
      },
    ];
  });
}

export function fleetModeLabel(row: {
  connection_mode?: string | null;
  connector_id?: string | null;
}): string {
  if (row.connection_mode === "sandbox") return "Sandbox";
  if (row.connection_mode === "hub" || row.connection_mode === "cloud") return "Magic Hub";
  if (row.connector_id) return "Local Connector";
  return "Direct (legacy)";
}

export function readAlertKeys(raw: string | null): string[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((k): k is string => typeof k === "string").slice(-500);
  } catch {
    return [];
  }
}
