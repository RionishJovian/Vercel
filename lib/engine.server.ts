import { computeVerification, planGatewayBootstrap } from "./planner";
import type {
  BootstrapApplyOutcome,
  BootstrapRollbackOutcome,
  BootstrapSnapshot,
  BootstrapTransport,
  GatewayBootstrapIntent,
} from "./types";
import { parseTag } from "../provisioning/tags";

type SupabaseLike = Parameters<(typeof import("@/lib/router-conn.server"))["loadRouterConn"]>[0];
const sections = [
  "/interface/bridge",
  "/interface/vlan",
  "/interface/bridge/port",
  "/ip/address",
  "/ip/pool",
  "/ip/dhcp-server",
  "/ip/dhcp-server/network",
  "/ip/firewall/nat",
  "/ip/firewall/filter",
];
const rows = async (
  c: Awaited<ReturnType<(typeof import("@/lib/router-conn.server"))["loadRouterConn"]>>,
  path: string,
  discoveryFailures: string[],
) => {
  const { routerAPI } = await import("@/lib/mikrotik.server");
  try {
    const r = await routerAPI.raw<Array<Record<string, string>>>(c, path);
    return Array.isArray(r) ? r : [];
  } catch {
    discoveryFailures.push(path);
    return [];
  }
};

export function createGatewayTransport(supabase: SupabaseLike): BootstrapTransport {
  const conn = async (id: string) =>
    (await import("@/lib/router-conn.server")).loadRouterConn(supabase, id);
  const discover = async (routerId: string): Promise<BootstrapSnapshot> => {
    const { routerAPI } = await import("@/lib/mikrotik.server");
    const c = await conn(routerId);
    const discoveryFailures: string[] = [];
    const [
      resource,
      interfaces,
      bridges,
      ports,
      vlans,
      addresses,
      routes,
      clients,
      servers,
      networks,
      pools,
      nats,
      ...managed
    ] = await Promise.all([
      routerAPI.raw<Record<string, string>>(c, "/system/resource"),
      rows(c, "/interface", discoveryFailures),
      rows(c, "/interface/bridge", discoveryFailures),
      rows(c, "/interface/bridge/port", discoveryFailures),
      rows(c, "/interface/vlan", discoveryFailures),
      rows(c, "/ip/address", discoveryFailures),
      rows(c, "/ip/route", discoveryFailures),
      rows(c, "/ip/dhcp-client", discoveryFailures),
      rows(c, "/ip/dhcp-server", discoveryFailures),
      rows(c, "/ip/dhcp-server/network", discoveryFailures),
      rows(c, "/ip/pool", discoveryFailures),
      rows(c, "/ip/firewall/nat", discoveryFailures),
      ...sections.map((s) => rows(c, s, discoveryFailures)),
    ]);
    const { capabilitiesFor, parseOsVersion } = await import("../provisioning/multi-wan");
    const version = parseOsVersion(resource.version ?? "");
    const rules = managed.flatMap((rs, i) =>
      rs
        .filter((r) => parseTag(r.comment))
        .map((r) => ({
          section: sections[i]!,
          id: r[".id"],
          comment: r.comment!,
          detail: JSON.stringify(r),
        })),
    );
    return {
      routerId,
      identity: resource["board-name"] ?? "router",
      boardName: resource["board-name"] ?? "unknown",
      version,
      capabilities: capabilitiesFor(version),
      interfaces: interfaces.map((r) => ({
        name: r.name ?? "",
        type: r.type ?? "unknown",
        running: r.running === "true",
      })),
      bridges: bridges.map((r) => r.name ?? "").filter(Boolean),
      bridgePorts: ports.map((r) => ({
        bridge: r.bridge ?? "",
        iface: r.interface ?? "",
        dynamic: r.dynamic === "true",
      })),
      vlans: vlans.map((r) => ({
        name: r.name ?? "",
        vlanId: Number(r["vlan-id"] ?? 0),
        iface: r.interface ?? "",
        comment: r.comment,
      })),
      addresses: addresses.map((r) => ({
        iface: r.interface ?? "",
        address: r.address ?? "",
        dynamic: r.dynamic === "true",
        comment: r.comment,
      })),
      routes: routes.map((r) => ({
        dst: r["dst-address"] ?? "",
        gateway: r.gateway ?? "",
        dynamic: r.dynamic === "true",
      })),
      dhcpClients: clients.map((r) => ({ iface: r.interface ?? "" })),
      dhcpServers: servers.map((r) => ({
        name: r.name ?? "",
        iface: r.interface ?? "",
        disabled: r.disabled === "true",
        comment: r.comment,
      })),
      dhcpNetworks: networks.map((r) => ({
        address: r.address ?? "",
        gateway: r.gateway,
        comment: r.comment,
      })),
      pools: pools.map((r) => ({ name: r.name ?? "", ranges: r.ranges ?? "", comment: r.comment })),
      natRules: nats.map((r) => ({
        chain: r.chain ?? "",
        action: r.action ?? "",
        srcAddress: r["src-address"],
        outInterface: r["out-interface"],
        comment: r.comment,
        disabled: r.disabled === "true",
      })),
      rules,
      discoveryFailures: [...new Set(discoveryFailures)],
      sandbox: false,
      takenAt: new Date().toISOString(),
    };
  };
  return {
    name: "routeros-rest",
    discover,
    async backup(id, label) {
      const { routerAPI } = await import("@/lib/mikrotik.server");
      const c = await conn(id);
      const name = `mmagic-${label}`.slice(0, 40);
      await routerAPI.execScript(c, `/system backup save name=${name}\n/export file=${name}`);
      return { id: name, name: `${name}.backup` };
    },
    async run(id, commands) {
      const { routerAPI } = await import("@/lib/mikrotik.server");
      const c = await conn(id);
      const out = [] as Array<{ ok: boolean; command: string; error?: string }>;
      for (const command of commands) {
        try {
          await routerAPI.execScript(c, command);
          out.push({ ok: true, command });
        } catch (e) {
          out.push({ ok: false, command, error: e instanceof Error ? e.message : String(e) });
          break;
        }
      }
      return out;
    },
    async verify(id, intent, tags) {
      return computeVerification(await discover(id), intent, tags);
    },
    async rollback(id, tags) {
      const { routerAPI } = await import("@/lib/mikrotik.server");
      const c = await conn(id);
      let removed = 0;
      const errors: string[] = [];
      for (const section of [...sections].reverse()) {
        let sectionRows: Array<Record<string, string>> = [];
        try {
          const result = await routerAPI.raw<Array<Record<string, string>>>(c, section);
          sectionRows = Array.isArray(result) ? result : [];
        } catch (e) {
          errors.push(`${section} read: ${e instanceof Error ? e.message : String(e)}`);
          continue;
        }
        for (const row of sectionRows) {
          if (!tags.includes(row.comment ?? "") || !row[".id"]) continue;
          try {
            await routerAPI.raw(c, `${section}/${encodeURIComponent(row[".id"])}`, {
              method: "DELETE",
            });
            removed++;
          } catch (e) {
            errors.push(`${section} ${row[".id"]}: ${e instanceof Error ? e.message : String(e)}`);
          }
        }
      }
      return {
        ok: errors.length === 0,
        removed,
        error: errors.length ? errors.join("; ") : undefined,
      };
    },
  };
}

