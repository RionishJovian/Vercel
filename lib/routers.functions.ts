import { createServerFn } from "@tanstack/react-start";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { z } from "zod";

const upsertSchema = z.object({
  id: z.string().uuid().optional(),
  name: z.string().min(1).max(80),
  host: z.string().min(1).max(200),
  port: z.number().int().min(1).max(65535),
  username: z.string().min(1).max(80),
  password: z.string().min(0).max(200),
  useTls: z.boolean(),
  // Verified TLS is the default. A self-signed certificate is an explicit,
  // per-router, reasoned exception that is recorded in the operations audit.
  allowInsecureTls: z.boolean().optional(),
  insecureTlsReason: z.string().max(300).optional(),
  isDefault: z.boolean().optional(),
  siteId: z.string().uuid().nullable().optional(),
  connectorId: z.string().uuid().nullable().optional(),
  /** Form connection method — Magic Hub host is a label, not a cloud dial target. */
  /** "remote" rejected below — Public IP / DDNS removed from the product. */
  connectionMethod: z.enum(["remote", "connector", "hub"]).optional(),
});

export const listRouters = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data: ownerRow, error: ownerErr } = await context.supabase.rpc("effective_owner", {
      _user_id: context.userId,
    });
    if (ownerErr) throw new Error(ownerErr.message);
    const ownerId = (ownerRow as string | null) ?? context.userId;
    const [{ data, error }, { data: routerKeys, error: routerKeysError }] = await Promise.all([
      context.supabase
        .from("router_connections")
        .select(
          "id, name, host, port, username, use_tls, allow_insecure_tls, is_default, created_at, site_id, connection_mode, connector_id, is_virtual, cloud_peer_id",
        )
        .eq("owner_id", ownerId)
        .order("created_at", { ascending: true }),
      context.supabase.rpc("get_router_unlock_keys"),
    ]);
    if (error) throw new Error(error.message);
    if (routerKeysError) throw new Error(routerKeysError.message);
    const { filterPhysicalRouters } = await import("./test-router");
    const rows = filterPhysicalRouters(data ?? []);
    const { getWebfigLaunchAccess, webfigLaunchersForRows } = await import("./webfig.server");
    const webfigAccess = await getWebfigLaunchAccess(context.supabase, context.userId);
    const launchers = await webfigLaunchersForRows(context.supabase, rows, {
      locked: !webfigAccess.allowed,
    });
    const states =
      routerKeys && typeof routerKeys === "object" && !Array.isArray(routerKeys)
        ? ((
            routerKeys as {
              router_states?: Record<string, { locked?: boolean; expires_at?: string }>;
            }
          ).router_states ?? {})
        : {};
    return rows.map((r) => ({
      ...r,
      webfig: launchers.get(r.id) ?? null,
      router_unlock: states[r.id] ?? { locked: false, expires_at: null },
    }));
  });

/** Consume one ready Router key to re-enable a router that was added with a key. */
export const reactivateRouterWithKey = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ routerId: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { data: result, error } = await context.supabase.rpc("reactivate_router_with_key", {
      _router_id: data.routerId,
    });
    if (error) {
      if (error.message.includes("ROUTER_KEY_REQUIRED"))
        throw new Error("Buy a Router key before re-unlocking this router.");
      if (error.message.includes("BASIC_ROUTER_NOT_LOCKABLE"))
        throw new Error("Your included basic router never needs a Router key.");
      throw new Error(error.message);
    }
    return result as { ok: true; router_id: string; expires_at: string };
  });

