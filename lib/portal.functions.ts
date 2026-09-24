import { createServerFn } from "@tanstack/react-start";
import type { DatabaseClient } from "./database.types";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { z } from "zod";
import { missingDefaultDataQuotaPlans } from "./portal/default-plans";
import {
  hotspotUserCreateBody,
  PLAN_TAG,
  planProfileBody,
  planProfileName,
} from "./portal/plan-profile";
import {
  DEFAULT_PAYMENT_METHODS,
  PORTAL_GUEST_MODES,
  PAYMENT_METHOD_ACTIONS,
  parsePaymentMethods,
  payInfoQrAssetName,
  normalizePortalGuestMode,
  type PortalGuestMode,
} from "./portal/modes";
import {
  classifyVoucherPlan,
  isArchivedVoucherPlan,
  operatorVoucherPlans,
  type PlanGroupId,
} from "./portal/plan-groups";
import type { PortalTheme } from "./portal-template.server";
import { safeCssHex } from "./html-escape";
import {
  hasHistoricRouterUsage,
  isLegacyVoucherImportExcluded,
  normalizeVoucherCode,
} from "./legacy-voucher-import";

const DEFAULT_SETTINGS = {
  business_name: "Neon Cafe Wi-Fi",
  welcome_text: "Enter the voucher code from the front desk to get online.",
  terms: "By connecting you agree to our fair-use policy.",
  primary_hex: "#ffb547",
  glass_tint_hex: "#7ad0ff",
  logo_path: null as string | null,
  hero_path: null as string | null,
  guest_mode: "voucher_only" as PortalGuestMode,
  payment_methods: DEFAULT_PAYMENT_METHODS,
  seller_phone: null as string | null,
  seller_label: "Talk to our seller",
  trial_minutes: 10,
  trial_cooldown_hours: 12,
  need_code_label: "I don't have a code",
  ask_desk_hint: "Ask the front desk for a Wi-Fi voucher code.",
  notify_ticket_activation: false,
};

const paymentMethodSchema = z.object({
  id: z.string().min(1).max(40),
  enabled: z.boolean(),
  label: z.string().min(1).max(80),
  description: z.string().max(240),
  accentHex: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  action: z.enum(PAYMENT_METHOD_ACTIONS),
  infoTitle: z.string().max(80),
  infoBody: z.string().max(2000),
  copyValue: z.string().max(120),
  qrImagePath: z.string().max(200).nullable().optional(),
  sort: z.number().int().min(0).max(100),
});

const portalSettingsPatchSchema = z.object({
  business_name: z.string().min(1).max(80),
  welcome_text: z.string().min(1).max(400),
  terms: z.string().max(600).default(""),
  primary_hex: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  glass_tint_hex: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  logo_path: z.string().nullable().optional(),
  hero_path: z.string().nullable().optional(),
  guest_mode: z.enum(PORTAL_GUEST_MODES).default("voucher_only"),
  payment_methods: z.array(paymentMethodSchema).max(12).optional(),
  seller_phone: z.string().max(40).nullable().optional(),
  seller_label: z.string().min(1).max(80).optional(),
  trial_minutes: z.number().int().min(1).max(120).optional(),
  trial_cooldown_hours: z.number().int().min(0).max(168).optional(),
  need_code_label: z.string().min(1).max(80).optional(),
  ask_desk_hint: z.string().min(1).max(200).optional(),
});

type PortalSettingsRow = Record<string, unknown>;

async function themeFromSettings(
  supabase: DatabaseClient,
  ownerId: string,
  s: PortalSettingsRow,
  _opts?: { siteIds?: string[] | null },
): Promise<PortalTheme> {
  const mode = normalizePortalGuestMode(s.guest_mode);
  const { data: plans } = await supabase
    .from("portal_plans")
    .select("plan_key, label, duration_label, price_label, status, sort")
    .eq("owner_id", ownerId)
    .order("sort", { ascending: true })
    .limit(40);

  const packages = (plans ?? [])
    .filter((p) => p.status !== "inactive" && !isArchivedVoucherPlan(p))
    .map((p) => ({
      label: p.label,
      durationLabel: p.duration_label ?? "",
      priceLabel: p.price_label ?? "",
    }));

  return {
    businessName: String(s.business_name ?? "Wi-Fi"),
    welcomeText: String(s.welcome_text ?? ""),
    terms: String(s.terms ?? ""),
    primaryHex: safeCssHex(String(s.primary_hex ?? "#ffb547"), "#ffb547"),
    glassTintHex: safeCssHex(String(s.glass_tint_hex ?? "#7ad0ff"), "#7ad0ff"),
    logoUrl: s.logo_path ? "img/logo.png" : undefined,
    heroUrl: s.hero_path ? "img/hero.jpg" : undefined,
    guestMode: mode,
    paymentMethods: parsePaymentMethods(s.payment_methods ?? s.commerce_channels),
    sellerPhone: (s.seller_phone as string | null) ?? "",
    sellerLabel: (s.seller_label as string | null) ?? "Talk to our seller",
    trialMinutes: Number(s.trial_minutes ?? 10),
    trialCooldownHours: Number(s.trial_cooldown_hours ?? 12),
    needCodeLabel: (s.need_code_label as string | null) ?? "I don't have a code",
    askDeskHint:
      (s.ask_desk_hint as string | null) ?? "Ask the front desk for a Wi-Fi voucher code.",
    packages,
  };
}

async function resolveOwner(supabase: DatabaseClient, userId: string): Promise<string> {
  const { data, error } = await supabase.rpc("effective_owner", {
    _user_id: userId,
  });
  if (error) throw new Error(error.message);
  return (data as string | null) ?? userId;
}

/**
 * Portal mode access must be checked wherever saved settings can be rendered or
 * sent to a router. The database trigger is authoritative for direct writes;
 * this keeps server actions safe as well and returns a clear app-level error.
 */
async function assertStoredPortalModeAccess(
  supabase: DatabaseClient,
  userId: string,
  ownerId: string,
): Promise<PortalGuestMode> {
  const { data, error } = await supabase
    .from("portal_settings")
    .select("guest_mode")
    .eq("owner_id", ownerId)
    .maybeSingle();
  if (error) throw new Error(error.message);

  const mode = normalizePortalGuestMode(data?.guest_mode);
  const { assertCanOperateGuestMode } = await import("./portal-grants.functions");
  await assertCanOperateGuestMode(supabase, userId, mode);
  return mode;
}

export const getPortalSettings = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data, error } = await context.supabase
      .from("portal_settings")
      .select("*")
      .eq("owner_id", ownerId)
      .maybeSingle();
    if (error) throw new Error(error.message);
    if (!data) {
      // Auto-create default row for this owner.
      const { data: inserted, error: insErr } = await context.supabase
        .from("portal_settings")
        .insert({ owner_id: ownerId, ...DEFAULT_SETTINGS })
        .select("*")
        .single();
      if (insErr) throw new Error(insErr.message);
      return inserted;
    }
    return data;
  });

