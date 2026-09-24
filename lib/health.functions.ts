import { createServerFn } from "@tanstack/react-start";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { z } from "zod";
import { cloudPeerStatusFromHandshake } from "./cloud-peer-status";
import type { RouterProbe } from "./health.server";

// Durable health history, alert rules, incidents and notification preferences.
// Every number the UI shows is either measured and timestamped, or explicitly
// marked unavailable — nothing here fabricates a reading.

const severity = z.enum(["info", "warning", "critical"]);
const incidentKind = z.enum([
  "site_offline",
  "wan_degraded",
  "connector_stale",
  "poe_ap_offline",
  "provisioning_failed",
  "interface_down",
  "dhcp_pool_high",
  "router_anomaly",
  "wan_saturated",
  "vpn_peer_down",
  "router_cpu_high",
  "router_memory_high",
]);

export const listHealthHistory = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        subjectKind: z.enum(["site", "router", "device", "connector"]).optional(),
        subjectId: z.string().max(80).optional(),
        limit: z.number().int().min(1).max(500).optional(),
      })
      .parse(raw ?? {}),
  )
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    let q = context.supabase
      .from("device_health_samples")
      .select(
        "id, subject_kind, subject_id, site_id, device_id, router_id, reachable, latency_ms, uptime_seconds, wan_state, connector_state, tunnel_state, cpu_usage_pct, memory_usage_pct, free_memory_bytes, total_memory_bytes, observed_at",
      )
      .order("observed_at", { ascending: false })
      .limit(data.limit ?? 200);
    if (data.subjectKind) q = q.eq("subject_kind", data.subjectKind);
    if (data.subjectId) q = q.eq("subject_id", data.subjectId);
    const { data: rows, error } = await q;
    if (error) throw new Error(error.message);
    return rows ?? [];
  });

/**
 * Take one reachability sweep, store it, and turn the result into incidents
 * through the shared dedup/cooldown rules. Called from the Incidents page and
 * from the scheduled maintenance hook.
 */