export const saveRouter = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => upsertSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { encryptSecret } = await import("./crypto.server");
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { requireNotExpired } = await import("./guards.server");
    await requireNotExpired(context.supabase, context.userId);
    // Public IP / DDNS (inbound HTTPS) was removed. Existing direct rows may still
    // be edited when connectionMethod is omitted; new/explicit remote saves are refused.
    if (data.connectionMethod === "remote") {
      throw new Error(
        "Public IP / DDNS has been removed. Use Magic Hub or Local Connector.",
      );
    }

    const logAudit = async (
      success: boolean,
      opts: { ownerId?: string | null; routerId?: string | null; error?: unknown } = {},
    ) => {
      const err = opts.error;
      const message = err instanceof Error ? err.message : err ? String(err) : null;
      const code =
        err && typeof err === "object" && "code" in err
          ? String((err as { code?: unknown }).code ?? "") || null
          : null;
      try {
        await supabaseAdmin.from("router_save_audit").insert({
          user_id: context.userId,
          owner_id: opts.ownerId ?? null,
          action: data.id ? "update" : "insert",
          router_id: opts.routerId ?? data.id ?? null,
          attempted_name: data.name,
          attempted_host: data.host,
          success,
          error_message: message,
          error_code: code,
        });
      } catch (auditErr) {
        console.error("[audit] failed to record router_save_audit", auditErr);
      }
    };

    // Cloud Remote: refuse private/SSRF targets. Local Connector: require LAN IPv4.
    // Magic Hub: host is a label only — allow CGNAT/private shapes; cloud never dials it.
    const { assertSafeEndpoint, assertHubHostLabel, EndpointError } =
      await import("./net/endpoint.server");
    const { assertConnectorLanEndpoint } = await import("./connector-guard.server");
    const { recordRouterOp } = await import("./audit.server");

    let existingMode: string | null = null;
    if (data.id) {
      const { data: existingModeRow } = await context.supabase
        .from("router_connections")
        .select("connection_mode")
        .eq("id", data.id)
        .maybeSingle();
      existingMode = existingModeRow?.connection_mode ?? null;
    }
    const hubLabel =
      data.connectionMethod === "hub" || existingMode === "hub" || existingMode === "cloud";

    try {
      if (hubLabel) {
        assertHubHostLabel(data.host, data.port);
      } else if (data.connectorId) {
        assertConnectorLanEndpoint(data.host, data.port);
      } else {
        await assertSafeEndpoint(data.host, data.port);
      }
    } catch (e) {
      await recordRouterOp({
        userId: context.userId,
        routerId: data.id ?? null,
        routerName: data.name,
        action: "endpoint_validation_failed",
        outcome: "blocked",
        detail: `host rejected during ${data.id ? "update" : "create"}`,
        error: e,
      });
      await logAudit(false, { routerId: data.id ?? null, error: e });
      throw e instanceof EndpointError ? new Error(e.message) : e;
    }

    const wantsInsecureTls = data.allowInsecureTls === true;
    const insecureReason = (data.insecureTlsReason ?? "").trim();
    if (wantsInsecureTls && !insecureReason)
      throw new Error(
        "Accepting a self-signed certificate needs a short reason so it can be audited.",
      );

    let ownerId: string | null = null;
    try {
      const { data: ownerRow, error: ownerErr } = await context.supabase.rpc("effective_owner", {
        _user_id: context.userId,
      });
      if (ownerErr) throw new Error(ownerErr.message);
      ownerId = (ownerRow as string | null) ?? context.userId;

      // Soft-resolve site: a leaked/stale foreign siteId must not block saving the router.
      // Explicit assignRouterToSite still hard-asserts ownership.
      const { resolveOwnedSiteId } = await import("./sites-ownership.server");
      const ownedSiteId =
        data.siteId !== undefined
          ? await resolveOwnedSiteId(context.supabase, ownerId, data.siteId)
          : undefined;

      // If password is empty on update, keep the existing one.
      let password_ciphertext: string | undefined;
      if (data.password && data.password.length > 0) {
        password_ciphertext = encryptSecret(data.password);
      }

      if (data.id) {
        const { assertRouterUnlockActive } = await import("./router-unlock-key.server");
        await assertRouterUnlockActive(context.supabase, data.id);
        const { assertNotVirtualRouter } = await import("./test-router");
        assertNotVirtualRouter(
          { connection_mode: existingMode },
          "be edited from the Routers form",
        );
        const { routerCredentialPatch, commitRouterUpdate } =
          await import("./router-credentials.server");

        const patch = {
          name: data.name,
          host: data.host,
          port: data.port,
          use_tls: data.useTls,
          allow_insecure_tls: wantsInsecureTls,
          insecure_tls_reason: wantsInsecureTls ? insecureReason : null,
          insecure_tls_approved_by: wantsInsecureTls ? context.userId : null,
          insecure_tls_approved_at: wantsInsecureTls ? new Date().toISOString() : null,
          is_default: data.isDefault ?? false,
          ...(ownedSiteId !== undefined ? { site_id: ownedSiteId } : {}),
          ...(data.connectorId !== undefined ? { connector_id: data.connectorId } : {}),
          // Username always updates; a blank edit password preserves the
          // stored ciphertext, a nonempty one re-encrypts (AES-256-GCM).
          ...routerCredentialPatch({ username: data.username, password: data.password }),
          ...(data.connectionMethod === "hub"
            ? { connection_mode: "hub" as const, connector_id: null }
            : data.connectionMethod === "connector"
              ? { connection_mode: "direct" as const }
              : data.connectionMethod === "remote"
                ? { connection_mode: "direct" as const, connector_id: null }
                : {}),
        };
        // Prove exactly one RLS-visible row was committed before claiming
        // success — a hidden/missing row must surface as a save failure.
        await commitRouterUpdate(context.supabase, data.id, patch);
        await logAudit(true, { ownerId, routerId: data.id });

        // Editing into Magic Hub with no peer yet should issue the paste script
        // the same way Add router does (otherwise operators only get "Router updated").
        if (data.connectionMethod === "hub") {
          const { data: hubRow } = await context.supabase
            .from("router_connections")
            .select("cloud_peer_id")
            .eq("id", data.id)
            .maybeSingle();
          if (!hubRow?.cloud_peer_id) {
            try {
              const { runProvisionHubPeer } = await import("./cloud-router.functions");
              const hub = await runProvisionHubPeer(context.supabase, context.userId, data.id);
              return { id: data.id, hub };
            } catch (hubErr) {
              return {
                id: data.id,
                hub: null,
                hubError: hubErr instanceof Error ? hubErr.message : String(hubErr),
              };
            }
          }
        }
        return { id: data.id };
      }

      if (!password_ciphertext) throw new Error("Password is required for a new router");
      const guards = await import("./guards.server");
      {
        await guards.requireNotExpired(context.supabase, context.userId);
        // Double-submit / retry guard: an identical router already saved by this
        // account is returned as-is instead of inserting a duplicate.
        const dup = await guards.findDuplicate(context.supabase, ownerId, "routers", {
          name: data.name,
          host: data.host,
          port: data.port,
        });
        if (dup) {
          await logAudit(true, { ownerId, routerId: dup });
          // Dedupe must still issue Magic Hub paste script when Add was hub-mode
          // (otherwise the toast claims success and no window opens).
          if (data.connectionMethod === "hub") {
            try {
              const { runProvisionHubPeer } = await import("./cloud-router.functions");
              const hub = await runProvisionHubPeer(context.supabase, context.userId, dup, {
                reissueScript: true,
              });
              return { id: dup, deduped: true as const, hub };
            } catch (hubErr) {
              return {
                id: dup,
                deduped: true as const,
                hub: null,
                hubError: hubErr instanceof Error ? hubErr.message : String(hubErr),
              };
            }
          }
          return { id: dup, deduped: true as const };
        }
        await guards.enforceDeviceQuota(context.supabase, context.userId, "routers");
      }
      const { data: inserted, error } = await context.supabase
        .from("router_connections")
        .insert({
          owner_id: ownerId,
          name: data.name,
          host: data.host,
          port: data.port,
          username: data.username,
          password_ciphertext,
          use_tls: data.useTls,
          allow_insecure_tls: wantsInsecureTls,
          insecure_tls_reason: wantsInsecureTls ? insecureReason : null,
          insecure_tls_approved_by: wantsInsecureTls ? context.userId : null,
          insecure_tls_approved_at: wantsInsecureTls ? new Date().toISOString() : null,
          is_default: data.isDefault ?? false,
          site_id: ownedSiteId ?? null,
          connector_id: data.connectorId ?? null,
          ...(data.connectionMethod === "hub" ? { connection_mode: "hub" } : {}),
        })
        .select("id")
        .single();
      if (error) throw guards.friendlyDeviceError(error, "routers");
      await logAudit(true, { ownerId, routerId: inserted.id });
      if (wantsInsecureTls)
        await recordRouterOp({
          userId: context.userId,
          ownerId,
          routerId: inserted.id,
          routerName: data.name,
          action: "tls_exception_granted",
          outcome: "ok",
          detail: `enabled at creation: ${insecureReason}`,
        });
      if (data.connectionMethod !== "hub") return { id: inserted.id };
      try {
        const { runProvisionHubPeer } = await import("./cloud-router.functions");
        const hub = await runProvisionHubPeer(context.supabase, context.userId, inserted.id);
        return { id: inserted.id, hub };
      } catch (hubErr) {
        return {
          id: inserted.id,
          hub: null,
          hubError: hubErr instanceof Error ? hubErr.message : String(hubErr),
        };
      }
    } catch (err) {
      await logAudit(false, { ownerId, error: err });
      throw err instanceof Error ? err : new Error(String(err));
    }
  });