export const savePortalSettings = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => portalSettingsPatchSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { requireNotExpired } = await import("./guards.server");
    await requireNotExpired(context.supabase, context.userId);
    const { assertCanOperateGuestMode } = await import("./portal-grants.functions");
    await assertCanOperateGuestMode(context.supabase, context.userId, data.guest_mode);

    const ownerId = await resolveOwner(context.supabase, context.userId);
    const patch = {
      business_name: data.business_name,
      welcome_text: data.welcome_text,
      terms: data.terms,
      primary_hex: data.primary_hex,
      glass_tint_hex: data.glass_tint_hex,
      logo_path: data.logo_path,
      hero_path: data.hero_path,
      guest_mode: data.guest_mode,
      payment_methods: data.payment_methods ?? DEFAULT_PAYMENT_METHODS,
      seller_phone: data.seller_phone ?? null,
      seller_label: data.seller_label ?? "Talk to our seller",
      trial_minutes: data.trial_minutes ?? 10,
      trial_cooldown_hours: data.trial_cooldown_hours ?? 12,
      need_code_label: data.need_code_label ?? "I don't have a code",
      ask_desk_hint: data.ask_desk_hint ?? "Ask the front desk for a Wi-Fi voucher code.",
    };
    const { data: existing, error: existingErr } = await context.supabase
      .from("portal_settings")
      .select("*")
      .eq("owner_id", ownerId)
      .maybeSingle();
    if (existingErr) throw new Error(existingErr.message);
    const { error } = await context.supabase
      .from("portal_settings")
      .upsert(
        { ...DEFAULT_SETTINGS, ...(existing ?? {}), owner_id: ownerId, ...patch },
        { onConflict: "owner_id" },
      );
    if (error) throw new Error(error.message);
    return { ok: true };
  });

export const uploadPortalAsset = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        kind: z.enum(["logo", "hero", "qr"]),
        dataUrl: z.string().min(20).max(6_000_000),
        /** Required when kind is qr — sanitized into the storage object name. */
        methodId: z.string().min(1).max(40).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const { requireNotExpired } = await import("./guards.server");
    await requireNotExpired(context.supabase, context.userId);
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const m = data.dataUrl.match(/^data:(image\/(png|jpeg|jpg|webp|gif));base64,(.+)$/);
    if (!m) throw new Error("Invalid image data");
    const mime = m[1];
    const bytes = Buffer.from(m[3], "base64");
    const ext = m[2] === "jpeg" ? "jpg" : m[2];
    if (data.kind === "qr") {
      const methodId = (data.methodId ?? "mmqr").replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 40);
      if (!methodId) throw new Error("MMQR method id is required");
      const path = `${ownerId}/qr-${methodId}.${ext}`;
      const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
      const { error } = await supabaseAdmin.storage
        .from("portal-assets")
        .upload(path, bytes, { contentType: mime, upsert: true });
      if (error) throw new Error(error.message);
      // Path is stored on the payment method JSON by the client — do not patch logo/hero columns.
      return { path };
    }
    const path = `${ownerId}/${data.kind}.${ext}`;

    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { error } = await supabaseAdmin.storage
      .from("portal-assets")
      .upload(path, bytes, { contentType: mime, upsert: true });
    if (error) throw new Error(error.message);
    const patch = data.kind === "logo" ? { logo_path: path } : { hero_path: path };
    const { data: existing, error: existingErr } = await context.supabase
      .from("portal_settings")
      .select("*")
      .eq("owner_id", ownerId)
      .maybeSingle();
    if (existingErr) throw new Error(existingErr.message);
    const { error: upErr } = await context.supabase
      .from("portal_settings")
      .upsert(
        { ...DEFAULT_SETTINGS, ...(existing ?? {}), owner_id: ownerId, ...patch },
        { onConflict: "owner_id" },
      );
    if (upErr) throw new Error(upErr.message);
    return { path };
  });

/**
 * Portal assets are always written as `<ownerUuid>/<logo|hero>.<ext>` or
 * `<ownerUuid>/qr-<methodId>.<ext>` by uploadPortalAsset. Anything else is
 * rejected so a crafted nested path can never be signed.
 */
const PORTAL_ASSET_PATH = /^[0-9a-f-]{36}\/(logo|hero|qr-[a-zA-Z0-9_-]+)\.[a-z0-9]{2,5}$/;

export const getSignedAssetUrl = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z.object({ path: z.string().min(1).max(200).regex(PORTAL_ASSET_PATH) }).parse(raw),
  )
  .handler(async ({ data, context }) => {
    // Authorize: path must live under the caller's effective owner folder.
    const { data: ownerId, error: ownerErr } = await context.supabase.rpc("effective_owner", {
      _user_id: context.userId,
    });
    if (ownerErr || !ownerId) throw new Error("Forbidden");
    const segments = data.path.split("/");
    if (segments.length !== 2 || segments[0] !== ownerId) throw new Error("Forbidden");

    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { data: signed, error } = await supabaseAdmin.storage
      .from("portal-assets")
      .createSignedUrl(data.path, 60 * 60);
    if (error) throw new Error(error.message);
    return { url: signed.signedUrl };
  });

export const buildPortalZip = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    await assertStoredPortalModeAccess(context.supabase, context.userId, ownerId);
    const { renderPortalFiles } = await import("./portal-template.server");
    const JSZip = (await import("jszip")).default;

    const { data: settings } = await context.supabase
      .from("portal_settings")
      .select("*")
      .eq("owner_id", ownerId)
      .maybeSingle();
    const s = (settings ?? { ...DEFAULT_SETTINGS, owner_id: ownerId }) as PortalSettingsRow;
    const theme = await themeFromSettings(context.supabase, ownerId, s);
    const files = renderPortalFiles(theme);
    const zip = new JSZip();
    for (const f of files) zip.file(f.name, f.content);

    // Attach uploaded logo/hero as img/*.
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    async function attach(path: string | null, target: string) {
      if (!path) return;
      const { data: blob, error } = await supabaseAdmin.storage
        .from("portal-assets")
        .download(path);
      if (error || !blob) return;
      const ab = await blob.arrayBuffer();
      zip.file(target, ab);
    }
    await attach((s.logo_path as string | null) ?? null, "img/logo.png");
    await attach((s.hero_path as string | null) ?? null, "img/hero.jpg");
    const methods = parsePaymentMethods(s.payment_methods ?? s.commerce_channels);
    for (const method of methods) {
      if (method.action !== "pay_info" || !method.qrImagePath) continue;
      await attach(method.qrImagePath, payInfoQrAssetName(method.id));
    }

    const buf = await zip.generateAsync({ type: "base64" });
    return { base64: buf, filename: "hotspot-portal.zip" };
  });

