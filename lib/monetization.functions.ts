import { createServerFn } from "@tanstack/react-start";
import type { DatabaseClient } from "./database.types";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { z } from "zod";
import { buildRevenueEntries, totalsFor, type RevenueSource } from "./payments/accounting";

const priceSchema = z.object({
  id: z.string().uuid().optional(),
  profile: z.string().min(1).max(80),
  price_cents: z.number().int().min(0).max(10_000_00),
  currency: z.string().min(1).max(6).default("USD"),
  site_id: z.string().uuid().nullable().optional(),
  router_id: z.string().uuid().nullable().optional(),
});

const saleSchema = z.object({
  profile: z.string().min(1).max(80),
  code: z.string().min(1).max(80),
  price_cents: z.number().int().min(0).max(10_000_00),
  currency: z.string().min(1).max(6).default("USD"),
  note: z.string().max(500).optional(),
  site_id: z.string().uuid().nullable().optional(),
  router_id: z.string().uuid().nullable().optional(),
  sold_at: z.string().datetime().optional(),
});

async function resolveOwner(supabase: DatabaseClient, userId: string): Promise<string> {
  const { data, error } = await supabase.rpc("effective_owner", { _user_id: userId });
  if (error) throw new Error(error.message);
  return (data as string | null) ?? userId;
}

export const listVoucherPrices = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const { data, error } = await context.supabase
      .from("voucher_prices")
      .select("id, profile, price_cents, currency, site_id, router_id, created_at")
      .order("profile", { ascending: true });
    if (error) throw new Error(error.message);
    return data ?? [];
  });

export const saveVoucherPrice = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => priceSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const owner_id = await resolveOwner(context.supabase, context.userId);
    if (data.id) {
      const { error } = await context.supabase
        .from("voucher_prices")
        .update({
          profile: data.profile,
          price_cents: data.price_cents,
          currency: data.currency,
          site_id: data.site_id ?? null,
          router_id: data.router_id ?? null,
        })
        .eq("id", data.id);
      if (error) throw new Error(error.message);
      return { id: data.id };
    }
    const { data: row, error } = await context.supabase
      .from("voucher_prices")
      .insert({
        owner_id,
        profile: data.profile,
        price_cents: data.price_cents,
        currency: data.currency,
        site_id: data.site_id ?? null,
        router_id: data.router_id ?? null,
      })
      .select("id")
      .single();
    if (error) throw new Error(error.message);
    return { id: row.id };
  });

export const deleteVoucherPrice = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { error } = await context.supabase.from("voucher_prices").delete().eq("id", data.id);
    if (error) throw new Error(error.message);
    return { ok: true };
  });

export const recordVoucherSale = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => saleSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const owner_id = await resolveOwner(context.supabase, context.userId);
    const { data: row, error } = await context.supabase
      .from("voucher_sales")
      .insert({
        owner_id,
        profile: data.profile,
        code: data.code,
        price_cents: data.price_cents,
        currency: data.currency,
        note: data.note ?? null,
        site_id: data.site_id ?? null,
        router_id: data.router_id ?? null,
        sold_by: context.userId,
        ...(data.sold_at ? { sold_at: data.sold_at } : {}),
      })
      .select("id")
      .single();
    if (error) throw new Error(error.message);
    return { id: row.id };
  });

export const deleteVoucherSale = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => z.object({ id: z.string().uuid() }).parse(raw))
  .handler(async ({ data, context }) => {
    const { error } = await context.supabase.from("voucher_sales").delete().eq("id", data.id);
    if (error) throw new Error(error.message);
    return { ok: true };
  });

export const listVoucherSales = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z
      .object({
        site_id: z.string().uuid().nullable().optional(),
        limit: z.number().int().min(1).max(1000).default(500),
      })
      .parse(raw ?? {}),
  )
  .handler(async ({ data, context }) => {
    let q = context.supabase
      .from("voucher_sales")
      .select("id, profile, code, price_cents, currency, note, site_id, router_id, sold_at")
      .order("sold_at", { ascending: false })
      .limit(data.limit);
    if (data.site_id) q = q.eq("site_id", data.site_id);
    const { data: rows, error } = await q;
    if (error) throw new Error(error.message);
    return rows ?? [];
  });