export const listRouterAudit = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z.object({ page: z.number().int().min(1).default(1) }).parse(raw ?? {}),
  )
  .handler(async ({ data: input, context }) => {
    const { data, error, count } = await context.supabase
      .from("router_save_audit")
      .select(
        "id, user_id, owner_id, action, router_id, attempted_name, attempted_host, success, error_message, error_code, created_at",
        { count: "exact" },
      )
      .order("created_at", { ascending: false })
      .range((input.page - 1) * 100, input.page * 100 - 1);
    if (error) throw new Error(error.message);
    const rows = data ?? [];
    if (rows.length === 0)
      return {
        rows: [] as Array<
          (typeof rows)[number] & { user_name: string | null; user_email: string | null }
        >,
        total: count ?? 0,
      };

    // Owners/admins see real account names instead of raw user IDs.
    const names = new Map<string, { name: string | null; email: string | null }>();
    try {
      const { requirePrivileged } = await import("./guards.server");
      await requirePrivileged(context.supabase, context.userId);
      const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
      const ids = [...new Set(rows.map((r) => r.user_id).filter(Boolean))] as string[];
      const [{ data: profiles }, { data: authList }] = await Promise.all([
        supabaseAdmin.from("profiles").select("id, username, display_name").in("id", ids),
        supabaseAdmin.auth.admin.listUsers({ page: 1, perPage: 200 }),
      ]);
      const emailById = new Map<string, string | null>(
        (authList?.users ?? []).map((u) => [u.id, u.email ?? null]),
      );
      for (const p of profiles ?? []) {
        names.set(p.id, {
          name: (p.username as string | null) ?? (p.display_name as string | null) ?? null,
          email: emailById.get(p.id) ?? null,
        });
      }
    } catch {
      // Non-privileged callers simply get no name enrichment.
    }

    return {
      rows: rows.map((r) => ({
        ...r,
        user_name: names.get(r.user_id)?.name ?? null,
        user_email: names.get(r.user_id)?.email ?? null,
      })),
      total: count ?? 0,
    };
  });

/**
 * Owner-only: ask the AI to explain one failed audit entry and give the
 * RouterOS / app-side steps that fix it.
 */
export const explainRouterAuditError = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { requirePrivileged } = await import("./guards.server");
    await requirePrivileged(context.supabase, context.userId);

    const { data: row, error } = await context.supabase
      .from("router_save_audit")
      .select("action, attempted_name, attempted_host, error_message, error_code, created_at")
      .eq("id", data.id)
      .maybeSingle();
    if (error) throw new Error(error.message);
    if (!row) throw new Error("Audit entry not found.");
    if (!row.error_message)
      return {
        summary: "This attempt succeeded — nothing to diagnose.",
        steps: [] as string[],
        command: null as string | null,
      };

    const key = process.env["LOVABLE_API_KEY"];
    if (!key) throw new Error("AI is not configured for this project.");

    const { diagnoseAuditError } = await import("./audit-diagnosis");
    return diagnoseAuditError(
      {
        action: row.action,
        attempted_name: row.attempted_name,
        attempted_host: row.attempted_host,
        error_code: row.error_code,
        error_message: row.error_message,
      },
      key,
    );
  });

export const deleteRouter = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        id: z.string().uuid(),
        confirmation: z.string().max(80).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const guards = await import("./guards.server");
    await guards.requireNotExpired(context.supabase, context.userId);
    // Removal stays available even when a Router key expired — an operator must
    // always be able to take a device off their account.
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");

    const { data: row } = await context.supabase
      .from("router_connections")
      .select("name, host, owner_id, connection_mode, cloud_peer_id")
      .eq("id", data.id)
      .maybeSingle();

    const { routerNeedsTypedRemoval, assertRemoveDeviceConfirmation } =
      await import("./device-removal");
    if (
      routerNeedsTypedRemoval({
        connectionMode: row?.connection_mode,
        cloudPeerId: row?.cloud_peer_id,
      })
    ) {
      assertRemoveDeviceConfirmation(data.confirmation);
    }

    if (row?.cloud_peer_id) {
      try {
        const { runTeardownHubPeer } = await import("./cloud-router.functions");
        await runTeardownHubPeer(context.supabase, context.userId, data.id);
      } catch (hubErr) {
        console.error("[hub] teardown on delete", hubErr);
      }
    }

    // A Router key activation points at the router it unlocked with ON DELETE
    // RESTRICT. Release that binding first, otherwise the delete always fails
    // with a raw foreign-key error and the router is stuck on the account.
    try {
      await supabaseAdmin
        .from("router_unlock_key_activations")
        .update({ consumed_router_id: null, consumed_at: null })
        .eq("consumed_router_id", data.id);
    } catch (unlockErr) {
      console.error("[router] failed to release unlock key binding", unlockErr);
    }

    const { error } = await context.supabase.from("router_connections").delete().eq("id", data.id);

    try {
      await supabaseAdmin.from("router_save_audit").insert({
        user_id: context.userId,
        owner_id: row?.owner_id ?? null,
        action: "delete",
        router_id: data.id,
        attempted_name: row?.name ?? null,
        attempted_host: row?.host ?? null,
        success: !error,
        error_message: error?.message ?? null,
        error_code: error?.code ?? null,
      });
    } catch (auditErr) {
      console.error("[audit] failed to record router delete", auditErr);
    }

    if (error) throw new Error(error.message);
    return { ok: true };
  });