export const listPlans = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data, error } = await context.supabase
      .from("portal_plans")
      .select("*")
      .eq("owner_id", ownerId)
      .order("sort", { ascending: true })
      .order("created_at", { ascending: true });
    if (error) throw new Error(error.message);
    const rows = data ?? [];
    const missing = missingDefaultDataQuotaPlans(
      rows.filter((r) => !isArchivedVoucherPlan(r)).map((r) => r.plan_key),
    );
    if (!missing.length) return operatorVoucherPlans(rows);
    const { error: insErr } = await context.supabase.from("portal_plans").insert(
      missing.map((p) => ({
        owner_id: ownerId,
        plan_key: p.plan_key,
        label: p.label,
        duration_label: p.duration_label,
        duration_minutes: p.duration_minutes,
        device_limit: p.device_limit,
        rate_limit: p.rate_limit,
        price_mmk: p.price_mmk,
        price_label: p.price_label,
        is_vip: p.is_vip,
        sort: p.sort,
        data_quota_mb: p.data_quota_mb,
        validity_days: p.validity_days,
        status: p.status,
      })),
    );
    if (insErr) return operatorVoucherPlans(rows);
    const { data: again, error: againErr } = await context.supabase
      .from("portal_plans")
      .select("*")
      .eq("owner_id", ownerId)
      .order("sort", { ascending: true });
    if (againErr) return operatorVoucherPlans(rows);
    return operatorVoucherPlans(again ?? rows);
  });

const MISSING_TICKET_ALERT_COL =
  "Paste the ticket-activation SQL in Lovable Cloud SQL Editor, then republish.";

export const getTicketActivationPref = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data, error } = await context.supabase
      .from("portal_settings")
      .select("notify_ticket_activation")
      .eq("owner_id", ownerId)
      .maybeSingle();
    if (error) {
      if (/notify_ticket_activation/i.test(error.message)) {
        return { enabled: false as const };
      }
      throw new Error(error.message);
    }
    return {
      enabled: Boolean(
        (data as { notify_ticket_activation?: boolean | null } | null)?.notify_ticket_activation,
      ),
    };
  });

export const saveTicketActivationPref = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ enabled: z.boolean() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { requireVoucherOperator } = await import("./guards.server");
    await requireVoucherOperator(context.supabase, context.userId);
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data: existing } = await context.supabase
      .from("portal_settings")
      .select("owner_id")
      .eq("owner_id", ownerId)
      .maybeSingle();
    const patch = { notify_ticket_activation: data.enabled };
    const result = existing
      ? await context.supabase.from("portal_settings").update(patch).eq("owner_id", ownerId)
      : await context.supabase
          .from("portal_settings")
          .insert({ owner_id: ownerId, ...DEFAULT_SETTINGS, ...patch });
    if (result.error) {
      if (/notify_ticket_activation/i.test(result.error.message)) {
        throw new Error(MISSING_TICKET_ALERT_COL);
      }
      throw new Error(result.error.message);
    }
    return { ok: true as const, enabled: data.enabled };
  });

const planSchema = z.object({
  id: z.string().uuid().optional(),
  plan_key: z.string().min(1).max(30),
  label: z.string().min(1).max(60),
  duration_label: z.string().max(30),
  duration_minutes: z.number().int().min(0).max(525_600).nullable(),
  device_limit: z.number().int().min(0).max(50).default(1),
  rate_limit: z.string().max(40).nullable().optional(),
  price_mmk: z.number().int().min(0).max(100_000_000),
  is_vip: z.boolean().default(false),
  manual_code: z.string().max(60).nullable().optional(),
  sort: z.number().int().min(0).default(0),
  /** Optional data allowance in megabytes; null means no data cap. */
  data_quota_mb: z.number().int().min(0).max(1_000_000).nullable().optional(),
  /** How many days a code stays redeemable after it is issued. */
  validity_days: z.number().int().min(0).max(3650).nullable().optional(),
  /** Inactive plans stay in the reports but are hidden from guests. */
  status: z.enum(["active", "inactive"]).default("active"),
});

export const savePlan = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => planSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { requireVoucherOperator } = await import("./guards.server");
    await requireVoucherOperator(context.supabase, context.userId);
    const ownerId = await resolveOwner(context.supabase, context.userId);
    if (data.is_vip && !data.manual_code?.trim()) {
      throw new Error("VIP plans need a voucher code that you set yourself.");
    }
    const planKey = data.plan_key.trim().toLowerCase();
    if (planKey.startsWith("archived-") || planKey === "archived") {
      throw new Error("Archived plan keys are reserved cleanup leftovers and cannot be saved.");
    }
    const fields = {
      plan_key: planKey,
      label: data.label,
      duration_label: data.duration_label,
      duration_minutes: data.is_vip ? null : data.duration_minutes,
      device_limit: data.device_limit,
      rate_limit: data.is_vip ? null : data.rate_limit || null,
      price_mmk: data.price_mmk,
      price_label: `${new Intl.NumberFormat("en-US").format(data.price_mmk)} MMK`,
      is_vip: data.is_vip,
      manual_code: data.manual_code?.trim() || null,
      sort: data.sort,
      data_quota_mb: data.data_quota_mb ?? null,
      validity_days: data.validity_days ?? null,
      status: data.status,
    };
    if (data.id) {
      const { error } = await context.supabase
        .from("portal_plans")
        .update(fields)
        .eq("id", data.id);
      if (error) throw new Error(error.message);
    } else {
      const { data: existing, error: existingError } = await context.supabase
        .from("portal_plans")
        .select("id")
        .eq("owner_id", ownerId)
        .eq("plan_key", planKey)
        .order("created_at", { ascending: true })
        .limit(1)
        .maybeSingle();
      if (existingError) throw new Error(existingError.message);
      if (existing?.id) {
        const { error } = await context.supabase
          .from("portal_plans")
          .update(fields)
          .eq("id", existing.id);
        if (error) throw new Error(error.message);
        return { ok: true, id: existing.id, updated: true };
      }
      const { error } = await context.supabase
        .from("portal_plans")
        .insert({ owner_id: ownerId, ...fields });
      if (error) throw new Error(error.message);
    }
    return { ok: true };
  });

export const deletePlan = createServerFn({ method: "POST" })
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
    const { requireVoucherOperator } = await import("./guards.server");
    await requireVoucherOperator(context.supabase, context.userId);
    const { assertTypedConfirmation, REMOVE_PLANS_PHRASE } = await import("./device-removal");
    assertTypedConfirmation(data.confirmation, REMOVE_PLANS_PHRASE);
    const { error } = await context.supabase.from("portal_plans").delete().eq("id", data.id);
    if (error) throw new Error(error.message);
    return { ok: true };
  });

// ---- Portal auto-deploy ----

const TEXT_FILE_NAMES = [
  "login.html",
  "status.html",
  "logout.html",
  "error.html",
  "alogin.html",
  "radvert.html",
  "style.css",
];