export const runHealthSweep = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const guards = await import("./guards.server");
    await guards.requireNotExpired(context.supabase, context.userId);
    const ownerId = await guards.effectiveOwner(context.supabase, context.userId);
    const { supabase } = context;
    const admin = await import("@/integrations/supabase/client.server");

    const [routersRes, connectorsRes, rulesRes, openRes] = await Promise.all([
      supabase.from("router_connections").select("id, name, site_id, connection_mode"),
      supabase.from("connectors").select("id, name, status, last_seen_at, enabled"),
      supabase.from("alert_rules").select("kind, enabled, severity, cooldown_minutes, threshold"),
      supabase
        .from("incidents")
        .select("id, kind, subject_id, last_notified_at, resolved_at")
        .is("resolved_at", null),
    ]);

    const { routersStatusFor } = await import("./health.server");
    const probes = await routersStatusFor(supabase, routersRes.data ?? []);

    // Fetch cloud peer handshake for hub-mode routers and update DB
    const { vpsRequest } = await import("./cloud-vps.server");
    for (const p of probes) {
      // Find the router row to get connection_mode and cloud_peer_id
      const routerRow = (routersRes.data ?? []).find((r) => r.id === p.id);
      if (!routerRow) continue;

      const isHubMode = routerRow.connection_mode === "hub";
      if (!isHubMode) continue;

      // Need cloud_peer_id from router_connections
      const { data: rc } = await supabase
        .from("router_connections")
        .select("cloud_peer_id")
        .eq("id", p.id)
        .maybeSingle();
      const peerId = rc?.cloud_peer_id as string | null;
      if (!peerId) continue;

      try {
        const peerStatus = await vpsRequest<{
          state?: string;
          lastHandshakeAt?: string | null;
        }>(
          { tenantId: ownerId, routerId: p.id },
          `/v1/peers/${encodeURIComponent(peerId)}`,
          { method: "GET", scopeName: "peers:inspect" },
        );
        const handshakeAt = peerStatus.lastHandshakeAt ?? null;
        const handshakeError = peerStatus.state === "error" ? "Cloud peer error" : null;

        // Attach to probe for UI
        p.cloudHandshakeAt = handshakeAt;
        p.cloudHandshakeError = handshakeError;

        if (handshakeAt) {
          await admin.supabaseAdmin
            .from("router_connections")
            .update({
              cloud_last_handshake_at: handshakeAt,
              cloud_last_seen_at: new Date().toISOString(),
              cloud_status: cloudPeerStatusFromHandshake("enabled", handshakeAt),
              cloud_last_error: handshakeError,
            } as never)
            .eq("id", p.id);
        }

        // Lightweight data-plane check: verify REST actually works through the tunnel
        // Uses /system/identity (tiny, fast) instead of the full 11-path verifyMagicHubRequiredReads
        if (handshakeAt) {
          try {
            const { loadRouterConn } = await import("./router-conn.server");
            const { routerAPI } = await import("./mikrotik.server");
            const conn = await loadRouterConn(admin.supabaseAdmin, p.id);
            await routerAPI.raw(conn, "/system/identity");
            // Success: data plane works, status already set to online above
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            await admin.supabaseAdmin
              .from("router_connections")
              .update({
                cloud_status: "error",
                cloud_last_error: `Handshake fresh but data plane failed: ${msg}`,
              } as never)
              .eq("id", p.id);
          }
        }
      } catch {
        // Attach error to probe
        p.cloudHandshakeError = "Cloud peer handshake check failed";
      }
    }

    const samples = probes.map((p) => ({
      owner_id: ownerId,
      site_id: p.siteId,
      router_id: p.id,
      subject_kind: "router" as const,
      subject_id: p.id,
      reachable: p.online,
      latency_ms: p.latencyMs,
      uptime_seconds: p.uptimeSeconds,
      cpu_usage_pct: p.cpuUsagePct,
      memory_usage_pct: p.memoryUsagePct,
      free_memory_bytes: p.freeMemoryBytes,
      total_memory_bytes: p.totalMemoryBytes,
      wan_state: p.wanState,
      tunnel_state: p.tunnelState,
      connector_state: null,
      detail: {},
    }));

    const now = Date.now();
    for (const c of connectorsRes.data ?? []) {
      if (!c["enabled"]) continue;
      const seen = c["last_seen_at"] ? Date.parse(c["last_seen_at"]) : null;
      const stale = seen === null || now - seen > 10 * 60_000;
      samples.push({
        owner_id: ownerId,
        site_id: null,
        router_id: null,
        subject_kind: "connector" as never,
        subject_id: c["id"],
        reachable: !stale,
        latency_ms: null,
        uptime_seconds: null,
        wan_state: null,
        tunnel_state: null,
        connector_state: stale ? "stale" : "online",
        detail: {},
      } as never);
    }

    if (samples.length) {
      const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
      await supabaseAdmin.from("device_health_samples").insert(samples);
    }

    // Build signals. `consecutive` comes from the stored history so a single
    // blip never raises an incident.
    const { data: recent } = await supabase
      .from("device_health_samples")
      .select(
        "subject_kind, subject_id, reachable, wan_state, connector_state, cpu_usage_pct, memory_usage_pct, observed_at",
      )
      .order("observed_at", { ascending: false })
      .limit(600);

    const { buildSignals } = await import("./health.server");
    const signals = buildSignals(recent ?? [], probes, connectorsRes.data ?? []);

    const { decideIncidents, resolvableIncidents, defaultRules, INCIDENT_KINDS } =
      await import("./alerts/rules");
    const rules = (rulesRes.data ?? []).length
      ? (rulesRes.data ?? []).flatMap((r) => {
          const kind = INCIDENT_KINDS.find((value) => value === r.kind);
          const level = severity.safeParse(r.severity);
          if (!kind || !level.success) return [];
          return [
            {
              kind,
              enabled: r.enabled,
              severity: level.data,
              cooldownMinutes: r.cooldown_minutes,
              threshold: r.threshold,
            },
          ];
        })
      : defaultRules();
    const open = (openRes.data ?? []).flatMap((i) => {
      const kind = INCIDENT_KINDS.find((value) => value === i.kind);
      if (!kind) return [];
      return [
        {
          kind,
          subjectId: i.subject_id,
          lastNotifiedAt: i.last_notified_at,
          resolvedAt: i.resolved_at,
        },
      ];
    });

    const decisions = decideIncidents(signals, rules, open, now);
    const created = decisions.filter((d) => d.action === "create");
    if (created.length) {
      await admin.supabaseAdmin.from("incidents").upsert(
        created.map((d) => ({
          owner_id: ownerId,
          kind: d.signal.kind,
          severity: (d as { severity: string }).severity,
          subject_id: d.signal.subjectId,
          subject_label: d.signal.subjectLabel,
          detail: d.signal.detail ?? null,
          last_seen_at: new Date().toISOString(),
          last_notified_at: new Date().toISOString(),
        })),
        { onConflict: "owner_id,kind,subject_id", ignoreDuplicates: true },
      );
    }

    const toResolve = resolvableIncidents(signals, open);
    if (toResolve.length) {
      for (const inc of toResolve) {
        await admin.supabaseAdmin
          .from("incidents")
          .update({ resolved_at: new Date().toISOString() })
          .eq("owner_id", ownerId)
          .eq("kind", inc.kind)
          .eq("subject_id", inc.subjectId)
          .is("resolved_at", null);
      }
    }

    return {
      sampled: samples.length,
      opened: created.length,
      resolved: toResolve.length,
      suppressed: decisions.length - created.length,
      observedAt: new Date().toISOString(),
    };
  });