/** Latest connection-test outcome per router, read from the shared audit log. */
export const listRouterTests = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const { data, error } = await context.supabase
      .from("router_save_audit")
      .select("router_id, success, error_message, created_at")
      .eq("action", "test")
      .order("created_at", { ascending: false })
      .limit(300);
    if (error) throw new Error(error.message);
    const latest: Record<string, { success: boolean; error: string | null; at: string }> = {};
    for (const row of data ?? []) {
      if (!row.router_id || latest[row.router_id]) continue;
      latest[row.router_id] = {
        success: row.success,
        error: row.error_message,
        at: row.created_at,
      };
    }
    return latest;
  });

/** Owner/admin only: reboot a RouterBoard. Every attempt is audited. */
export const rebootRouter = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const guards = await import("./guards.server");
    await guards.requireFeature(context.supabase, context.userId, "reboot");
    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");

    const { data: row } = await context.supabase
      .from("router_connections")
      .select("name, host, owner_id, connection_mode")
      .eq("id", data.id)
      .maybeSingle();

    const { assertNotVirtualRouter } = await import("./test-router");
    assertNotVirtualRouter(row, "be rebooted");

    let ok = true;
    let message: string | null = null;
    try {
      const conn = await loadRouterConn(context.supabase, data.id);
      await routerAPI.execScript(conn, "/system reboot");
    } catch (e) {
      ok = false;
      message = e instanceof Error ? e.message : String(e);
    }

    try {
      await supabaseAdmin.from("router_save_audit").insert({
        user_id: context.userId,
        owner_id: row?.owner_id ?? null,
        action: "reboot",
        router_id: data.id,
        attempted_name: row?.name ?? null,
        attempted_host: row?.host ?? null,
        success: ok,
        error_message: message,
        error_code: null,
      });
    } catch (auditErr) {
      console.error("[audit] failed to record router reboot", auditErr);
    }

    if (!ok) throw new Error(message ?? "Reboot failed");
    return { ok: true };
  });

export type TestStep = {
  name: string;
  status: "ok" | "fail" | "warn" | "skip";
  detail?: string;
};

const testConnectionSchema = z.object({
  host: z.string().min(1).max(200),
  port: z.number().int().min(1).max(65535),
  username: z.string().min(1).max(80),
  password: z.string().min(1).max(200),
  useTls: z.boolean(),
});

export const testConnection = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => testConnectionSchema.parse(raw))
  .handler(async ({ data }) => {
    const { routerAPI } = await import("./mikrotik.server");
    const net = await import("node:net");
    const tls = await import("node:tls");
    const dns = await import("node:dns/promises");

    const steps: TestStep[] = [];
    const remediation: string[] = [];
    const host = data.host.trim();
    const port = data.port;
    const scheme = data.useTls ? "https" : "http";
    const endpoint = `${scheme}://${host}:${port}/rest/system/resource`;

    let ip: string | null = null;
    const ipLiteral = /^\d{1,3}(\.\d{1,3}){3}$|^\[?[0-9a-f:]+\]?$/i.test(host);
    if (ipLiteral) {
      ip = host;
      steps.push({ name: "DNS resolution", status: "skip", detail: "Host is an IP literal" });
    } else {
      try {
        const r = await dns.lookup(host);
        ip = r.address;
        steps.push({ name: "DNS resolution", status: "ok", detail: `${host} → ${ip}` });
      } catch (e) {
        steps.push({ name: "DNS resolution", status: "fail", detail: (e as Error).message });
        remediation.push(`Hostname "${host}" does not resolve. Verify the DDNS name or A record.`);
        return { ok: false, endpoint, steps, remediation, error: "DNS resolution failed" };
      }
    }

    const tcpOk = await new Promise<{ ok: boolean; err?: string }>((resolve) => {
      const sock = new net.Socket();
      const timer = setTimeout(() => {
        sock.destroy();
        resolve({ ok: false, err: "timeout after 8s" });
      }, 8_000);
      sock.once("connect", () => {
        clearTimeout(timer);
        sock.destroy();
        resolve({ ok: true });
      });
      sock.once("error", (e) => {
        clearTimeout(timer);
        resolve({ ok: false, err: e.message });
      });
      sock.connect(port, ip!);
    });
    if (!tcpOk.ok) {
      steps.push({ name: `TCP connect ${ip}:${port}`, status: "fail", detail: tcpOk.err });
      remediation.push(
        `Port ${port} is not reachable. Enable /ip service ${data.useTls ? "www-ssl" : "www"} and open the firewall.`,
      );
      return { ok: false, endpoint, steps, remediation, error: `TCP ${port} closed` };
    }
    steps.push({ name: `TCP connect ${ip}:${port}`, status: "ok" });

    if (data.useTls) {
      const tlsRes = await new Promise<{ ok: boolean; err?: string }>((resolve) => {
        const sock = tls.connect({
          host: ip!,
          port,
          servername: ipLiteral ? undefined : host,
          rejectUnauthorized: false,
          timeout: 8_000,
        });
        sock.once("secureConnect", () => {
          sock.destroy();
          resolve({ ok: true });
        });
        sock.once("timeout", () => {
          sock.destroy();
          resolve({ ok: false, err: "TLS handshake timeout" });
        });
        sock.once("error", (e) => resolve({ ok: false, err: e.message }));
      });
      if (!tlsRes.ok) {
        steps.push({ name: "TLS handshake", status: "fail", detail: tlsRes.err });
        remediation.push(
          "TLS handshake failed. Regenerate the certificate and bind it to www-ssl.",
        );
        return { ok: false, endpoint, steps, remediation, error: "TLS failed" };
      }
      steps.push({ name: "TLS handshake", status: "ok" });
    }

    try {
      const info = await routerAPI.ping({
        host,
        port,
        username: data.username,
        password: data.password,
        useTls: data.useTls,
      });
      const version = (info as { version?: string })?.version;
      steps.push({
        name: `GET ${endpoint}`,
        status: "ok",
        detail: version ? `RouterOS ${version}` : "200 OK",
      });
      return { ok: true as const, endpoint, steps, remediation, info };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      steps.push({ name: `GET ${endpoint}`, status: "fail", detail: msg });
      if (/\b401\b|unauthorized/i.test(msg)) {
        remediation.push(
          `Auth rejected. Verify the API user "${data.username}" and password on the router.`,
        );
        remediation.push("Ensure the user's group has 'api,rest-api,read,write' policies.");
      } else if (/\b403\b|forbidden/i.test(msg)) {
        remediation.push(
          "Authenticated but forbidden. Add the 'rest-api' policy to the user's group.",
        );
      } else if (/\b404\b/.test(msg)) {
        remediation.push("/rest not found. RouterOS 7.1+ is required.");
      }
      return { ok: false as const, endpoint, steps, remediation, error: msg };
    }
  });