export const listDeployableRouters = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data, error } = await context.supabase
      .from("router_connections")
      .select("id, name, host, is_virtual, environment, connection_mode, site_id")
      .eq("owner_id", ownerId)
      .order("name", { ascending: true });
    if (error) throw new Error(error.message);
    const { isVirtualRouter } = await import("./test-router");
    return (data ?? [])
      .filter((r) => !isVirtualRouter(r))
      .map((r) => ({
        id: r.id as string,
        name: r.name as string,
        host: r.host as string,
        environment: ((r.environment as string | null) ?? "production") as "test" | "production",
        connection_mode: (r.connection_mode as string | null) ?? null,
        site_id: (r.site_id as string | null) ?? null,
      }));
  });

export const listPortalDeploys = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data, error } = await context.supabase
      .from("portal_deploy_audit")
      .select("id, router_id, router_name, ok, error, duration_ms, created_at")
      .eq("owner_id", ownerId)
      .order("created_at", { ascending: false })
      .limit(20);
    if (error) throw new Error(error.message);
    return data ?? [];
  });

export const getPortalDeployProbe = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        routerId: z.string().uuid(),
        profileNames: z.array(z.string().min(1).max(80)).max(20).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const { data: ownerId, error: oErr } = await context.supabase.rpc("effective_owner", {
      _user_id: context.userId,
    });
    if (oErr || !ownerId) throw new Error("Cannot resolve owner");
    await assertStoredPortalModeAccess(context.supabase, context.userId, ownerId as string);

    const { data: row } = await context.supabase
      .from("router_connections")
      .select("id, name, site_id, is_virtual")
      .eq("id", data.routerId)
      .maybeSingle();
    if (!row) throw new Error("Router not found");
    const { isVirtualRouter } = await import("./test-router");
    if (isVirtualRouter(row)) throw new Error("Sandbox routers are not available.");

    const siteIds = row.site_id ? [row.site_id as string] : null;
    const { files, logoPath, heroPath } = await renderBundleForOwner(
      context.supabase,
      ownerId as string,
      { siteIds },
    );
    const { loadRouterConn } = await import("./router-conn.server");
    const { probePortalOnRouter } = await import("./portal/portal-probe.server");
    const conn = await loadRouterConn(context.supabase, data.routerId);
    const probe = await probePortalOnRouter(conn, {
      expectedFiles: files,
      logoPath,
      heroPath,
      profileNames: data.profileNames,
    });
    return {
      routerId: data.routerId,
      routerName: (row.name as string) ?? "router",
      ...probe,
    };
  });

/** Read-only active-portal inventory. Device HTML/JS is never exposed to the browser. */
export const scanPortalOnDevice = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        routerId: z.string().uuid(),
        profileNames: z.array(z.string().min(1).max(80)).max(20).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    await assertStoredPortalModeAccess(context.supabase, context.userId, ownerId);
    const { data: row, error } = await context.supabase
      .from("router_connections")
      .select("id, name, owner_id, is_virtual")
      .eq("id", data.routerId)
      .eq("owner_id", ownerId)
      .maybeSingle();
    if (error) throw new Error(error.message);
    if (!row) throw new Error("Router not found.");
    const { isVirtualRouter } = await import("./test-router");
    if (isVirtualRouter(row)) throw new Error("Sandbox routers are not available.");
    const { loadRouterConn } = await import("./router-conn.server");
    const { scanPortalOnRouter } = await import("./portal/portal-probe.server");
    const scan = await scanPortalOnRouter(await loadRouterConn(context.supabase, data.routerId), {
      profileNames: data.profileNames,
    });
    return { routerId: data.routerId, routerName: (row.name as string) ?? "router", ...scan };
  });

export async function renderBundleForOwner(
  supabase: DatabaseClient,
  ownerId: string,
  opts?: { siteIds?: string[] | null },
) {
  const { renderPortalTextBundle } = await import("./portal-template.server");
  const { data: settings } = await supabase
    .from("portal_settings")
    .select("*")
    .eq("owner_id", ownerId)
    .maybeSingle();

  const s = (settings ?? {
    ...DEFAULT_SETTINGS,
    owner_id: ownerId,
  }) as PortalSettingsRow;
  const theme = await themeFromSettings(supabase, ownerId, s, opts);
  const files = renderPortalTextBundle(theme);
  const methods = parsePaymentMethods(s.payment_methods ?? s.commerce_channels);
  const qrAssets = methods
    .filter((m) => m.action === "pay_info" && m.qrImagePath)
    .map((m) => ({
      storagePath: m.qrImagePath as string,
      bundlePath: payInfoQrAssetName(m.id),
    }));
  return {
    files,
    logoPath: (s.logo_path as string | null) ?? null,
    heroPath: (s.hero_path as string | null) ?? null,
    qrAssets,
  };
}