export const listIncidents = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z.object({ includeResolved: z.boolean().optional() }).parse(raw ?? {}),
  )
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    let q = context.supabase
      .from("incidents")
      .select(
        "id, kind, severity, subject_id, subject_label, detail, opened_at, last_seen_at, resolved_at, acknowledged_at",
      )
      .order("opened_at", { ascending: false })
      .limit(200);
    if (!data.includeResolved) q = q.is("resolved_at", null);
    const { data: rows, error } = await q;
    if (error) throw new Error(error.message);
    return rows ?? [];
  });

export const acknowledgeIncident = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { error } = await context.supabase
      .from("incidents")
      .update({ acknowledged_at: new Date().toISOString(), acknowledged_by: context.userId })
      .eq("id", data.id);
    if (error) throw new Error(error.message);
    return { ok: true as const };
  });

/** Hide an already-acknowledged incident from the open list. */
export const dismissIncident = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data: row, error: readErr } = await context.supabase
      .from("incidents")
      .select("id, acknowledged_at")
      .eq("id", data.id)
      .maybeSingle();
    if (readErr) throw new Error(readErr.message);
    if (!row?.acknowledged_at) throw new Error("Acknowledge this alert before removing it.");
    const { error } = await context.supabase
      .from("incidents")
      .update({ resolved_at: new Date().toISOString() })
      .eq("id", data.id)
      .not("acknowledged_at", "is", null);
    if (error) throw new Error(error.message);
    return { ok: true as const };
  });

export const listAlertRules = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data, error } = await context.supabase
      .from("alert_rules")
      .select("id, kind, enabled, severity, cooldown_minutes, threshold")
      .order("kind", { ascending: true });
    if (error) throw new Error(error.message);
    if ((data ?? []).length) return data ?? [];
    const { defaultRules } = await import("./alerts/rules");
    return defaultRules().map((r) => ({
      id: null,
      kind: r.kind,
      enabled: r.enabled,
      severity: r.severity,
      cooldown_minutes: r.cooldownMinutes,
      threshold: r.threshold,
    }));
  });

export const saveAlertRule = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        kind: incidentKind,
        enabled: z.boolean(),
        severity,
        cooldownMinutes: z.number().int().min(1).max(1440),
        threshold: z.number().int().min(1).max(20),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const guards = await import("./guards.server");
    await guards.requireNotExpired(context.supabase, context.userId);
    await guards.requireFeature(context.supabase, context.userId, "alerts");
    const ownerId = await guards.effectiveOwner(context.supabase, context.userId);
    const { error } = await context.supabase.from("alert_rules").upsert(
      {
        owner_id: ownerId,
        kind: data.kind,
        enabled: data.enabled,
        severity: data.severity,
        cooldown_minutes: data.cooldownMinutes,
        threshold: data.threshold,
      },
      { onConflict: "owner_id,kind" },
    );
    if (error) throw new Error(error.message);
    return { ok: true as const };
  });

export const getNotificationPrefs = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data } = await context.supabase
      .from("notification_prefs")
      .select("in_app, email, quiet_hours_start, quiet_hours_end, min_severity")
      .maybeSingle();
    return (
      data ?? {
        in_app: true,
        email: false,
        quiet_hours_start: null,
        quiet_hours_end: null,
        min_severity: "warning",
      }
    );
  });

export const saveNotificationPrefs = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        inApp: z.boolean(),
        email: z.boolean(),
        quietHoursStart: z.number().int().min(0).max(23).nullable(),
        quietHoursEnd: z.number().int().min(0).max(23).nullable(),
        minSeverity: severity,
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const guards = await import("./guards.server");
    await guards.requireNotExpired(context.supabase, context.userId);
    const ownerId = await guards.effectiveOwner(context.supabase, context.userId);
    const { error } = await context.supabase.from("notification_prefs").upsert(
      {
        owner_id: ownerId,
        in_app: data.inApp,
        email: data.email,
        quiet_hours_start: data.quietHoursStart,
        quiet_hours_end: data.quietHoursEnd,
        min_severity: data.minSeverity,
      },
      { onConflict: "owner_id" },
    );
    if (error) throw new Error(error.message);
    return { ok: true as const };
  });
