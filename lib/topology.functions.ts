import { createServerFn } from "@tanstack/react-start";
import { z } from "zod";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { buildSiteTopologySnapshot } from "./topology/build-topology";
import { parseTopologyMagicDudeInsight } from "./topology/magic-dude-insight";
import { probeRouterLanTopology } from "./topology/probe.server";
import type {
  SiteTopologySnapshot,
  TopologyDownstreamDevice,
  TopologyPortLabel,
} from "./topology/types";
import type { DatabaseClient } from "./database.types";

const portLabelSchema = z.object({
  port: z.string().min(1).max(32),
  label: z.string().max(80),
  kind: z.enum(["switch", "ap", "other"]).default("other"),
});

const switchChildPortSchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1).max(32),
  label: z.string().max(80).default(""),
  role: z.enum(["ap", "other", "uplink"]).default("ap"),
  ipAddress: z.string().trim().max(64).nullable().default(null),
  macAddress: z.string().trim().max(32).nullable().default(null),
});

const downstreamDeviceSchema = z.object({
  id: z.string().uuid(),
  parentPort: z.string().min(1).max(32),
  label: z.string().min(1).max(80),
  role: z.enum(["switch", "ap", "other"]).default("other"),
  ipAddress: z.string().trim().max(64).nullable().default(null),
  macAddress: z.string().max(32).nullable().default(null),
  vendorHint: z.string().max(80).nullable().default(null),
  switchPorts: z.array(switchChildPortSchema).max(64).default([]),
});

type TopologyScope = {
  /** Row reader — service role for platform admins, JWT client for tenants. */
  db: DatabaseClient;
  /** Unlock / auth.uid() checks must always use the caller's client. */
  accessClient: DatabaseClient;
  isPlatformAdmin: boolean;
  ownerId: string;
};

/** Any signed-in, non-expired account may open Site topology (tenant-scoped). */
async function assertTopologyAccess(ctx: { supabase: DatabaseClient; userId: string }) {
  const { requireNotExpired } = await import("./guards.server");
  await requireNotExpired(ctx.supabase, ctx.userId);
}

async function resolveTopologyScope(ctx: {
  supabase: DatabaseClient;
  userId: string;
}): Promise<TopologyScope> {
  await assertTopologyAccess(ctx);
  const { isPlatformAdminUser } = await import("./guards.server");
  const isPlatformAdmin = await isPlatformAdminUser(ctx.supabase, ctx.userId);

  if (isPlatformAdmin) {
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    return {
      db: supabaseAdmin,
      accessClient: ctx.supabase,
      isPlatformAdmin: true,
      ownerId: ctx.userId,
    };
  }

  const { data: ownerRow, error: ownerErr } = await ctx.supabase.rpc("effective_owner", {
    _user_id: ctx.userId,
  });
  if (ownerErr) throw new Error(ownerErr.message);
  const ownerId = (ownerRow as string | null) ?? ctx.userId;
  return {
    db: ctx.supabase,
    accessClient: ctx.supabase,
    isPlatformAdmin: false,
    ownerId,
  };
}

/** Route guard — signed-in operators (tenant-scoped) and platform administrators. */
export const assertTopologyPageAccess = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await assertTopologyAccess(context);
  });

/** @deprecated Prefer assertTopologyPageAccess — kept for older imports during rollout. */
export const assertTopologyPlatformAccess = assertTopologyPageAccess;

function parsePortLabels(raw: unknown): TopologyPortLabel[] {
  if (!Array.isArray(raw)) return [];
  const out: TopologyPortLabel[] = [];
  for (const row of raw) {
    const parsed = portLabelSchema.safeParse(row);
    if (parsed.success) out.push(parsed.data);
  }
  return out;
}

function parseDownstreamDevices(raw: unknown): TopologyDownstreamDevice[] {
  if (!Array.isArray(raw)) return [];
  const out: TopologyDownstreamDevice[] = [];
  for (const row of raw) {
    const parsed = downstreamDeviceSchema.safeParse(row);
    if (parsed.success) out.push(parsed.data);
  }
  return out;
}

/** Platform-admin catalog: sites that have at least one physical router. */
export const listTopologySites = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const scope = await resolveTopologyScope(context);
    const { filterPhysicalRouters } = await import("./test-router");

    let routersQuery = scope.db
      .from("router_connections")
      .select("id, name, site_id, owner_id, is_virtual")
      .order("name");
    let sitesQuery = scope.db.from("sites").select("id, name, location, owner_id");
    if (!scope.isPlatformAdmin) {
      routersQuery = routersQuery.eq("owner_id", scope.ownerId);
      sitesQuery = sitesQuery.eq("owner_id", scope.ownerId);
    }

    const [{ data: routers, error: rErr }, { data: sites, error: sErr }] = await Promise.all([
      routersQuery,
      sitesQuery,
    ]);
    if (rErr) throw new Error(rErr.message);
    if (sErr) throw new Error(sErr.message);

    const physical = filterPhysicalRouters(routers ?? []);
    const siteById = new Map((sites ?? []).map((s) => [s.id, s]));

    type Entry = {
      siteId: string;
      siteName: string;
      location: string | null;
      routers: Array<{ id: string; name: string }>;
    };
    const map = new Map<string, Entry>();

    for (const r of physical) {
      if (!r.site_id) continue;
      const site = siteById.get(r.site_id);
      if (!site) continue;
      let entry = map.get(r.site_id);
      if (!entry) {
        entry = {
          siteId: r.site_id,
          siteName: site.name,
          location: site.location ?? null,
          routers: [],
        };
        map.set(r.site_id, entry);
      }
      entry.routers.push({ id: r.id, name: r.name });
    }

    return [...map.values()].sort((a, b) => a.siteName.localeCompare(b.siteName));
  });