export const publishPortalToRouter = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        routerIds: z.array(z.string().uuid()).min(1).max(20),
        /** Hotspot server profiles to switch. Empty = every profile on the router. */
        profileNames: z.array(z.string().min(1).max(80)).max(20).optional(),
        confirmation: z.string().max(200).default(""),
        /** When true, deploy even if the live portal already matches saved settings. */
        forceRepublish: z.boolean().optional().default(false),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const { requireFeature } = await import("./guards.server");
    await requireFeature(context.supabase, context.userId, "portal_deploy");
    const { data: ownerId, error: oErr } = await context.supabase.rpc("effective_owner", {
      _user_id: context.userId,
    });
    if (oErr || !ownerId) throw new Error("Cannot resolve owner");
    await assertStoredPortalModeAccess(context.supabase, context.userId, ownerId as string);

    // Gate the batch on the strictest environment in the selection.
    const { data: rows } = await context.supabase
      .from("router_connections")
      .select("id, name, environment, is_virtual, connection_mode, site_id")
      .in("id", data.routerIds);
    const targets = (rows ?? []) as Array<{
      id: string;
      name: string;
      environment: string | null;
      is_virtual?: boolean | null;
      connection_mode?: string | null;
      site_id?: string | null;
    }>;
    const gates = await import("./test-router");
    if (targets.some((t) => gates.isVirtualRouter(t)))
      throw new Error(gates.SANDBOX_REMOVED_MESSAGE);
    if (targets.length !== data.routerIds.length)
      throw new Error("One or more selected routers are not available to this account.");
    const ordered = data.routerIds.map((id) => targets.find((t) => t.id === id)!);
    const siteIds = [...new Set(ordered.map((t) => t.site_id).filter((id): id is string => !!id))];

    const { files, logoPath, heroPath, qrAssets } = await renderBundleForOwner(
      context.supabase,
      ownerId,
      {
        siteIds: siteIds.length ? siteIds : null,
      },
    );
    const { portalBundleFingerprint } = await import("./portal/bundle-fingerprint");
    const bundleFingerprint = portalBundleFingerprint(files, {
      logo: Boolean(logoPath),
      hero: Boolean(heroPath),
      qr: qrAssets.map((a) => a.bundlePath).sort().join(","),
    });

    const { loadRouterConn } = await import("./router-conn.server");
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { executeDeploy, readProfileTargets } = await import("./portal/deploy.server");
    const { recordRouterOp } = await import("./audit.server");

    async function signedAsset(path: string | null) {
      if (!path) return null;
      const { data: s } = await supabaseAdmin.storage
        .from("portal-assets")
        .createSignedUrl(path, 60 * 30);
      return s?.signedUrl ?? null;
    }
    const logoUrl = await signedAsset(logoPath);
    const heroUrl = await signedAsset(heroPath);
    const extraBinaries: Array<{ path: string; url: string }> = [];
    for (const asset of qrAssets) {
      const url = await signedAsset(asset.storagePath);
      if (url) extraBinaries.push({ path: asset.bundlePath, url });
    }
    const version = new Date()
      .toISOString()
      .replace(/[^0-9]/g, "")
      .slice(0, 14);
    const { portalManifestText } = await import("./portal/deploy-plan");
    const manifestContent = portalManifestText({
      version,
      fingerprint: bundleFingerprint,
      deployedAt: new Date().toISOString(),
    });

    const textFetchUrls: Record<string, string> = {};
    const stagingDir = `hotspot-mm-${version}`;
    try {
      for (const f of [...files, { name: "mm-manifest.json", content: manifestContent }]) {
        const storagePath = `${ownerId}/deploys/${version}/${f.name}`;
        const { error: upErr } = await supabaseAdmin.storage
          .from("portal-assets")
          .upload(storagePath, Buffer.from(f.content, "utf8"), {
            contentType: f.name.endsWith(".css")
              ? "text/css"
              : f.name.endsWith(".json")
                ? "application/json"
                : "text/html; charset=utf-8",
            upsert: true,
          });
        if (upErr) continue;
        const { data: signed } = await supabaseAdmin.storage
          .from("portal-assets")
          .createSignedUrl(storagePath, 60 * 30);
        if (signed?.signedUrl) textFetchUrls[`${stagingDir}/${f.name}`] = signed.signedUrl;
      }
    } catch {
      // Storage missing — REST /file/add is still tried first.
    }

    const { environment: batchEnv, expected } = gates.portalDeployGate(ordered);
    try {
      gates.assertConfirmation(data.confirmation, expected);
    } catch (e) {
      await recordRouterOp({
        userId: context.userId,
        ownerId: ownerId as string,
        action: "confirmation_failed",
        environment: batchEnv,
        outcome: "blocked",
        detail: `portal deploy — expected "${expected}"`,
        error: e,
      });
      throw e instanceof Error ? e : new Error(String(e));
    }

    const results = [];
    for (const target of ordered) {
      const start = performance.now();
      const routerName = target.name ?? "router";
      const environment = (target.environment ?? "production") as "test" | "production";
      await recordRouterOp({
        userId: context.userId,
        ownerId: ownerId as string,
        routerId: target.id,
        routerName,
        action: "portal_deploy_started",
        environment,
        outcome: "ok",
        detail: `version ${version}`,
      });
      try {
        const conn = await loadRouterConn(context.supabase, target.id);
        const profiles = await readProfileTargets(conn, data.profileNames);
        if (!profiles.length) throw new Error("No matching hotspot profile found on this router.");

        if (!data.forceRepublish) {
          const { probePortalOnRouter } = await import("./portal/portal-probe.server");
          const probe = await probePortalOnRouter(conn, {
            expectedFiles: files,
            logoPath,
            heroPath,
            profileNames: data.profileNames,
          });
          if (probe.status === "matches") {
            const duration_ms = Math.round(performance.now() - start);
            await recordRouterOp({
              userId: context.userId,
              ownerId: ownerId as string,
              routerId: target.id,
              routerName,
              action: "portal_deploy_result",
              environment,
              outcome: "ok",
              detail: `skipped — portal already matches (${probe.liveDirectory ?? "live"})`,
              durationMs: duration_ms,
            });
            results.push({
              routerId: target.id,
              routerName,
              ok: true,
              skipped: true,
              files: [] as string[],
              rolledBack: false,
              error: "",
              probeStatus: probe.status,
              liveDirectory: probe.liveDirectory,
            });
            continue;
          }
        }

        const { result } = await executeDeploy(conn, {
          version,
          files,
          logoUrl,
          heroUrl,
          extraBinaries,
          profiles,
          fingerprint: bundleFingerprint,
          manifestContent,
          textFetchUrls,
        });
        if (result.ok) {
          const { pruneOrphanMagicPortalDirs } = await import("./portal/prune-portal-dirs.server");
          await pruneOrphanMagicPortalDirs(conn).catch(() => ({
            removedDirs: [] as string[],
            removedFiles: 0,
            errors: [] as string[],
          }));
          const settingsRow = await context.supabase
            .from("portal_settings")
            .select("guest_mode, trial_minutes")
            .eq("owner_id", ownerId)
            .maybeSingle();
          const guestMode = (settingsRow.data as { guest_mode?: string } | null)?.guest_mode;
          if (normalizePortalGuestMode(guestMode) === "commerce") {
            const { ensureHotspotTrialAccess } = await import("./portal/trial.server");
            const trialMinutes = Number(
              (settingsRow.data as { trial_minutes?: number } | null)?.trial_minutes ?? 10,
            );
            await ensureHotspotTrialAccess(conn, {
              hotspotProfileIds: profiles.map((p) => p.id),
              trialMinutes,
            });
          }
        }
        const duration_ms = Math.round(performance.now() - start);
        await supabaseAdmin
          .from("portal_deploy_audit")
          .insert({
            owner_id: ownerId,
            user_id: context.userId,
            router_id: target.id,
            router_name: routerName,
            ok: result.ok,
            files: result.written,
            snapshot: JSON.parse(
              JSON.stringify({
                ...result.prior,
                version,
              }),
            ),
            error: result.ok
              ? null
              : [result.error, result.rollbackError && `rollback: ${result.rollbackError}`]
                  .filter(Boolean)
                  .join(" | ")
                  .slice(0, 500),
            duration_ms,
          })
          .then(({ error }) => {
            if (error) console.error("[portal-deploy] audit insert failed", error.message);
          });
        await recordRouterOp({
          userId: context.userId,
          ownerId: ownerId as string,
          routerId: target.id,
          routerName,
          action: "portal_deploy_result",
          environment,
          outcome: result.ok ? "ok" : result.partial ? "partial" : "failed",
          detail: `version ${version}, ${result.completed}/${result.total} steps${
            result.rolledBack ? ", rolled back" : ""
          }`,
          error: result.error,
          durationMs: duration_ms,
        });
        results.push({
          routerId: target.id,
          routerName,
          ok: result.ok,
          skipped: false,
          files: result.written,
          rolledBack: result.rolledBack,
          error: result.error ?? "",
        });
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        const duration_ms = Math.round(performance.now() - start);
        await supabaseAdmin
          .from("portal_deploy_audit")
          .insert({
            owner_id: ownerId,
            user_id: context.userId,
            router_id: target.id,
            router_name: routerName,
            ok: false,
            files: [],
            snapshot: {},
            error: msg.slice(0, 500),
            duration_ms,
          })
          .then(({ error }) => {
            if (error) console.error("[portal-deploy] audit insert failed", error.message);
          });
        await recordRouterOp({
          userId: context.userId,
          ownerId: ownerId as string,
          routerId: target.id,
          routerName,
          action: "portal_deploy_result",
          environment,
          outcome: "failed",
          error: e,
          durationMs: duration_ms,
        });
        results.push({
          routerId: target.id,
          routerName,
          ok: false,
          files: [] as string[],
          rolledBack: false,
          error: msg,
        });
      }
    }
    return { results, version };
  });