export const revenueRollup = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z.object({ site_id: z.string().uuid().nullable().optional() }).parse(raw ?? {}),
  )
  .handler(async ({ data, context }) => {
    let q = context.supabase
      .from("voucher_sales")
      .select("price_cents, currency, sold_at, profile")
      .gte("sold_at", new Date(Date.now() - 90 * 24 * 3600 * 1000).toISOString());
    if (data.site_id) q = q.eq("site_id", data.site_id);
    const { data: rows, error } = await q;
    if (error) throw new Error(error.message);
    const now = Date.now();
    const buckets = {
      today: 0,
      week: 0,
      month: 0,
      all90: 0,
      count_today: 0,
      count_week: 0,
      count_month: 0,
      count_all: 0,
    };
    const currency = rows?.[0]?.currency ?? "USD";
    const byProfile: Record<string, { count: number; cents: number }> = {};
    const byDay: Record<string, number> = {};
    for (const r of rows ?? []) {
      const t = new Date(r.sold_at).getTime();
      const age = now - t;
      buckets.all90 += r.price_cents;
      buckets.count_all++;
      if (age <= 24 * 3600 * 1000) {
        buckets.today += r.price_cents;
        buckets.count_today++;
      }
      if (age <= 7 * 24 * 3600 * 1000) {
        buckets.week += r.price_cents;
        buckets.count_week++;
      }
      if (age <= 30 * 24 * 3600 * 1000) {
        buckets.month += r.price_cents;
        buckets.count_month++;
      }
      const p = byProfile[r.profile] ?? { count: 0, cents: 0 };
      p.count++;
      p.cents += r.price_cents;
      byProfile[r.profile] = p;
      const day = new Date(r.sold_at).toISOString().slice(0, 10);
      byDay[day] = (byDay[day] ?? 0) + r.price_cents;
    }
    return { currency, buckets, byProfile, byDay };
  });