export const getSiteTopology = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        siteId: z.string().uuid(),
        routerId: z.string().uuid().optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }): Promise<SiteTopologySnapshot> => {
    const { snapshot } = await loadAuthorizedTopology(data, context);
    return snapshot;
  });

async function loadAuthorizedTopology(
  data: { siteId: string; routerId?: string },
  context: { supabase: DatabaseClient; userId: string },
) {
  const scope = await resolveTopologyScope(context);
  const { loadRouterConn } = await import("./router-conn.server");
  const { filterPhysicalRouters } = await import("./test-router");

  let siteQuery = scope.db.from("sites").select("id, name, owner_id").eq("id", data.siteId);
  if (!scope.isPlatformAdmin) siteQuery = siteQuery.eq("owner_id", scope.ownerId);
  const { data: site, error: siteErr } = await siteQuery.maybeSingle();
  if (siteErr) throw new Error(siteErr.message);
  if (!site) throw new Error("Site not found.");

  let routersQuery = scope.db
    .from("router_connections")
    .select("id, name, site_id, owner_id, is_virtual")
    .eq("site_id", data.siteId);
  if (!scope.isPlatformAdmin) routersQuery = routersQuery.eq("owner_id", scope.ownerId);
  const { data: routers, error: routerErr } = await routersQuery;
  if (routerErr) throw new Error(routerErr.message);

  const physical = filterPhysicalRouters(routers ?? []);
  if (!physical.length) throw new Error("No physical routers on this site.");

  const router =
    physical.find((r) => r.id === data.routerId) ??
    physical.find((r) => r.name.toLowerCase().includes("rb")) ??
    physical[0]!;

  const { data: cfg } = await scope.db
    .from("site_topology_config")
    .select("router_id, port_labels, downstream_devices")
    .eq("site_id", data.siteId)
    .maybeSingle();

  const portLabels = parsePortLabels(cfg?.port_labels);
  const downstreamDevices = parseDownstreamDevices(cfg?.downstream_devices);
  // Catalog reads are platform-admin scoped, but the lock check must carry
  // the authenticated caller. Passing supabaseAdmin here makes auth.uid()
  // null and falsely marks the included router as locked.
  const conn = await loadRouterConn(scope.db, router.id, scope.accessClient);
  const probe = await probeRouterLanTopology(conn);

  const snapshot = buildSiteTopologySnapshot({
    siteId: site.id,
    siteName: site.name,
    routerId: router.id,
    routerName: router.name,
    routerOnline: probe.reachable,
    probe,
    portLabels,
    downstreamDevices,
  });
  return { snapshot, ownerId: router.owner_id as string };
}

const magicDudeInputSchema = z.object({
  siteId: z.string().uuid(),
  routerId: z.string().uuid(),
  nodeId: z.string().min(1).max(128),
});

/**
 * Manual, explanation-only AI investigation for one selected topology node.
 * The server rebuilds the snapshot, so browser-provided topology data is never trusted.
 */
