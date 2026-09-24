// Vendor drivers for switch / PoE port views.
//
// Server-only. Every driver returns the same SwitchPortView. When a vendor
// cannot be read through this app the driver says so explicitly instead of
// returning zeros, so the UI can show "not available" rather than a wrong
// number.

import type { SwitchPort, SwitchPortView } from "./ports";
import type { DeviceDescriptor } from "./vendors";
import { capability } from "./vendors";
import type { DatabaseClient } from "../database.types";

export type PortDriverContext = {
  supabase: DatabaseClient;
  /** router_connections row id, required for MikroTik transports. */
  routerId: string | null;
};

function num(v: unknown): number | null {
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function roleFor(row: Record<string, string>, bridged: Set<string>): SwitchPort["role"] {
  const name = row["name"] ?? "";
  const comment = (row["comment"] ?? "").toLowerCase();
  if (comment.includes("uplink") || /^(sfp|ether1)\b/i.test(name)) return "uplink";
  if (bridged.has(name)) return "access";
  return "unknown";
}

/** MikroTik CRS/CSS/RB ports through the existing RouterOS REST client. */
async function mikrotikPorts(ctx: PortDriverContext): Promise<SwitchPortView> {
  if (!ctx.routerId) throw new Error("This device is not linked to a registered MikroTik router.");
  const { loadRouterConn } = await import("../router-conn.server");
  const { routerAPI } = await import("../mikrotik.server");
  const conn = await loadRouterConn(ctx.supabase, ctx.routerId);

  const [eth, poe, bridgePorts] = await Promise.all([
    routerAPI.raw<Array<Record<string, string>>>(conn, "/interface/ethernet"),
    routerAPI
      .raw<Array<Record<string, string>>>(conn, "/interface/ethernet/poe/monitor")
      .catch(() => [] as Array<Record<string, string>>),
    routerAPI
      .raw<Array<Record<string, string>>>(conn, "/interface/bridge/port")
      .catch(() => [] as Array<Record<string, string>>),
  ]);

  const poeByName = new Map<string, Record<string, string>>();
  for (const p of poe ?? []) {
    const n = p["name"] ?? p["interface"];
    if (n) poeByName.set(n, p);
  }
  const bridged = new Set((bridgePorts ?? []).map((b) => b["interface"] ?? "").filter(Boolean));
  const mgmt = conn.host;

  const ports: SwitchPort[] = (eth ?? []).map((row, i) => {
    const name = row["name"] ?? `ether${i + 1}`;
    const p = poeByName.get(name);
    const poeStatus = (p?.["poe-out-status"] ?? "").toLowerCase();
    return {
      ref: row[".id"] ?? name,
      name,
      index: i + 1,
      enabled: row["disabled"] !== "true",
      link: row["running"] === "true" ? "up" : row["running"] === "false" ? "down" : "unknown",
      speedMbps: num((row["rate"] ?? "").replace(/[^0-9]/g, "")),
      duplex:
        row["full-duplex"] === "true" ? "full" : row["full-duplex"] === "false" ? "half" : null,
      role: roleFor(row, bridged),
      vlan: num(row["pvid"]),
      poe: !p
        ? "unsupported"
        : poeStatus.includes("powered")
          ? "on"
          : poeStatus.includes("short") || poeStatus.includes("overload")
            ? "fault"
            : poeStatus
              ? "off"
              : "unknown",
      poeWatts: p ? num(p["poe-out-power"]) : null,
      apMac: null,
      apName: null,
      // Best-effort: RouterOS does not name the management port, so we only
      // flag it when the host address is literally the interface name.
      isManagement: name === mgmt,
      description: row["comment"] ?? null,
    };
  });

  const used = ports.reduce((sum, p) => sum + (p.poeWatts ?? 0), 0);
  return {
    ports,
    budget: { totalWatts: null, usedWatts: ports.some((p) => p.poeWatts != null) ? used : null },
    measured: true,
    fetchedAt: new Date().toISOString(),
  };
}

/** Read the port list for a registered device, or explain why we cannot. */
export async function readPorts(
  device: DeviceDescriptor,
  ctx: PortDriverContext,
): Promise<SwitchPortView> {
  const cap = capability(device, "ports");
  if (!cap.supported) throw new Error(cap.reason);
  if (device.vendor === "mikrotik") return mikrotikPorts(ctx);
  throw new Error(
    `Reading live ports from ${device.vendor} hardware is not wired up yet — the inventory entry and capability list are available, but port data is not.`,
  );
}

/** Power-cycle PoE on one port. Callers must have passed poeCycleGuard first. */
export async function cyclePoe(
  device: DeviceDescriptor,
  ctx: PortDriverContext,
  port: SwitchPort,
): Promise<{ ok: true }> {
  const cap = capability(device, "poeCycle");
  if (!cap.supported) throw new Error(cap.reason);
  if (device.vendor !== "mikrotik") {
    throw new Error(
      `Power-cycling a ${device.vendor} port is not wired up yet. Use the vendor's own tool for now.`,
    );
  }
  if (!ctx.routerId) throw new Error("This device is not linked to a registered MikroTik router.");
  const { loadRouterConn } = await import("../router-conn.server");
  const { routerAPI } = await import("../mikrotik.server");
  const conn = await loadRouterConn(ctx.supabase, ctx.routerId);
  const id = encodeURIComponent(port.ref);
  await routerAPI.raw(conn, `/interface/ethernet/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ "poe-out": "off" }),
  });
  await new Promise((r) => setTimeout(r, 3000));
  await routerAPI.raw(conn, `/interface/ethernet/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ "poe-out": "auto-on" }),
  });
  return { ok: true };
}