export const testRouter = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const run = async () => {
      const { decryptSecret } = await import("./crypto.server");

      const { routerAPI } = await import("./mikrotik.server");

      const { data: row, error } = await context.supabase
        .from("router_connections")
        .select("*")
        .eq("id", data.id)
        .single();
      if (error || !row) throw new Error(error?.message ?? "Router not found");

      const { selectTestMethod, testViaConnector, testViaCloud, testViaSandbox } =
        await import("./router-test.server");
      const method = selectTestMethod(row);

      // Leftover virtual-lab rows: never dial a host, hub, or LAN address.
      if (method === "sandbox") {
        return await testViaSandbox(row);
      }

      // Local Connector: the device lives on the customer LAN. Never dial the
      // stored host from the cloud — go through the connector transport.
      if (method === "connector") {
        const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
        const { loadRouterConn } = await import("./router-conn.server");
        const password = decryptSecret(row.password_ciphertext);
        return await testViaConnector(row, {
          secrets: [password, row.username],
          loadConnector: async (id) => {
            const { data: c } = await supabaseAdmin
              .from("connectors")
              .select("id, name, enabled, status, last_seen_at")
              .eq("id", id)
              .eq("owner_id", row.owner_id)
              .maybeSingle();
            return c ?? null;
          },
          ping: async () => {
            const conn = await loadRouterConn(context.supabase, row.id);
            return (await routerAPI.ping(conn)) as import("./router-test.server").RouterInfo;
          },
        });
      }

      // Magic Hub: the hub proxies REST; the public host is irrelevant.
      if (method === "hub") {
        const cloud = await import("./cloud-vps.server");
        const password = decryptSecret(row.password_ciphertext);
        return await testViaCloud(row, {
          secrets: [password, row.username],
          isConfigured: () => cloud.isCloudConfigured(),
          restBase: (peerId) => cloud.cloudRestBase(peerId),
          peerStatus: (peerId) =>
            cloud.peerStatus(
              { tenantId: row.owner_id as string, routerId: row.id as string },
              peerId,
            ),
          ping: async (restBase) =>
            (await routerAPI.ping({
              host: row.host,
              port: row.port,
              username: row.username,
              password,
              useTls: row.use_tls,
              allowInsecureTls: row.allow_insecure_tls === true,
              baseUrlOverride: restBase,
            })) as import("./router-test.server").RouterInfo,
        });
      }

      const net = await import("node:net");
      const tls = await import("node:tls");
      const dns = await import("node:dns/promises");

      const steps: TestStep[] = [];
      const remediation: string[] = [];
      const host = row.host;
      const port = row.port;
      const scheme = row.use_tls ? "https" : "http";
      const endpoint = `${scheme}://${host}:${port}/rest/system/resource`;

      // 1) DNS
      let ip: string | null = null;
      const ipLiteral = /^\d{1,3}(\.\d{1,3}){3}$|^\[?[0-9a-f:]+\]?$/i.test(host);
      if (ipLiteral) {
        ip = host;
        steps.push({ name: "DNS resolution", status: "skip", detail: "Host is an IP literal" });
      } else {
        try {
          const r = await dns.lookup(host);
          ip = r.address;
          steps.push({ name: "DNS resolution", status: "ok", detail: `${host} → ${ip}` });
          if (
            /^(10\.|192\.168\.|127\.|169\.254\.|172\.(1[6-9]|2\d|3[01])\.|100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.)/.test(
              ip,
            )
          ) {
            steps.push({
              name: "Public IP check",
              status: "warn",
              detail: `${ip} is private/CGNAT — unreachable from this server`,
            });
            remediation.push(
              "Your DDNS name resolves to a private or CGNAT address. Request a public IP from your ISP, or route through a VPN/tunnel (WireGuard, ZeroTier).",
            );
          }
        } catch (e) {
          steps.push({ name: "DNS resolution", status: "fail", detail: (e as Error).message });
          remediation.push(
            `Hostname "${host}" doesn't resolve. On the router run /ip cloud print and confirm status=updated, then copy the dns-name exactly (no https://, no port, no path).`,
          );
          remediation.push(
            "If using a custom domain, point an A record directly at the router's public IP and disable Cloudflare proxy (grey cloud, not orange).",
          );
          return { ok: false, endpoint, steps, remediation, error: "DNS resolution failed" };
        }
      }

      // 2) TCP reachability
      const tcpOk = await new Promise<{ ok: boolean; err?: string }>((resolve) => {
        const sock = new net.Socket();
        const timer = setTimeout(() => {
          sock.destroy();
          resolve({ ok: false, err: "timeout after 8s" });
        }, 8_000);
        sock.once("connect", () => {
          clearTimeout(timer);
          sock.destroy();
          resolve({ ok: true });
        });
        sock.once("error", (e) => {
          clearTimeout(timer);
          resolve({ ok: false, err: e.message });
        });
        sock.connect(port, ip!);
      });
      if (!tcpOk.ok) {
        steps.push({ name: `TCP connect ${ip}:${port}`, status: "fail", detail: tcpOk.err });
        remediation.push(
          `Port ${port} is not reachable. On the router enable /ip service ${row.use_tls ? "www-ssl" : "www"} and confirm it listens on port ${port}.`,
        );
        remediation.push(
          `Open the firewall: /ip firewall filter add chain=input protocol=tcp dst-port=${port} action=accept place-before=0 comment="mikrotik-magic REST"`,
        );
        remediation.push(
          "If your ISP blocks inbound ports, switch to a different port (e.g. 8443) or use a VPN tunnel.",
        );
        return { ok: false, endpoint, steps, remediation, error: `TCP ${port} closed` };
      }
      steps.push({ name: `TCP connect ${ip}:${port}`, status: "ok" });

      // 3) TLS handshake (only if https)
      if (row.use_tls) {
        const tlsRes = await new Promise<{ ok: boolean; err?: string; issuer?: string }>(
          (resolve) => {
            const sock = tls.connect({
              host: ip!,
              port,
              servername: ipLiteral ? undefined : host,
              rejectUnauthorized: false,
              timeout: 8_000,
            });
            sock.once("secureConnect", () => {
              const cert = sock.getPeerCertificate();
              sock.destroy();
              const cn = cert?.issuer?.CN ?? cert?.subject?.CN;
              resolve({ ok: true, issuer: Array.isArray(cn) ? cn[0] : cn });
            });
            sock.once("timeout", () => {
              sock.destroy();
              resolve({ ok: false, err: "TLS handshake timeout" });
            });
            sock.once("error", (e) => resolve({ ok: false, err: e.message }));
          },
        );
        if (!tlsRes.ok) {
          steps.push({ name: "TLS handshake", status: "fail", detail: tlsRes.err });
          remediation.push(
            "TLS handshake failed. On the router regenerate the certificate: /certificate add name=mikrotik-magic common-name=<your-ddns> key-usage=tls-server && /certificate sign mikrotik-magic",
          );
          remediation.push(
            `Then bind it to www-ssl: /ip service set www-ssl certificate=mikrotik-magic disabled=no port=${port}`,
          );
          return { ok: false, endpoint, steps, remediation, error: "TLS failed" };
        }
        steps.push({
          name: "TLS handshake",
          status: "ok",
          detail: tlsRes.issuer ? `cert CN: ${tlsRes.issuer}` : undefined,
        });
      }

      // 4) REST /system/resource with auth
      try {
        const info = await routerAPI.ping({
          host: row.host,
          port: row.port,
          username: row.username,
          password: decryptSecret(row.password_ciphertext),
          useTls: row.use_tls,
        });
        steps.push({
          name: `GET ${endpoint}`,
          status: "ok",
          detail: (info as { version?: string })?.version
            ? `RouterOS ${(info as { version?: string }).version}`
            : "200 OK",
        });
        return { ok: true, endpoint, steps, remediation, info };
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        steps.push({ name: `GET ${endpoint}`, status: "fail", detail: msg });
        if (/\b401\b|unauthorized/i.test(msg)) {
          remediation.push(
            `Auth rejected. Verify the API user exists: /user print where name=${row.username}`,
          );
          remediation.push(
            "Ensure the user's group has 'api,rest-api,read,write' policies: /user group print",
          );
          remediation.push(
            `If unsure, recreate: /user add name=${row.username} group=full password=<newpw> — then update the password here.`,
          );
        } else if (/\b403\b|forbidden/i.test(msg)) {
          remediation.push(
            "User is authenticated but lacks REST permission. Add the 'rest-api' policy to their group.",
          );
          remediation.push(
            "Check /ip service www-ssl address= isn't restricting to a LAN subnet that excludes this server.",
          );
        } else if (/\b404\b/.test(msg)) {
          remediation.push(
            "/rest endpoint not found. RouterOS 7.1+ is required. Upgrade: /system package update check-for-updates then install.",
          );
        } else if (/certificate|self.?signed|cert/i.test(msg)) {
          remediation.push(
            "Certificate issue. Toggle 'Allow insecure TLS' on this router row, or install a valid cert on www-ssl.",
          );
        } else {
          remediation.push(
            "REST call failed after TCP/TLS succeeded. Check /log print on the router for www-ssl errors around this timestamp.",
          );
        }
        return { ok: false, endpoint, steps, remediation, error: msg };
      }
    };

    const raw = (await run()) as Partial<import("./router-test.server").TransportTestResult> & {
      ok: boolean;
      endpoint: string;
      steps: TestStep[];
      remediation: string[];
      info?: unknown;
    };
    const { selectTestMethod, sanitizeReason } = await import("./router-test.server");

    let auditMethod: import("./router-test.server").TestMethod = raw.method ?? "direct";
    try {
      const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
      const { data: row } = await context.supabase
        .from("router_connections")
        .select("name, host, owner_id, connector_id, connection_mode, username")
        .eq("id", data.id)
        .maybeSingle();
      auditMethod = raw.method ?? (row ? selectTestMethod(row) : "direct");
      const reason = raw.reason ?? (raw.ok ? null : "unknown");
      const sanitized = raw.error ? sanitizeReason(raw.error, [row?.username]) : null;
      await supabaseAdmin.from("router_save_audit").insert({
        user_id: context.userId,
        owner_id: row?.owner_id ?? null,
        action: "test",
        router_id: data.id,
        attempted_name: row?.name ?? null,
        attempted_host: row?.host ?? null,
        success: raw.ok,
        error_message: sanitized ? `[${auditMethod}/${reason}] ${sanitized}` : null,
        error_code: raw.ok ? null : (reason ?? null),
      });
    } catch (auditErr) {
      console.error("[audit] failed to record router test", auditErr);
    }

    const result: import("./router-test.server").TransportTestResult = {
      ok: raw.ok,
      method: auditMethod,
      endpoint: raw.endpoint,
      steps: raw.steps,
      remediation: raw.remediation,
      ...(raw.reason ? { reason: raw.reason } : {}),
      ...(raw.error ? { error: raw.error } : {}),
      ...(raw.info ? { info: raw.info as import("./router-test.server").RouterInfo } : {}),
    };
    return result;
  });

