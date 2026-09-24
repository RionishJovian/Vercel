import { createServerFn } from "@tanstack/react-start";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { z } from "zod";
import type { DatabaseClient } from "./database.types";
import type { FleetRouter } from "./fleet-probe.server";

/** Fleet health snapshot for every router the caller can see. */
export const getFleetHealth = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data: rows, error } = await context.supabase
      .from("router_connections")
      .select("id, name, host, connection_mode, connector_id, is_virtual, site_id");
    if (error) throw new Error(error.message);
    const { filterPhysicalRouters } = await import("./test-router");
    const physical = filterPhysicalRouters(rows ?? []);
    if (!physical.length) return { routers: [] as FleetRouter[], generated_at: Date.now() };

    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const { collectFleetSnapshot } = await import("./fleet-probe.server");
    const routers: FleetRouter[] = await Promise.all(
      physical.map((r) =>
        collectFleetSnapshot(
          {
            id: r.id,
            name: r.name,
            host: r.host,
            connection_mode: r.connection_mode,
            connector_id: r.connector_id,
            site_id: r.site_id ?? null,
          },
          {
            loadConn: (id) => loadRouterConn(context.supabase, id),
            api: {
              ping: routerAPI.ping,
              activeUsers: routerAPI.activeUsers,
              listBindings: routerAPI.listBindings,
              hosts: routerAPI.hosts,
              users: routerAPI.users,
              raw: routerAPI.raw,
            },
          },
        ),
      ),
    );
    const withLogs = await attachSyslogToday(context.supabase, routers);
    return { routers: withLogs, generated_at: Date.now() };
  });

async function attachSyslogToday(
  supabase: DatabaseClient,
  routers: FleetRouter[],
): Promise<FleetRouter[]> {
  const start = new Date();
  start.setHours(0, 0, 0, 0);
  const since = start.toISOString();
  return Promise.all(
    routers.map(async (r) => {
      try {
        const { count, error } = await supabase
          .from("syslog_events")
          .select("id", { count: "exact", head: true })
          .eq("router_id", r.id)
          .gte("received_at", since);
        if (error) return { ...r, syslog_today: null };
        return { ...r, syslog_today: count ?? 0 };
      } catch {
        return { ...r, syslog_today: null };
      }
    }),
  );
}

/** Return latest N scan runs of a given kind for this owner. */
export const listFleetScans = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({ kind: z.enum(["ai"]), limit: z.number().int().min(1).max(50).default(10) })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data: rows, error } = await context.supabase
      .from("fleet_scan_runs")
      .select("id, kind, payload, max_severity, router_count, generated_at")
      .eq("kind", data.kind)
      .order("generated_at", { ascending: false })
      .limit(data.limit);
    if (error) throw new Error(error.message);
    return rows ?? [];
  });

/**
 * Manual AI scan quota lives in ai-quota.server.ts so the AI Insights panel and
 * the Fleet page share one allowance.
 */
import { DEFAULT_MONTHLY_AI_SCAN_LIMIT } from "./ai-scan-quota";
export { DEFAULT_MONTHLY_AI_SCAN_LIMIT };

async function aiScanQuota(context: { supabase: DatabaseClient; userId: string }) {
  const mod = await import("./ai-quota.server");
  return mod.aiScanQuota(context);
}

/** How many manual AI scans the caller has left. */
export const getAiScanQuota = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => aiScanQuota(context));

/** Trigger an AI fleet scan right now, respecting the per-user quota. */
export const runAiScanNow = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const quota = await aiScanQuota(context);
    (await import("./ai-quota.server")).assertScanAllowed(quota);

    const { data: ownerId, error: ownerErr } = await context.supabase.rpc("effective_owner", {
      _user_id: context.userId,
    });
    if (ownerErr) throw new Error(ownerErr.message);

    const { data: rows, error } = await context.supabase
      .from("router_connections")
      .select("id, name, host, connection_mode, connector_id, is_virtual");
    if (error) throw new Error(error.message);
    const { filterPhysicalRouters } = await import("./test-router");
    const physical = filterPhysicalRouters(rows ?? []);
    if (!physical.length) throw new Error("No routers to scan yet — add a router first.");

    const { runAiScanForOwner } = await import("./fleet-ai.server");
    const out = await runAiScanForOwner(
      (ownerId as string) ?? context.userId,
      physical,
      context.userId,
      context.supabase,
    );
    const after = await aiScanQuota(context);
    return { ...out, quota: after };
  });