export const rollbackPortalDeploy = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ deployId: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { requireFeature } = await import("./guards.server");
    await requireFeature(context.supabase, context.userId, "portal_deploy");
    const { data: row, error } = await context.supabase
      .from("portal_deploy_audit")
      .select("router_id, router_name, owner_id, snapshot")
      .eq("id", data.deployId)
      .single();
    if (error || !row) throw new Error("Deploy not found");

    const prior = row.snapshot as unknown as import("./portal/deploy-plan").PriorState | null;
    if (!prior || !Array.isArray(prior.profiles))
      throw new Error(
        "This deployment predates staged rollback and has no restorable state recorded.",
      );

    const { loadRouterConn } = await import("./router-conn.server");
    const { executeRestore } = await import("./portal/deploy.server");
    const { unrestorableAssets } = await import("./portal/deploy-plan");
    const { recordRouterOp } = await import("./audit.server");
    const conn = await loadRouterConn(context.supabase, row.router_id);
    const start = performance.now();
    const res = await executeRestore(conn, prior);

    await recordRouterOp({
      userId: context.userId,
      ownerId: row.owner_id as string,
      routerId: row.router_id as string,
      routerName: row.router_name as string,
      action: "portal_rollback_result",
      outcome: res.ok ? "ok" : "partial",
      detail: `${res.steps} restore steps`,
      error: res.error,
      durationMs: Math.round(performance.now() - start),
    });

    return {
      ok: res.ok,
      restored: res.steps,
      ...(res.error ? { error: res.error } : {}),
      // Binary assets that existed before cannot be recovered byte-for-byte.
      manualCheck: unrestorableAssets(prior),
    };
  });

// ---- Voucher plan templates -> router ----

type PlanRow = {
  id: string;
  plan_key: string;
  label: string;
  duration_minutes: number | null;
  device_limit: number;
  rate_limit: string | null;
  price_mmk: number;
  is_vip: boolean;
  manual_code: string | null;
  data_quota_mb?: number | null;
  validity_days?: number | null;
  status?: string | null;
};

export type PushPlansScope = "default" | "all" | "time" | "data" | "custom";

export type PushedVoucherProfile = {
  planKey: string;
  planLabel: string;
  hotspotProfile: string;
  action: "created" | "updated";
};

export type PlanPushPlanResult = {
  planKey: string;
  planLabel: string;
  hotspotProfile: string;
  ok: boolean;
  action?: "created" | "updated";
  error?: string | null;
};

export function plansForScope<
  T extends {
    plan_key: string;
    is_vip?: boolean;
    data_quota_mb?: number | null;
    duration_minutes?: number | null;
  },
>(plans: readonly T[], scope: PushPlansScope): T[] {
  if (scope === "all") return [...plans];

  const desired: PlanGroupId[] =
    scope === "default" ? ["time", "data"] : ([scope] as unknown as PlanGroupId[]);

  return plans.filter((plan) => desired.includes(classifyVoucherPlan(plan)));
}

function profileName(p: PlanRow) {
  return planProfileName(p.plan_key);
}

function profileBody(p: PlanRow): Record<string, string> {
  return planProfileBody(p);
}

export async function ensurePlanProfileOnRouter(
  conn: import("./mikrotik.server").RouterConn,
  plan: PlanRow,
): Promise<void> {
  const { ensurePlanProfileOnRouter: ensure } = await import("./portal/ensure-plan-profile.server");
  await ensure(conn, plan);
}

export const pushPlansToRouter = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        routerIds: z.array(z.string().uuid()).min(1).max(20),
        mode: z.enum(["add", "replace"]).default("add"),
        scope: z.enum(["default", "all", "time", "data", "custom"]).default("all"),
        confirmation: z.string().max(80).optional(),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) => {
    const { requireVoucherOperator } = await import("./guards.server");
    await requireVoucherOperator(context.supabase, context.userId);
    if (data.mode === "replace") {
      const { assertTypedConfirmation, REMOVE_PLANS_PHRASE } = await import("./device-removal");
      assertTypedConfirmation(data.confirmation, REMOVE_PLANS_PHRASE);
    }
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const { data: plans, error } = await context.supabase
      .from("portal_plans")
      .select("*")
      .eq("owner_id", ownerId)
      .order("sort", { ascending: true });
    if (error) throw new Error(error.message);
    if (!plans?.length) throw new Error("No plans to push yet.");

    const livePlans = operatorVoucherPlans(plans as unknown as PlanRow[]);
    const plansToPush = plansForScope(livePlans, data.scope);
    if (!plansToPush.length) {
      throw new Error(`No voucher profiles match scope "${data.scope}".`);
    }

    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");

    const results = await Promise.all(
      (data.routerIds as string[]).map(async (routerId) => {
        const { data: row } = await context.supabase
          .from("router_connections")
          .select("name, is_virtual, connection_mode, site_id")
          .eq("id", routerId)
          .single();
        const routerName = row?.name ?? "router";
        const { formatVoucherPlanPushError } = await import("./portal/plan-push-error");
        try {
          const { assertNotVirtualRouter } = await import("./test-router");
          assertNotVirtualRouter(row, "receive voucher plans");
          const conn = await loadRouterConn(context.supabase, routerId);

          const { assertHotspotReadyForPlanPush } =
            await import("./portal/push-plans-preflight.server");
          await assertHotspotReadyForPlanPush(conn);

          const { ensureRouterClockAligned } = await import("./router-clock");
          // NTP + Asia/Yangon together so a 1970 clock cannot survive plan push alone.
          const timezoneWarning = await ensureRouterClockAligned(conn);

          const existing = (await routerAPI.profiles(conn)) ?? [];
          const { isManagedVoucherProfile } = await import("./portal/plan-profile");
          const managed = existing.filter((e) => isManagedVoucherProfile(e));

          if (data.mode === "replace") {
            // Only clear profiles inside the selected scope; plans outside it
            // (e.g. custom-priced offers when pushing defaults) must survive.
            const keep = new Set(
              livePlans
                .filter((p) => !plansToPush.some((s) => s.plan_key === p.plan_key))
                .map((p) => profileName(p)),
            );
            for (const m of managed) {
              if (m[".id"] && !keep.has(m["name"] ?? "")) {
                await routerAPI.removeUserProfile(conn, m[".id"]!).catch(() => null);
              }
            }
          }

          let written = 0;
          const errors: string[] = [];
          const profilesWritten: PushedVoucherProfile[] = [];
          const planResults: PlanPushPlanResult[] = [];
          for (const p of plansToPush as PlanRow[]) {
            const body = profileBody(p);
            try {
              const { ensurePlanProfileOnRouter } =
                await import("./portal/ensure-plan-profile.server");
              const action = await ensurePlanProfileOnRouter(conn, p);
              profilesWritten.push({
                planKey: p.plan_key,
                planLabel: p.label,
                hotspotProfile: body["name"],
                action,
              });
              planResults.push({
                planKey: p.plan_key,
                planLabel: p.label,
                hotspotProfile: body["name"],
                ok: true,
                action,
              });
              written++;
            } catch (e) {
              const message = formatVoucherPlanPushError(e);
              errors.push(`${body.name}: ${message}`);
              planResults.push({
                planKey: p.plan_key,
                planLabel: p.label,
                hotspotProfile: body["name"],
                ok: false,
                error: message,
              });
            }
          }
          if (!written) {
            throw new Error(errors.join(" | ") || "No hotspot user profiles could be saved.");
          }
          const { APP_TIMEZONE } = await import("./time");
          return {
            routerId,
            routerName,
            ok: errors.length === 0,
            written,
            error: errors.length ? errors.join(" | ") : (null as string | null),
            timezone: APP_TIMEZONE,
            timezoneWarning,
            profilesWritten,
            planResults,
          };
        } catch (e) {
          return {
            routerId,
            routerName,
            ok: false,
            written: 0,
            error: formatVoucherPlanPushError(e),
            timezone: null as string | null,
            timezoneWarning: null as string | null,
            profilesWritten: [],
            planResults: [] as PlanPushPlanResult[],
          };
        }
      }),
    );
    return { results, mode: data.mode };
  });