// ---------------------------------------------------------------------------
// Automatic revenue dashboard. Settled payment orders are the revenue source;
// voucher first use is kept as an operational redemption signal. Voucher and
// manual rows remain only for pre-order historical data.
// ---------------------------------------------------------------------------
export const revenueDashboard = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) =>
    z.object({ site_id: z.string().uuid().nullable().optional() }).parse(raw ?? {}),
  )
  .handler(async ({ data, context }) => {
    const { appStartOfDay, appDayKey } = await import("./time");
    const { effectiveOwner } = await import("./guards.server");
    const { planPerformance, visiblePlanPerformance } = await import("./payments/plan-revenue");
    const ownerId = await effectiveOwner(context.supabase, context.userId);

    let vq = context.supabase
      .from("voucher_codes")
      .select(
        "id, code, plan_key, plan_label, price_mmk, status, first_seen_at, expires_at, created_at, site_id, router_id, device_mac, order_id",
      )
      .eq("owner_id", ownerId)
      .order("created_at", { ascending: false })
      .limit(5000);
    if (data.site_id) vq = vq.eq("site_id", data.site_id);

    let sq = context.supabase
      .from("voucher_sales")
      .select("id, code, profile, price_cents, currency, sold_at, site_id, router_id")
      .eq("owner_id", ownerId);
    if (data.site_id) sq = sq.eq("site_id", data.site_id);

    let oq = context.supabase
      .from("payment_orders")
      .select(
        "id, status, method, amount_minor, plan_label, issued_code, site_id, router_id, settled_at, refunded_at, created_at",
      )
      .eq("owner_id", ownerId)
      .limit(5000);
    if (data.site_id) oq = oq.eq("site_id", data.site_id);

    const [
      { data: vouchers, error: vErr },
      { data: legacy, error: sErr },
      { data: orders, error: oErr },
    ] = await Promise.all([vq, sq, oq]);
    if (vErr) throw new Error(vErr.message);
    if (sErr) throw new Error(sErr.message);
    if (oErr) throw new Error(oErr.message);

    const [{ data: sites }, { data: routers }, { data: plans, error: plansError }] =
      await Promise.all([
        context.supabase.from("sites").select("id, name").eq("owner_id", ownerId),
        context.supabase.from("router_connections").select("id, name").eq("owner_id", ownerId),
        context.supabase
          .from("portal_plans")
          .select("plan_key, label, status")
          .eq("owner_id", ownerId),
      ]);
    if (plansError) throw new Error(plansError.message);
    const siteName = new Map((sites ?? []).map((s) => [s.id, s.name]));
    const routerName = new Map((routers ?? []).map((r) => [r.id, r.name]));

    const now = Date.now();
    const startToday = appStartOfDay(now);
    const startYesterday = startToday - 86_400_000;
    const start7 = startToday - 6 * 86_400_000;
    const startPrev7 = startToday - 13 * 86_400_000;
    const start30 = startToday - 29 * 86_400_000;
    const startPrev30 = startToday - 59 * 86_400_000;

    type Earning = {
      at: number;
      amount: number;
      plan: string;
      site_id: string | null;
      router_id: string | null;
      code: string | null;
      device: string | null;
      source: RevenueSource;
    };

    const entries = buildRevenueEntries({
      orders: (orders ?? []) as never,
      vouchers: (vouchers ?? []) as never,
      legacySales: (legacy ?? []) as never,
    });
    const voucherByCode = new Map((vouchers ?? []).map((v) => [v.code, v]));
    const earnings: Earning[] = entries.map((entry) => ({
      ...entry,
      device: entry.code ? (voucherByCode.get(entry.code)?.device_mac ?? null) : null,
    }));

    const funnel = {
      issued: 0,
      active: 0,
      used: 0,
      expired: 0,
      unused: 0,
      cancelled: 0,
      lost_mmk: 0,
    };
    const planStats: Record<
      string,
      { label: string; issued: number; used: number; amount: number }
    > = {};
    const planKeyByLabel = new Map<string, string>();

    for (const v of vouchers ?? []) {
      const label = v.plan_label || v.plan_key || "Unknown";
      const p = (planStats[v.plan_key || label] ??= {
        label,
        issued: 0,
        used: 0,
        amount: 0,
      });
      planKeyByLabel.set(label, v.plan_key || label);
      p.issued += 1;
      funnel.issued += 1;

      // Cancelled voucher stock is retained for audit, but is never a sale —
      // even if the code was briefly seen by the router before cancellation.
      if (["cancelled", "deleted"].includes((v.status ?? "").toLowerCase())) {
        funnel.cancelled += 1;
        continue;
      }

      const firstSeen = v.first_seen_at ? new Date(v.first_seen_at).getTime() : null;
      const expiresAt = v.expires_at ? new Date(v.expires_at).getTime() : null;
      const isExpired = v.status === "expired" || (expiresAt != null && expiresAt <= now);

      if (firstSeen != null) {
        if (isExpired) {
          // Keep the funnel's Used count identical to the financial rule.
          p.used += 1;
          funnel.used += 1;
        } else {
          funnel.active += 1;
        }
      } else if (isExpired) {
        funnel.expired += 1;
        funnel.lost_mmk += v.price_mmk ?? 0;
      } else {
        funnel.unused += 1;
      }
    }

    for (const entry of entries) {
      const key = planKeyByLabel.get(entry.plan) ?? `sale:${entry.plan}`;
      const p = (planStats[key] ??= {
        label: entry.plan,
        issued: 0,
        used: 0,
        amount: 0,
      });
      p.amount += entry.amount;
    }

    const range = (from: number, to = Number.MAX_SAFE_INTEGER) => totalsFor(entries, from, to);
    const all = totalsFor(entries);

    const totals = {
      today: range(startToday).net,
      yesterday: range(startYesterday, startToday).net,
      week: range(start7).net,
      prev_week: range(startPrev7, start7).net,
      month: range(start30).net,
      prev_month: range(startPrev30, start30).net,
      all: all.net,
      count_today: range(startToday).count,
      count_week: range(start7).count,
      count_month: range(start30).count,
      count_all: all.count,
    };

    // 30-day daily series (app timezone days), oldest first.
    const dayTotals = new Map<string, number>();
    for (let i = 29; i >= 0; i--) dayTotals.set(appDayKey(startToday - i * 86_400_000), 0);
    for (const e of earnings) {
      if (e.at < start30) continue;
      const k = appDayKey(e.at);
      if (dayTotals.has(k)) dayTotals.set(k, (dayTotals.get(k) ?? 0) + e.amount);
    }
    const byDay = [...dayTotals.entries()].map(([day, amount]) => ({ day, amount }));

    const group = (key: "site_id" | "router_id", names: Map<string, string>) => {
      const m = new Map<string, { name: string; amount: number; count: number }>();
      for (const e of earnings) {
        const id = e[key] ?? "none";
        const entry = m.get(id) ?? {
          name: id === "none" ? "Unassigned" : (names.get(id) ?? "Unknown"),
          amount: 0,
          count: 0,
        };
        entry.amount += e.amount;
        entry.count += 1;
        m.set(id, entry);
      }
      return [...m.values()].sort((a, b) => b.amount - a.amount);
    };

    const byPlan = Object.values(planStats)
      .map((p) => ({
        ...p,
        used_rate: p.issued ? Math.round((p.used / p.issued) * 100) : 0,
      }))
      .sort((a, b) => b.amount - a.amount);
    const planPerformanceRows = visiblePlanPerformance(
      planPerformance(
        (vouchers ?? []) as never,
        (orders ?? []) as never,
        (plans ?? []).filter(
          (p) => !(p.plan_key ?? "").toLowerCase().startsWith("archived-") && p.status !== "archived",
        ) as never,
        now,
      ),
    );

    const recent = earnings
      .sort((a, b) => b.at - a.at)
      .slice(0, 20)
      .map((e) => ({
        code: e.code,
        plan: e.plan,
        amount: e.amount,
        at: new Date(e.at).toISOString(),
        device: e.device,
        site: e.site_id ? (siteName.get(e.site_id) ?? null) : null,
        source: e.source,
      }));

    return {
      currency: "MMK",
      totals,
      funnel,
      byDay,
      byPlan,
      planPerformance: planPerformanceRows,
      bySite: group("site_id", siteName),
      byRouter: group("router_id", routerName),
      recent,
    };
  });

