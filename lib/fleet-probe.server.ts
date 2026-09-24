// Probe one router for the Fleet page / AI scan using the same transport
// as Test and Live (loadRouterConn). Never dial the stored public host for
// Magic Hub, connector, sandbox, or tunnel rows.
import type { RouterConn } from "./mikrotik.server";
import { parseCpuLoad, toBytes } from "./fleet-health";

export type FleetRouter = {
  id: string;
  name: string;
  host: string;
  online: boolean;
  error?: string;
  version?: string;
  uptime?: string;
  /** RouterOS board-name from /system/resource — real hardware identity. */
  board_name?: string;
  /** /system/identity name when available. */
  identity?: string;
  cpu_load?: number;
  free_memory?: number;
  total_memory?: number;
  /** Celsius from /system/health when the board exposes it. */
  temperature_c?: number;
  active_sessions?: number;
  hotspot_users?: number;
  blocked_bindings?: number;
  hosts?: number;
  /** Physical/virtual interfaces that are enabled but not running. */
  interfaces_down?: Array<{ name: string; type?: string }>;
  dhcp_pools?: Array<{ name: string; total: number; used: number }>;
  wireguard_peers?: { total: number; handshake_ok: number };
  firewall_rules?: number;
  queue_trees?: number;
  /** Ingested syslog_events for this router since local midnight. */
  syslog_today?: number | null;
  last_checked: number;
  connection_mode?: string | null;
  connector_id?: string | null;
  /** Site this router is attached to (null = unassigned). */
  site_id?: string | null;
};

export type FleetProbeApi = {
  ping: (c: RouterConn) => Promise<unknown>;
  activeUsers: (c: RouterConn) => Promise<unknown[]>;
  listBindings: (c: RouterConn) => Promise<Array<{ type?: string }>>;
  hosts: (c: RouterConn) => Promise<unknown[]>;
  users: (c: RouterConn) => Promise<unknown[]>;
  /** Escape hatch for deeper RouterOS reads (health, pools, peers, …). */
  raw?: <T = unknown>(c: RouterConn, path: string) => Promise<T>;
};

function parseTemperatureC(health: unknown): number | undefined {
  if (!Array.isArray(health)) return undefined;
  for (const row of health) {
    if (!row || typeof row !== "object") continue;
    const r = row as Record<string, string>;
    const name = (r.name ?? r["name"] ?? "").toLowerCase();
    if (!name.includes("temp")) continue;
    const n = Number(String(r.value ?? r["value"] ?? "").replace(/[^\d.-]/g, ""));
    if (Number.isFinite(n)) return n;
  }
  return undefined;
}

/** Rough address count from a RouterOS pool ranges string like "10.0.0.2-10.0.0.254". */
export function estimatePoolSize(ranges: string | undefined): number {
  if (!ranges) return 0;
  let total = 0;
  for (const part of ranges.split(",")) {
    const bit = part.trim();
    const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)-(\d+)\.(\d+)\.(\d+)\.(\d+)$/.exec(bit);
    if (!m) continue;
    const a =
      ((Number(m[1]) << 24) | (Number(m[2]) << 16) | (Number(m[3]) << 8) | Number(m[4])) >>> 0;
    const b =
      ((Number(m[5]) << 24) | (Number(m[6]) << 16) | (Number(m[7]) << 8) | Number(m[8])) >>> 0;
    if (b >= a) total += b - a + 1;
  }
  return total;
}

function handshakeFresh(peer: Record<string, string>, nowSec: number): boolean {
  const raw = peer["last-handshake"] ?? peer["last-handshake"] ?? "";
  if (!raw) return false;
  // RouterOS may return seconds since handshake as a number string, or a duration.
  const asNum = Number(raw);
  if (Number.isFinite(asNum) && asNum > 1_000_000_000) {
    // unix timestamp
    return nowSec - asNum < 180;
  }
  if (Number.isFinite(asNum) && asNum >= 0 && asNum < 1_000_000) {
    // seconds ago
    return asNum < 180;
  }
  // Duration like 1m20s — treat any non-empty recent-looking duration as ok if < 3m-ish.
  if (/^\d+[smhd]/i.test(raw) && !/^[3-9]\d*m|^[1-9]\d*h|^[1-9]\d*d/i.test(raw)) {
    return true;
  }
  return Boolean(peer["current-endpoint"]);
}