export const investigateTopologyWithMagicDude = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => magicDudeInputSchema.parse(raw))
  .handler(async ({ data, context }) => {
    // Authorize before checking allowance, probing a router, or contacting the AI provider.
    await assertTopologyAccess(context);
    const quotaMod = await import("./ai-quota.server");
    const quotaBefore = await quotaMod.aiScanQuota(context);
    quotaMod.assertScanAllowed(quotaBefore);

    const { snapshot, ownerId } = await loadAuthorizedTopology(data, context);
    const selectedNode = snapshot.nodes.find((node) => node.id === data.nodeId);
    if (!selectedNode) throw new Error("The selected topology node is no longer available.");

    const nodeById = new Map(snapshot.nodes.map((node) => [node.id, node]));
    const connections = snapshot.edges
      .filter((edge) => edge.from === selectedNode.id || edge.to === selectedNode.id)
      .map((edge) => ({
        from: nodeById.get(edge.from)?.label ?? edge.from,
        to: nodeById.get(edge.to)?.label ?? edge.to,
        status: edge.status,
      }));
    const topologyEvidence = {
      site: snapshot.siteName,
      router: snapshot.routerName,
      routerOnline: snapshot.routerOnline,
      observedAt: new Date(snapshot.polledAt).toISOString(),
      selectedNode: {
        kind: selectedNode.kind,
        label: selectedNode.label,
        detail: selectedNode.detail ?? null,
        ipAddress: selectedNode.ipAddress ?? null,
        macAddress: selectedNode.macAddress ?? null,
        vendor: selectedNode.vendor ?? null,
        connection: selectedNode.connection ?? null,
        status: selectedNode.status,
      },
      directConnections: connections,
      guestPool: snapshot.guestPool
        ? {
            name: snapshot.guestPool.name,
            usagePct: snapshot.guestPool.usagePct,
            poolHealth: snapshot.poolHealth,
            healthDetail: snapshot.poolHealthDetail,
          }
        : null,
      probeError: snapshot.probeError,
    };
    const system = `You are Magic Dude, a careful MikroTik network topology assistant.
Analyze only the supplied observed topology evidence for one selected node.
Return ONLY JSON matching this shape:
{"title":"...","summary":"...","evidence":["..."],"impact":"...","recommendation":"...","confidence":"high|medium|low"}
Rules:
- Do not invent devices, causes, traffic, configurations, or measurements.
- Distinguish observed facts from possible causes. If evidence is incomplete, use low confidence.
- Give a safe read-only or physical verification recommendation only.
- Never return RouterOS commands, configuration changes, credentials, or destructive actions.
- Keep wording concise and clear for a network operator.`;

    const { completeChat } = await import("./ai/chat-completions.server");
    let completion: Awaited<ReturnType<typeof completeChat>>;
    try {
      completion = await completeChat(
        [
          { role: "system", content: system },
          { role: "user", content: JSON.stringify(topologyEvidence) },
        ],
        { temperature: 0.1, maxTokens: 320, responseFormat: "json_object" },
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      const status =
        typeof error === "object" && error && "status" in error
          ? Number((error as { status: number }).status)
          : 0;
      if (status === 429) throw new Error("Magic Dude is busy — try again shortly.");
      if (message === "ollama_timeout" || status === 504)
        throw new Error("Magic Dude took too long. Try again shortly.");
      if (message === "not_configured" || status === 503)
        throw new Error("Magic Dude is unavailable right now.");
      throw new Error("Magic Dude could not analyze this device right now.");
    }
    let insight;
    try {
      insight = parseTopologyMagicDudeInsight(completion.answer, selectedNode);
    } catch {
      throw new Error("Magic Dude returned an invalid response. Try again shortly.");
    }

    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { error: scanError } = await supabaseAdmin.from("fleet_scan_runs").insert({
      owner_id: ownerId,
      kind: "ai",
      payload: {
        source: "topology_magic_dude",
        site_id: snapshot.siteId,
        router_id: snapshot.routerId,
        node_id: selectedNode.id,
        node_status: selectedNode.status,
      },
      max_severity:
        selectedNode.status === "down"
          ? "critical"
          : selectedNode.status === "unknown"
            ? "warning"
            : "ok",
      router_count: 1,
      triggered_by: context.userId,
    });
    if (scanError) throw new Error(scanError.message);

    const { logAiUsage } = await import("./ai-usage.server");
    await logAiUsage({
      ownerId,
      userId: context.userId,
      feature: "topology_magic_dude",
      model: completion.model,
      usage: completion.usage,
    });
    return { insight, generatedAt: Date.now(), quota: await quotaMod.aiScanQuota(context) };
  });

export const saveSiteTopologyLabels = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        siteId: z.string().uuid(),
        routerId: z.string().uuid(),
        portLabels: z.array(portLabelSchema),
        downstreamDevices: z.array(downstreamDeviceSchema).max(64).default([]),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const scope = await resolveTopologyScope(context);
    const { getRoles } = await import("./guards.server");
    const roles = await getRoles(context.supabase, context.userId);
    if (roles.includes("read_only")) {
      throw new Error("Your account is read-only, so it can't save topology labels.");
    }

    let siteQuery = scope.db.from("sites").select("id").eq("id", data.siteId);
    if (!scope.isPlatformAdmin) siteQuery = siteQuery.eq("owner_id", scope.ownerId);
    const { data: site, error: siteErr } = await siteQuery.maybeSingle();
    if (siteErr) throw new Error(siteErr.message);
    if (!site) throw new Error("Site not found.");

    let routerQuery = scope.db
      .from("router_connections")
      .select("id")
      .eq("id", data.routerId)
      .eq("site_id", data.siteId);
    if (!scope.isPlatformAdmin) routerQuery = routerQuery.eq("owner_id", scope.ownerId);
    const { data: router, error: routerErr } = await routerQuery.maybeSingle();
    if (routerErr) throw new Error(routerErr.message);
    if (!router) throw new Error("Router not found on this site.");

    const { error } = await scope.db.from("site_topology_config").upsert(
      {
        site_id: data.siteId,
        router_id: data.routerId,
        port_labels: data.portLabels,
        downstream_devices: data.downstreamDevices,
        updated_at: new Date().toISOString(),
      },
      { onConflict: "site_id" },
    );
    if (error) throw new Error(error.message);
    return { ok: true as const };
  });