// ---------------------------------------------------------------------------
// Plan-scoped voucher performance + the voucher inspector.
// Both read through the caller's RLS-scoped client, so an owner only ever sees
// their own tenant's vouchers, plans and orders.
// ---------------------------------------------------------------------------

const planReportSchema = z.object({
  site_id: z.string().uuid().nullable().optional(),
  limit: z.number().int().min(1).max(5000).default(5000),
});

export const planPerformanceReport = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => planReportSchema.parse(raw ?? {}))
  .handler(async ({ data, context }) => {
    const { planPerformance, visiblePlanPerformance } = await import("./payments/plan-revenue");
    const ownerId = await resolveOwner(context.supabase, context.userId);

    let vq = context.supabase
      .from("voucher_codes")
      .select(
        "id, code, plan_key, plan_label, price_mmk, status, created_at, first_seen_at, expires_at, device_mac, site_id, router_id, order_id",
      )
      .eq("owner_id", ownerId)
      .order("created_at", { ascending: false })
      .limit(data.limit);
    if (data.site_id) vq = vq.eq("site_id", data.site_id);

    let oq = context.supabase
      .from("payment_orders")
      .select(
        "id, plan_key, plan_label, status, amount_minor, issued_code, settled_at, refunded_at",
      )
      .eq("owner_id", ownerId);
    if (data.site_id) oq = oq.eq("site_id", data.site_id);

    const [{ data: vouchers, error: vErr }, { data: orders, error: oErr }, { data: plans }] =
      await Promise.all([
        vq,
        oq,
        context.supabase
          .from("portal_plans")
          .select("plan_key, label, status")
          .eq("owner_id", ownerId),
      ]);
    if (vErr) throw new Error(vErr.message);
    if (oErr) throw new Error(oErr.message);

    const rows = visiblePlanPerformance(
      planPerformance(
        (vouchers ?? []) as never,
        (orders ?? []) as never,
        (plans ?? []).filter(
          (p) => !(p.plan_key ?? "").toLowerCase().startsWith("archived-") && p.status !== "archived",
        ) as never,
      ),
    );
    return {
      currency: "MMK",
      plans: rows,
      totals: rows.reduce(
        (acc, r) => ({
          issued: acc.issued + r.issued,
          active: acc.active + r.active,
          used: acc.used + r.used,
          expired: acc.expired + r.expired,
          cancelled: acc.cancelled + r.cancelled,
          gross: acc.gross + r.gross,
          refunded: acc.refunded + r.refunded,
          net: acc.net + r.net,
        }),
        { issued: 0, active: 0, used: 0, expired: 0, cancelled: 0, gross: 0, refunded: 0, net: 0 },
      ),
    };
  });