export const routerTelemetry = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({ id: z.string().uuid(), includeServices: z.boolean().optional().default(true) })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const conn = await loadRouterConn(context.supabase, data.id);

    const [resR, ifsR, actR] = await Promise.allSettled([
      routerAPI.ping(conn),
      routerAPI.raw<Array<Record<string, string>>>(conn, "/interface"),
      routerAPI.activeUsers(conn),
    ]);
    const interfaces = ifsR.status === "fulfilled" ? ifsR.value : [];
    const resource = resR.status === "fulfilled" ? (resR.value as Record<string, string>) : null;
    const activeCount = actR.status === "fulfilled" ? actR.value.length : null;
    const readError = (result: PromiseSettledResult<unknown>) =>
      result.status === "rejected"
        ? result.reason instanceof Error
          ? result.reason.message
          : String(result.reason)
        : null;
    const errors = {
      resource: readError(resR),
      interfaces: readError(ifsR),
      activeUsers: readError(actR),
    };

    let healthR: PromiseSettledResult<Array<Record<string, string>>> | null = null;
    let fwR: PromiseSettledResult<Array<Record<string, string>>> | null = null;
    let wgR: PromiseSettledResult<Array<Record<string, string>>> | null = null;
    let qR: PromiseSettledResult<Array<Record<string, string>>> | null = null;
    if (data.includeServices) {
      [healthR, fwR, wgR, qR] = await Promise.allSettled([
        routerAPI.raw<Array<Record<string, string>>>(conn, "/system/health"),
        routerAPI.raw<Array<Record<string, string>>>(conn, "/ip/firewall/filter"),
        routerAPI.raw<Array<Record<string, string>>>(conn, "/interface/wireguard/peers"),
        routerAPI.raw<Array<Record<string, string>>>(conn, "/queue/tree"),
      ]);
    }

    let temperature_c: number | null = null;
    if (healthR?.status === "fulfilled" && Array.isArray(healthR.value)) {
      for (const row of healthR.value) {
        const name = String(row.name ?? "").toLowerCase();
        if (!name.includes("temp")) continue;
        const n = Number(String(row.value ?? "").replace(/[^\d.-]/g, ""));
        if (Number.isFinite(n)) {
          temperature_c = n;
          break;
        }
      }
    }

    const nowSec = Math.floor(Date.now() / 1000);
    let wireguard: { total: number; handshake_ok: number } | null = null;
    if (wgR?.status === "fulfilled") {
      const peers = wgR.value;
      wireguard = {
        total: peers.length,
        handshake_ok: peers.filter((p) => {
          const raw = p["last-handshake"] ?? "";
          const asNum = Number(raw);
          if (Number.isFinite(asNum) && asNum > 1_000_000_000) return nowSec - asNum < 180;
          if (Number.isFinite(asNum) && asNum >= 0 && asNum < 1_000_000) return asNum < 180;
          return Boolean(p["current-endpoint"]);
        }).length,
      };
    }

    // Syslog volume today from ingested events (real DB), not a fake card.
    let syslog_today: number | null = null;
    if (data.includeServices) {
      try {
        const start = new Date();
        start.setHours(0, 0, 0, 0);
        const { count, error } = await context.supabase
          .from("syslog_events")
          .select("id", { count: "exact", head: true })
          .eq("router_id", data.id)
          .gte("received_at", start.toISOString());
        if (!error) syslog_today = count ?? 0;
      } catch {
        syslog_today = null;
      }
    }

    return {
      ts: Date.now(),
      online: resR.status === "fulfilled",
      resource,
      board_name: resource?.["board-name"] ?? null,
      temperature_c,
      activeCount,
      services: {
        firewall_rules: fwR?.status === "fulfilled" ? fwR.value.length : null,
        wireguard: wgR ? wireguard : null,
        queue_trees: qR?.status === "fulfilled" ? qR.value.length : null,
        syslog_today: data.includeServices ? syslog_today : null,
      },
      interfaces: interfaces.map((i) => ({
        name: i.name,
        type: i.type,
        running: i.running === "true",
        disabled: i.disabled === "true",
        rxBytes: Number(i["rx-byte"] ?? 0),
        txBytes: Number(i["tx-byte"] ?? 0),
      })),
      errors,
    };
  });