/**
 * Terminal router picker. Primary users receive only their own tenant's routers;
 * Developers receive all RLS-authorized physical routers with a tenant label but
 * never a host, username, credential, WebFig launcher, or encryption material.
 */
export const listTerminalRouters = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const guards = await import("./guards.server");
    const roles = await guards.getRoles(context.supabase, context.userId);
    const isPlatformAdmin = await guards.isPlatformAdminUser(context.supabase, context.userId);
    if (!guards.isPrivilegedAccount(roles, isPlatformAdmin))
      throw new Error("Terminal access denied.");

    const viewerTenantId = await guards.effectiveOwner(context.supabase, context.userId);
    let query = context.supabase
      .from("router_connections")
      .select("id, name, owner_id, connection_mode, is_virtual")
      .order("created_at", { ascending: true });
    if (!isPlatformAdmin) query = query.eq("owner_id", viewerTenantId);
    const { data, error } = await query;
    if (error) throw new Error(error.message);

    const { filterPhysicalRouters } = await import("./test-router");
    const routers = filterPhysicalRouters(data ?? []);
    const labels = new Map<string, string>();
    if (isPlatformAdmin && routers.length) {
      const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
      const ownerIds = [...new Set(routers.map((router) => router.owner_id))];
      const { data: profiles } = await supabaseAdmin
        .from("profiles")
        .select("id, display_name, username")
        .in("id", ownerIds);
      for (const profile of profiles ?? []) {
        labels.set(
          profile.id,
          profile.display_name || profile.username || `${profile.id.slice(0, 8)}…`,
        );
      }
    }
    return {
      viewerTenantId,
      isPlatformAdmin,
      routers: routers.map((router) => ({
        id: router.id,
        name: router.name,
        ownerId: router.owner_id,
        ownerLabel:
          labels.get(router.owner_id) ??
          (router.owner_id === viewerTenantId ? "Your tenant" : "Tenant"),
        connectionMode: router.connection_mode ?? null,
      })),
    };
  });