const ledgerSchema = z.object({
  site_id: z.string().uuid().nullable().optional(),
  plan_key: z.string().max(80).nullable().optional(),
  state: z
    .enum(["all", "redeemed", "active", "used", "expired", "cancelled", "unused"])
    .default("all"),
  search: z.string().max(80).nullable().optional(),
  limit: z.number().int().min(1).max(500).default(100),
  offset: z.number().int().min(0).max(1_000_000).default(0),
});

/** Used / expired voucher inspector: plan, code, device, timestamps, status. */
export const voucherLedger = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => ledgerSchema.parse(raw ?? {}))
  .handler(async ({ data, context }) => {
    const { filterVouchers } = await import("./payments/plan-revenue");
    const ownerId = await resolveOwner(context.supabase, context.userId);
    const now = new Date();
    const nowIso = now.toISOString();

    let q = context.supabase
      .from("voucher_codes")
      .select(
        "id, code, plan_key, plan_label, price_mmk, status, created_at, first_seen_at, expires_at, device_mac, site_id, router_id",
        { count: "exact" },
      )
      .eq("owner_id", ownerId)
      .order("created_at", { ascending: false });
    if (data.site_id) q = q.eq("site_id", data.site_id);
    if (data.plan_key) q = q.eq("plan_key", data.plan_key);

    // Keep derived-state filtering in SQL so the inspector never downloads a
    // multi-thousand-row tenant ledger just to show one page. The predicates
    // mirror voucherState() below; filterVouchers still attaches the display
    // state to the returned page for one source of truth in the UI.
    const withoutCancelled = () => {
      q = q.not("status", "in", "(cancelled,deleted)");
    };
    let stateFilter: string | null = null;
    switch (data.state) {
      case "cancelled":
        q = q.in("status", ["cancelled", "deleted"]);
        break;
      case "redeemed":
        withoutCancelled();
        q = q.not("first_seen_at", "is", null);
        break;
      case "used":
        withoutCancelled();
        stateFilter = `and(first_seen_at.not.is.null,or(status.eq.expired,expires_at.lte.${nowIso}))`;
        break;
      case "active":
        withoutCancelled();
        stateFilter = `and(first_seen_at.not.is.null,status.neq.expired,or(expires_at.is.null,expires_at.gt.${nowIso}))`;
        break;
      case "expired":
        withoutCancelled();
        stateFilter = `and(first_seen_at.is.null,or(status.eq.expired,expires_at.lte.${nowIso}))`;
        break;
      case "unused":
        withoutCancelled();
        stateFilter = `and(first_seen_at.is.null,status.neq.expired,or(expires_at.is.null,expires_at.gt.${nowIso}))`;
        break;
      case "all":
        break;
    }

    const search = (data.search ?? "")
      .replace(new RegExp("[^\\p{L}\\p{N}\\s.@:+/-]", "gu"), " ")
      .replace(/\s+/g, " ")
      .trim();
    if (search) {
      const searchFilter = `or(code.ilike.*${search}*,plan_label.ilike.*${search}*,device_mac.ilike.*${search}*)`;
      q = q.or(stateFilter ? `and(${stateFilter},${searchFilter})` : searchFilter);
    } else if (stateFilter) {
      q = q.or(stateFilter);
    }

    const { data: rows, count, error } = await q.range(data.offset, data.offset + data.limit - 1);
    if (error) throw new Error(error.message);

    const filtered = filterVouchers(
      (rows ?? []) as never,
      {
        state: data.state,
        plan_key: data.plan_key ?? null,
        search: search || null,
      },
      now.getTime(),
    );
    return { total: count ?? 0, rows: filtered };
  });