export async function applyGatewayBootstrap(
  transport: BootstrapTransport,
  intent: GatewayBootstrapIntent & { reviewedIntentHash?: string },
): Promise<{ plan: ReturnType<typeof planGatewayBootstrap>; outcome: BootstrapApplyOutcome }> {
  const { reviewedIntentHash, ...planIntent } = intent;
  const snapshot = await transport.discover(planIntent.routerId);
  const plan = planGatewayBootstrap(planIntent, snapshot);
  const base: BootstrapApplyOutcome = {
    ok: false,
    intentHash: plan.intentHash,
    backup: null,
    executed: [],
    verification: null,
    rolledBack: false,
    rollbackRemoved: 0,
    sandbox: plan.sandbox,
  };
  if (reviewedIntentHash && reviewedIntentHash !== plan.intentHash) {
    return {
      plan,
      outcome: {
        ...base,
        error: "The Gateway Bootstrap inputs changed after review. Build and review a new dry run.",
      },
    };
  }
  if (plan.blocked)
    return { plan, outcome: { ...base, error: "Preflight blockers must be resolved." } };
  if (plan.noop) return { plan, outcome: { ...base, ok: true } };
  const backup = await transport.backup(planIntent.routerId, `gateway-${plan.intentHash}`);
  const steps = plan.steps.filter((step) => step.action === "add");
  const executed = await transport.run(
    planIntent.routerId,
    steps.map((step) => step.command),
  );
  const tags = steps.map((step) => step.tag);
  const failed = executed.find((step) => !step.ok);
  if (failed) {
    const rollback = await transport.rollback(planIntent.routerId, tags);
    return {
      plan,
      outcome: {
        ...base,
        backup,
        executed,
        rolledBack: rollback.ok,
        rollbackRemoved: rollback.removed,
        error: failed.error ?? "apply failed",
      },
    };
  }
  const verification = await transport.verify(planIntent.routerId, planIntent, tags);
  if (!verification.ok) {
    const rollback = await transport.rollback(planIntent.routerId, tags);
    return {
      plan,
      outcome: {
        ...base,
        backup,
        executed,
        verification,
        rolledBack: rollback.ok,
        rollbackRemoved: rollback.removed,
        error: "Post-apply validation failed.",
      },
    };
  }
  return { plan, outcome: { ...base, ok: true, backup, executed, verification } };
}