export type IssuePlanVouchersInput = {
  planId: string;
  routerId: string;
  count: number;
  /** When set (reseller Issue stock), every code starts with this shop mark. */
  codePrefix?: string;
};

export async function issuePlanVouchersForUser(
  supabase: DatabaseClient,
  userId: string,
  data: IssuePlanVouchersInput,
) {
    const { requireVoucherOperator } = await import("./guards.server");
    await requireVoucherOperator(supabase, userId);
    const ownerId = await resolveOwner(supabase, userId);
    const { data: plan, error } = await supabase
      .from("portal_plans")
      .select("*")
      .eq("id", data.planId)
      .eq("owner_id", ownerId)
      .single();
    if (error || !plan) throw new Error("Plan not found");
    const p = plan as PlanRow;

    if (p.status === "inactive") {
      throw new Error("This plan is inactive — set it to active before generating codes.");
    }
    // Free-tier allowance: plan issuance is the third way vouchers are created,
    // so it enforces the same monthly rule. A VIP plan yields exactly one code.
    const { assertVoucherQuota } = await import("./vouchers/quota.server");
    await assertVoucherQuota(supabase, userId, p.is_vip ? 1 : data.count);

    const {
      allocateUniqueCodes,
      expiresAtFromValidityDays,
      isValidShopCodePrefix,
      normalizeShopCodePrefix,
      randomCode: genCode,
      randomPrefixedShopCode,
    } = await import("./portal/voucher-codes");

    let codes: string[];
    if (p.is_vip) {
      if (data.codePrefix) {
        throw new Error("VIP plans use a single manual code — pick a normal plan for shop-prefixed stock.");
      }
      const vip = p.manual_code?.trim().toUpperCase();
      if (!vip) throw new Error("Set a VIP code on the plan first.");
      codes = [vip];
    } else {
      const { data: existingRows } = await supabase
        .from("voucher_codes")
        .select("code")
        .eq("owner_id", ownerId)
        .neq("status", "cancelled")
        .limit(5000);
      const existing = new Set((existingRows ?? []).map((r) => String(r.code).toUpperCase()));
      const prefix = data.codePrefix ? normalizeShopCodePrefix(data.codePrefix) : "";
      if (data.codePrefix && !isValidShopCodePrefix(prefix)) {
        throw new Error("Shop code prefix must be 2–4 characters using A–Z / 2–9 (no I/O/0/1).");
      }
      codes = allocateUniqueCodes({
        count: data.count,
        existing,
        generate: () => (prefix ? randomPrefixedShopCode(prefix) : genCode()),
      });
    }

    const { data: routerRow } = await supabase
      .from("router_connections")
      .select("site_id")
      .eq("id", data.routerId)
      .eq("owner_id", ownerId)
      .maybeSingle();
    if (!routerRow) throw new Error("Router not found");
    const siteId = (routerRow?.site_id as string | null | undefined) ?? null;

    const { loadRouterConn } = await import("./router-conn.server");
    const { routerAPI } = await import("./mikrotik.server");
    const conn = await loadRouterConn(supabase, data.routerId);

    // Resolve the binding ONCE and persist it on every voucher.
    const boundProfile = profileName(p);
    await ensurePlanProfileOnRouter(conn, p);
    const { data: lastDeploy } = await supabase
      .from("portal_deploy_audit")
      .select("snapshot, created_at")
      .eq("router_id", data.routerId)
      .eq("ok", true)
      .order("created_at", { ascending: false })
      .limit(1)
      .maybeSingle();
    const deployVersion =
      ((lastDeploy?.snapshot as { version?: string } | null)?.version ?? null) || null;

    const stockExpiresAt = p.is_vip ? null : expiresAtFromValidityDays(p.validity_days);
    const issued: string[] = [];
    const failed: Array<{ code: string; error: string }> = [];

    for (const code of codes) {
      const { data: row, error: insErr } = await supabase
        .from("voucher_codes")
        .insert({
          owner_id: ownerId,
          router_id: data.routerId,
          site_id: siteId,
          plan_id: p.id,
          plan_key: p.plan_key,
          plan_label: p.label,
          hotspot_profile: boundProfile,
          profile_bound_at: new Date().toISOString(),
          deploy_version: deployVersion,
          code,
          price_mmk: p.price_mmk,
          duration_minutes: p.is_vip ? null : p.duration_minutes,
          expires_at: stockExpiresAt,
          status: "unused",
        })
        .select("id")
        .single();

      if (insErr || !row) {
        const msg = insErr?.message ?? "Ledger insert failed";
        if (/duplicate|unique/i.test(msg)) {
          failed.push({
            code,
            error: p.is_vip
              ? "This VIP code is already in the ledger (another router or soft-deleted). Change the VIP code or delete the old row."
              : "Code already exists in the ledger — skipped.",
          });
        } else {
          failed.push({ code, error: msg });
        }
        continue;
      }

      try {
        await routerAPI.addUser(
          conn,
          hotspotUserCreateBody({
            name: code,
            password: code,
            profile: boundProfile,
            comment: `${PLAN_TAG}:${p.plan_key}`,
            dataQuotaMb: p.is_vip ? null : p.data_quota_mb,
          }),
        );
        issued.push(code);
      } catch (routerErr) {
        await supabase.from("voucher_codes").delete().eq("id", row.id);
        failed.push({
          code,
          error: routerErr instanceof Error ? routerErr.message : String(routerErr),
        });
      }
    }

    if (!issued.length) {
      throw new Error(failed[0]?.error ?? "Could not create vouchers on the router.");
    }

    return {
      issued,
      failed,
      planLabel: p.label,
      priceMmk: p.price_mmk,
      expiresAt: stockExpiresAt,
      routerId: data.routerId,
    };
}