/** Read the slower RouterOS service counters without repeating the fast snapshot. */
export const routerServiceCounts = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const conn = await loadRouterConn(context.supabase, data.id);

    const start = new Date();
    start.setHours(0, 0, 0, 0);
    const [healthR, fwR, wgR, qR, syslogR] = await Promise.allSettled([
      routerAPI.raw<Array<Record<string, string>>>(conn, "/system/health"),
      routerAPI.raw<Array<Record<string, string>>>(conn, "/ip/firewall/filter"),
      routerAPI.raw<Array<Record<string, string>>>(conn, "/interface/wireguard/peers"),
      routerAPI.raw<Array<Record<string, string>>>(conn, "/queue/tree"),
      context.supabase
        .from("syslog_events")
        .select("id", { count: "exact", head: true })
        .eq("router_id", data.id)
        .gte("received_at", start.toISOString()),
    ]);
    const readError = (result: PromiseSettledResult<unknown>) =>
      result.status === "rejected"
        ? result.reason instanceof Error
          ? result.reason.message
          : String(result.reason)
        : null;

    let temperature_c: number | null = null;
    if (healthR.status === "fulfilled" && Array.isArray(healthR.value)) {
      for (const row of healthR.value) {
        const name = String(row.name ?? "").toLowerCase();
        if (!name.includes("temp")) continue;
        const n = Number(String(row.value ?? "").replace(/[^\d.-]/g, ""));
        if (Number.isFinite(n)) {
          temperature_c = n;
          break;
        }
      }
    }

    const nowSec = Math.floor(Date.now() / 1000);
    let wireguard: { total: number; handshake_ok: number } | null = null;
    if (wgR.status === "fulfilled") {
      const peers = wgR.value;
      wireguard = {
        total: peers.length,
        handshake_ok: peers.filter((p) => {
          const raw = p["last-handshake"] ?? "";
          const asNum = Number(raw);
          if (Number.isFinite(asNum) && asNum > 1_000_000_000) return nowSec - asNum < 180;
          if (Number.isFinite(asNum) && asNum >= 0 && asNum < 1_000_000) return asNum < 180;
          return Boolean(p["current-endpoint"]);
        }).length,
      };
    }

    const syslogError =
      syslogR.status === "fulfilled" && syslogR.value.error
        ? syslogR.value.error.message
        : readError(syslogR);

    return {
      checkedAt: new Date().toISOString(),
      temperature_c,
      services: {
        firewall_rules: fwR.status === "fulfilled" ? fwR.value.length : null,
        wireguard,
        queue_trees: qR.status === "fulfilled" ? qR.value.length : null,
        syslog_today:
          syslogR.status === "fulfilled" && !syslogR.value.error
            ? (syslogR.value.count ?? 0)
            : null,
      },
      errors: {
        health: readError(healthR),
        firewall: readError(fwR),
        wireguard: readError(wgR),
        queues: readError(qR),
        syslog: syslogError,
      },
    };
  });

/**
 * Sum live RouterOS interface byte counters across reachable routers.
 * Rates are derived client-side from two samples.
 */