export async function collectFleetSnapshot(
  row: {
    id: string;
    name: string;
    host: string;
    connection_mode?: string | null;
    connector_id?: string | null;
    site_id?: string | null;
  },
  deps: {
    loadConn: (id: string) => Promise<RouterConn>;
    api: FleetProbeApi;
  },
): Promise<FleetRouter> {
  const base: FleetRouter = {
    id: row.id,
    name: row.name,
    host: row.host,
    online: false,
    last_checked: Date.now(),
    connection_mode: row.connection_mode ?? null,
    connector_id: row.connector_id ?? null,
    site_id: row.site_id ?? null,
  };
  try {
    const conn = await deps.loadConn(row.id);
    const res = (await deps.api.ping(conn)) as Record<string, string>;
    base.online = true;
    base.version = res.version;
    base.uptime = res.uptime;
    base.board_name = res["board-name"] || undefined;
    base.cpu_load = parseCpuLoad(res["cpu-load"]);
    base.free_memory = toBytes(res["free-memory"]);
    base.total_memory = toBytes(res["total-memory"]);
    const [active, bindings, hosts, users] = await Promise.allSettled([
      deps.api.activeUsers(conn),
      deps.api.listBindings(conn),
      deps.api.hosts(conn),
      deps.api.users(conn),
    ]);
    if (active.status === "fulfilled") base.active_sessions = active.value.length;
    if (bindings.status === "fulfilled")
      base.blocked_bindings = bindings.value.filter((b) => b.type === "blocked").length;
    if (hosts.status === "fulfilled") base.hosts = hosts.value.length;
    if (users.status === "fulfilled") base.hotspot_users = users.value.length;

    const raw = deps.api.raw;
    if (raw) {
      const deeper = await Promise.allSettled([
        raw<Record<string, string> | Array<Record<string, string>>>(conn, "/system/identity"),
        raw<Array<Record<string, string>>>(conn, "/system/health"),
        raw<Array<Record<string, string>>>(conn, "/interface"),
        raw<Array<Record<string, string>>>(conn, "/ip/pool"),
        raw<Array<Record<string, string>>>(conn, "/ip/pool/used"),
        raw<Array<Record<string, string>>>(conn, "/interface/wireguard/peers"),
        raw<Array<Record<string, string>>>(conn, "/ip/firewall/filter"),
        raw<Array<Record<string, string>>>(conn, "/queue/tree"),
      ]);

      if (deeper[0].status === "fulfilled") {
        const idn = deeper[0].value;
        if (Array.isArray(idn)) base.identity = idn[0]?.name;
        else if (idn && typeof idn === "object")
          base.identity = (idn as Record<string, string>).name;
      }
      if (deeper[1].status === "fulfilled") {
        base.temperature_c = parseTemperatureC(deeper[1].value);
      }
      if (deeper[2].status === "fulfilled") {
        base.interfaces_down = deeper[2].value
          .filter((i) => i.disabled !== "true" && i.running !== "true" && i.name)
          .map((i) => ({ name: i.name, type: i.type }));
      }
      if (deeper[3].status === "fulfilled") {
        const pools = deeper[3].value;
        const usedRows = deeper[4].status === "fulfilled" ? deeper[4].value : [];
        const usedByPool = new Map<string, number>();
        for (const u of usedRows) {
          const pool = u.pool ?? u["pool"];
          if (!pool) continue;
          usedByPool.set(pool, (usedByPool.get(pool) ?? 0) + 1);
        }
        base.dhcp_pools = pools
          .map((p) => {
            const name = p.name;
            if (!name) return null;
            const total = estimatePoolSize(p.ranges);
            const used = usedByPool.get(name) ?? 0;
            return { name, total, used };
          })
          .filter((p): p is { name: string; total: number; used: number } => !!p && p.total > 0);
      }
      if (deeper[5].status === "fulfilled") {
        const peers = deeper[5].value;
        const nowSec = Math.floor(Date.now() / 1000);
        base.wireguard_peers = {
          total: peers.length,
          handshake_ok: peers.filter((p) => handshakeFresh(p, nowSec)).length,
        };
      }
      if (deeper[6].status === "fulfilled") base.firewall_rules = deeper[6].value.length;
      if (deeper[7].status === "fulfilled") base.queue_trees = deeper[7].value.length;
    }
  } catch (e) {
    base.error = e instanceof Error ? e.message : String(e);
  }
  return base;
}