export const issuePlanVouchers = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        planId: z.string().uuid(),
        routerId: z.string().uuid(),
        count: z.number().int().min(1).max(100).default(1),
      })
      .parse(raw),
  )
  .handler(async ({ data, context }) =>
    issuePlanVouchersForUser(context.supabase, context.userId, data),
  );

export const listVoucherCodes = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z.object({ routerId: z.string().uuid().optional() }).parse(raw ?? {}),
  )
  .handler(async ({ data, context }) => {
    const ownerId = await resolveOwner(context.supabase, context.userId);
    let query = context.supabase
      .from("voucher_codes")
      .select("*")
      .eq("owner_id", ownerId)
      .order("created_at", { ascending: false })
      .limit(5000);
    if (data.routerId) query = query.eq("router_id", data.routerId);
    const { data: rows, error } = await query;
    if (error) throw new Error(error.message);
    return rows ?? [];
  });

const voucherReconciliationSchema = z.object({
  voucherIds: z.array(z.string().uuid()).min(1).max(500),
  action: z.enum(["keep", "obsolete", "investigate"]),
  reason: z.string().trim().min(3).max(500),
});

const legacyVoucherScanSchema = z.object({ routerId: z.string().uuid() });
const legacyVoucherImportSchema = z.object({
  routerId: z.string().uuid(),
  imports: z
    .array(
      z.object({
        code: z.string().trim().min(1).max(120),
        routerProfile: z.string().trim().min(1).max(120),
        planId: z.string().uuid(),
      }),
    )
    .min(1)
    .max(500),
  reason: z.string().trim().min(3).max(500),
});

type LegacyRouterUser = Record<string, string>;

async function readLegacyVoucherCandidates(
  supabase: DatabaseClient,
  ownerId: string,
  routerId: string,
): Promise<{
  importable: Array<{ code: string; routerProfile: string; comment: string | null }>;
  alreadySynced: number;
  needsReview: Array<{ code: string; routerProfile: string; reason: string }>;
  excluded: number;
}> {
  const { loadRouterConn } = await import("./router-conn.server");
  const { routerAPI } = await import("./mikrotik.server");
  const [conn, ledgerResult] = await Promise.all([
    loadRouterConn(supabase, routerId),
    supabase
      .from("voucher_codes")
      .select("code")
      .eq("owner_id", ownerId)
      .eq("router_id", routerId)
      .limit(5000),
  ]);
  if (ledgerResult.error) throw new Error(ledgerResult.error.message);
  const users = (await routerAPI.users(conn)) as LegacyRouterUser[];
  const ledgerCodes = new Set(
    (ledgerResult.data ?? []).map((row) => normalizeVoucherCode(row.code)),
  );
  const seen = new Set<string>();
  const importable: Array<{ code: string; routerProfile: string; comment: string | null }> = [];
  const needsReview: Array<{ code: string; routerProfile: string; reason: string }> = [];
  let alreadySynced = 0;
  let excluded = 0;

  for (const user of users) {
    if (isLegacyVoucherImportExcluded(user)) {
      excluded++;
      continue;
    }
    const code = normalizeVoucherCode(user.name ?? "");
    const routerProfile = (user.profile ?? "").trim();
    if (!code || !routerProfile || seen.has(code)) {
      needsReview.push({
        code: code || "(unnamed)",
        routerProfile: routerProfile || "—",
        reason: "Missing or duplicate router account data",
      });
      continue;
    }
    seen.add(code);
    if (ledgerCodes.has(code)) {
      alreadySynced++;
      continue;
    }
    if (hasHistoricRouterUsage(user)) {
      needsReview.push({
        code,
        routerProfile,
        reason: "RouterOS shows historic usage; import would not recreate revenue",
      });
      continue;
    }
    importable.push({ code, routerProfile, comment: user.comment?.trim() || null });
  }
  return { importable, alreadySynced, needsReview, excluded };
}

/** Read-only device scan. It never writes RouterOS or the voucher ledger. */
export const scanLegacyRouterVouchers = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => legacyVoucherScanSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { requirePrivileged } = await import("./guards.server");
    await requirePrivileged(context.supabase, context.userId);
    const ownerId = await resolveOwner(context.supabase, context.userId);
    return readLegacyVoucherCandidates(context.supabase, ownerId, data.routerId);
  });

/**
 * Imports only accounts that are still router-only and unused on a fresh scan.
 * The database function is service-role-only, records an immutable audit event,
 * and does not expose a direct authenticated-client import path.
 */
export const importLegacyRouterVouchers = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => legacyVoucherImportSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { requirePrivileged } = await import("./guards.server");
    await requirePrivileged(context.supabase, context.userId);
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const fresh = await readLegacyVoucherCandidates(context.supabase, ownerId, data.routerId);
    const allowed = new Map(fresh.importable.map((row) => [normalizeVoucherCode(row.code), row]));
    const seen = new Set<string>();
    const imports = data.imports.map((item) => {
      const code = normalizeVoucherCode(item.code);
      const routerRow = allowed.get(code);
      if (
        !routerRow ||
        normalizeVoucherCode(routerRow.routerProfile) !==
          normalizeVoucherCode(item.routerProfile) ||
        seen.has(code)
      ) {
        throw new Error(
          `Voucher ${code} is no longer an unused router-only account. Scan again before importing.`,
        );
      }
      seen.add(code);
      return { code, router_profile: routerRow.routerProfile, plan_id: item.planId };
    });
    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { data: result, error } = await supabaseAdmin.rpc("import_legacy_router_vouchers", {
      _owner_id: ownerId,
      _actor_user_id: context.userId,
      _router_id: data.routerId,
      _imports: imports,
      _reason: data.reason,
    });
    if (error) throw new Error(error.message);
    return result;
  });

/**
 * Reconciles app-ledger-only voucher rows without touching RouterOS. The SQL
 * function repeats authorization and tenant checks so the RPC cannot be used
 * as a client-side authorization bypass.
 */
export const reconcileVoucherLedger = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => voucherReconciliationSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { requirePrivileged } = await import("./guards.server");
    await requirePrivileged(context.supabase, context.userId);
    const { data: result, error } = await context.supabase.rpc("reconcile_voucher_ledger", {
      _voucher_ids: data.voucherIds,
      _action: data.action,
      _reason: data.reason,
    });
    if (error) throw new Error(error.message);
    return result;
  });