/** REST command runner — Primary own-tenant or audited Developer support call. */
export const runTerminalCommand = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        routerId: z.string().uuid(),
        method: z.enum(["GET", "POST", "PATCH", "PUT", "DELETE"]).default("GET"),
        path: z
          .string()
          .min(1)
          .max(300)
          .regex(/^\//, "Path must start with /")
          .refine((p) => !/\s/.test(p), "Path must not contain whitespace"),
        body: z.string().max(4000).optional(),
        supportReason: z.string().trim().min(5).max(300).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    // Terminal is Primary or Developer — every other role is blocked outright.
    const g = await import("./guards.server");
    const roles = await g.getRoles(context.supabase, context.userId);
    const isPlatformAdmin = await g.isPlatformAdminUser(context.supabase, context.userId);
    if (!g.isPrivilegedAccount(roles, isPlatformAdmin)) {
      throw new Error(
        "Terminal access is restricted to Primary café owners and Developers. Contact us on Telegram if you need a command run.",
      );
    }

    // SAFETY GATE: write methods can change live device config (e.g. disable a
    // WAN port). Block all mutating methods; keep read-only GET for diagnosis.
    if (data.method !== "GET") {
      throw new Error(
        "Write operations (POST/PATCH/PUT/DELETE) are disabled for safety to prevent accidental device changes. Read-only GET remains available.",
      );
    }

    const { data: row, error } = await context.supabase
      .from("router_connections")
      .select("id, name, owner_id, site_id")
      .eq("id", data.routerId)
      .maybeSingle();
    if (error) throw new Error(error.message);
    if (!row)
      throw new Error(
        "Access denied: you can only open a terminal for routers assigned to your account.",
      );
    const viewerTenantId = await g.effectiveOwner(context.supabase, context.userId);
    const crossTenant = row.owner_id !== viewerTenantId;
    if (crossTenant && !isPlatformAdmin)
      throw new Error("Access denied: router is outside your tenant.");
    if (crossTenant && data.method !== "GET" && !data.supportReason) {
      throw new Error(
        "A support reason is required before a Developer changes another tenant's router.",
      );
    }
    // Same transport as Test / Live users / Fleet — Magic Hub, Local Connector,
    // Magic Hub, Local Connector, or public IP. Do not dial host:port yourself.
    const { loadRouterConn } = await import("./router-conn.server");
    const { exchange } = await import("./mikrotik.server");
    const conn = await loadRouterConn(context.supabase, row.id);
    const started = Date.now();
    const { status, body: text } = await exchange(conn, data.path, {
      method: data.method,
      body: data.body && data.method !== "GET" ? data.body : undefined,
    });
    const ms = Date.now() - started;
    const { redactText } = await import("./redact.server");
    const snippet = redactText(text).slice(0, 20_000);
    try {
      await context.supabase.from("terminal_history").insert({
        user_id: context.userId,
        owner_id: viewerTenantId,
        router_id: data.routerId,
        site_id: row.site_id ?? null,
        method: data.method,
        path: data.path,
        body: data.body ? redactText(data.body) : null,
        status,
        ms,
        response_snippet: snippet.slice(0, 4000),
      });
    } catch {
      /* audit best-effort */
    }
    if (crossTenant) {
      const { recordRouterOp } = await import("./audit.server");
      await recordRouterOp({
        userId: context.userId,
        ownerId: row.owner_id,
        routerId: row.id,
        routerName: row.name,
        action: data.method === "GET" ? "developer_terminal_read" : "developer_terminal_write",
        outcome: status >= 200 && status < 300 ? "ok" : "failed",
        detail: `${data.method} ${data.path}${data.supportReason ? ` · Reason: ${data.supportReason}` : ""}`,
        durationMs: ms,
      });
    }
    return { status, ms, body: snippet };
  });

/**
 * Owner/admin Apply fix — runs a single-line RouterOS CLI via /execute on the
 * connected board, after sanitizeFixCommand. Audited in terminal_history.
 */
export const applyInsightFix = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        routerId: z.string().uuid(),
        fixCommand: z.string().min(1).max(500),
        insightId: z.string().max(120).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    await (await import("./guards.server")).requirePrivileged(context.supabase, context.userId);

    const { sanitizeFixCommand } = await import("./apply-fix");
    const checked = sanitizeFixCommand(data.fixCommand);
    if (!checked.ok) throw new Error(checked.reason);

    const { data: row, error } = await context.supabase
      .from("router_connections")
      .select("id, name, site_id")
      .eq("id", data.routerId)
      .maybeSingle();
    if (error) throw new Error(error.message);
    if (!row) throw new Error("Router not found or not visible to your account.");

    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const conn = await loadRouterConn(context.supabase, row.id);
    const started = Date.now();
    let status = 200;
    let text = "";
    try {
      const result = await routerAPI.execScript(conn, checked.script);
      text =
        result === undefined || result === null
          ? "ok"
          : typeof result === "string"
            ? result
            : JSON.stringify(result);
    } catch (e) {
      status = 0;
      text = e instanceof Error ? e.message : String(e);
    }
    const ms = Date.now() - started;
    const snippet = text.slice(0, 4000);
    try {
      const ownerId = await (
        await import("./guards.server")
      ).effectiveOwner(context.supabase, context.userId);
      await context.supabase.from("terminal_history").insert({
        user_id: context.userId,
        owner_id: ownerId,
        router_id: data.routerId,
        site_id: row.site_id ?? null,
        method: "POST",
        path: "/execute",
        body: JSON.stringify({
          script: checked.script,
          insight_id: data.insightId ?? null,
          source: "apply_insight_fix",
        }),
        status,
        ms,
        response_snippet: snippet,
      });
    } catch {
      /* audit best-effort */
    }
    if (status === 0) throw new Error(`Apply fix failed on ${row.name}: ${snippet.slice(0, 300)}`);
    return { ok: true as const, ms, body: snippet, routerName: row.name, script: checked.script };
  });