export const getFleetTrafficSnapshot = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const { data: rows, error } = await context.supabase
      .from("router_connections")
      .select("id, is_virtual, connection_mode")
      .limit(12);
    if (error) throw new Error(error.message);
    const { filterPhysicalRouters } = await import("./test-router");
    const physical = filterPhysicalRouters(rows ?? []);
    if (!physical.length) {
      return {
        observedAt: new Date().toISOString(),
        onlineRouters: 0,
        totalRxBytes: 0,
        totalTxBytes: 0,
      };
    }

    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");

    let totalRxBytes = 0;
    let totalTxBytes = 0;
    let onlineRouters = 0;

    await Promise.all(
      physical.map(async (row) => {
        try {
          const conn = await loadRouterConn(context.supabase, row.id);
          await routerAPI.ping(conn);
          const interfaces = await routerAPI.raw<Array<Record<string, string>>>(conn, "/interface");
          onlineRouters += 1;
          for (const iface of interfaces) {
            if (iface.disabled === "true") continue;
            totalRxBytes += Number(iface["rx-byte"] ?? 0);
            totalTxBytes += Number(iface["tx-byte"] ?? 0);
          }
        } catch {
          /* offline or unreachable — skip */
        }
      }),
    );

    return {
      observedAt: new Date().toISOString(),
      onlineRouters,
      totalRxBytes,
      totalTxBytes,
    };
  });

export const routersStatus = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    await (await import("./guards.server")).requireNotExpired(context.supabase, context.userId);
    const observedAt = new Date().toISOString();
    const { data: rows, error } = await context.supabase
      .from("router_connections")
      .select("id, name, connection_mode, is_virtual");
    if (error) throw new Error(error.message);
    const { filterPhysicalRouters } = await import("./test-router");
    const physical = filterPhysicalRouters(rows ?? []);
    if (!physical.length) {
      return {
        observedAt,
        count: 0,
        online: 0,
        offline: 0,
        routers: [] as Array<{
          id: string;
          name: string;
          online: boolean;
          mode: "direct" | "hub" | "sandbox";
          error?: string;
          checkedAt: string;
          latencyMs: number | null;
        }>,
      };
    }
    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const results = await Promise.all(
      physical.map(async (r) => {
        const mode =
          r.connection_mode === "hub" || r.connection_mode === "cloud"
            ? ("hub" as const)
            : r.connection_mode === "sandbox"
              ? ("sandbox" as const)
              : ("direct" as const);
        try {
          const started = Date.now();
          const conn = await loadRouterConn(context.supabase, r.id);
          await routerAPI.ping(conn);
          return {
            id: r.id,
            name: r.name,
            online: true as const,
            mode,
            checkedAt: observedAt,
            latencyMs: Date.now() - started,
          };
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          return {
            id: r.id,
            name: r.name,
            online: false as const,
            mode,
            error: msg,
            checkedAt: observedAt,
            latencyMs: null,
          };
        }
      }),
    );
    const online = results.filter((r) => r.online).length;
    return {
      observedAt,
      count: physical.length,
      online,
      offline: physical.length - online,
      routers: results,
    };
  });

/**
 * Verify identity — the owner-confirmed half of the router identity guard.
 *
 * Records the board's live identity as this router's known-good fingerprint. Until a
 * router has a row here, a config push to it is refused (TARGET_UNVERIFIED): silently
 * adopting whatever board answers is how a push once landed on the wrong device.
 * Owner clicks this once per router; after that, any push that resolves elsewhere throws
 * TARGET_MISMATCH inside the guard.
 */
export const verifyRouterIdentity = createServerFn({ method: "POST" })
  .inputValidator((input: { routerId: string }) => {
    const parsed = z.object({ routerId: z.string().min(1) }).safeParse(input);
    if (!parsed.success) throw new Error("TARGET_UNRESOLVED: routerId is required");
    return parsed.data;
  })
  .middleware([requireSupabaseAuth])
  .handler(async ({ data, context }) => {
    const { requireNotExpired, isPlatformAdminUser, getRoles, isPrivilegedAccount } = await import(
      "./guards.server"
    );
    await requireNotExpired(context.supabase, context.userId);
    const [roles, isPlatformAdmin] = await Promise.all([
      getRoles(context.supabase, context.userId),
      isPlatformAdminUser(context.supabase, context.userId),
    ]);
    // Allow platform admin OR the router's effective owner
    const { data: ownerRow, error: ownerErr } = await context.supabase.rpc("effective_owner", {
      _user_id: context.userId,
    });
    if (ownerErr) throw new Error(ownerErr.message);
    const ownerId =
      typeof ownerRow === "string" ? ownerRow : (ownerRow as { id?: string } | null)?.id ?? null;
    if (!ownerId) throw new Error("TARGET_UNRESOLVED: could not resolve owner for identity check");
    if (!isPlatformAdmin && !isPrivilegedAccount(roles, isPlatformAdmin)) {
      // Not a platform admin — verify this user owns the router
      const { data: routerRow, error: routerErr } = await context.supabase
        .from("router_connections")
        .select("owner_id")
        .eq("id", data.routerId)
        .maybeSingle();
      if (routerErr) throw new Error(routerErr.message);
      if (!routerRow) throw new Error("Router not found");
      if (routerRow.owner_id !== ownerId) {
        throw new Error("Only the router owner can verify its identity.");
      }
    }

    const { loadRouterConn } = await import("./router-conn.server");
    const conn = await loadRouterConn(context.supabase, data.routerId);

    const { captureFingerprint } = await import("./router/target-guard.server");
    const captured = await captureFingerprint(context.supabase, ownerId, data.routerId, "router", conn);

    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { error: auditErr } = await supabaseAdmin.from("router_save_audit").insert({
      user_id: context.userId,
      owner_id: ownerId,
      action: "verify_identity",
      router_id: data.routerId,
      resolved_serial: captured.fingerprint.serial,
      resolved_board_name: captured.fingerprint.boardName,
      fingerprint_match: true,
    });
    if (auditErr) console.error("[audit] verify_identity", auditErr.message);

    return {
      ok: true as const,
      fingerprintId: captured.id,
      serial: captured.fingerprint.serial,
      boardName: captured.fingerprint.boardName,
      model: captured.fingerprint.model,
      routerosVersion: captured.fingerprint.routerosVersion,
    };
  });
