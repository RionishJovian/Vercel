import { createServerFn } from "@tanstack/react-start";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import { buildRevenueEntries, totalsFor } from "./payments/accounting";

/**
 * Business-oriented snapshot for the owner landing page. Revenue uses the
 * same used-voucher accounting stream as the Revenue page. Live guest count
 * is read from the same RouterOS snapshot path as the Live and Easy Guests
 * screens so the Home summary cannot drift from the actual hotspot.
 */
export const getBusinessSnapshot = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const { supabase } = context;
    const { effectiveOwner } = await import("./guards.server");
    const ownerId = await effectiveOwner(supabase, context.userId);
    const { appStartOfDay, appStartOfDaysAgo } = await import("./time");

    const startToday = appStartOfDay();
    const start7 = appStartOfDaysAgo(6);
    const start30 = appStartOfDaysAgo(29);
    const dayStarts = Array.from({ length: 7 }, (_, i) => appStartOfDaysAgo(6 - i));

    const [routers, vouchers, sales, orders, sites] = await Promise.all([
      supabase
        .from("router_connections")
        .select("id, name, site_id, connection_mode, is_virtual")
        .eq("owner_id", ownerId),
      supabase
        .from("voucher_codes")
        .select(
          "code, plan_key, plan_label, price_mmk, status, first_seen_at, expires_at, site_id, router_id, order_id",
        )
        .eq("owner_id", ownerId)
        .limit(5000),
      supabase
        .from("voucher_sales")
        .select("code, profile, price_cents, sold_at, site_id, router_id")
        .eq("owner_id", ownerId),
      supabase
        .from("payment_orders")
        .select(
          "id, status, method, amount_minor, plan_label, issued_code, site_id, router_id, settled_at, refunded_at, created_at",
        )
        .eq("owner_id", ownerId)
        .limit(5000),
      supabase.from("sites").select("id, name").eq("owner_id", ownerId),
    ]);
    if (vouchers.error) throw new Error(vouchers.error.message);
    if (sales.error) throw new Error(sales.error.message);
    if (orders.error) throw new Error(orders.error.message);

    // Keep Home and Revenue on one financial definition: a code becomes
    // revenue only after the app ledger classifies it as Used. RouterOS-only
    // hotspot accounts cannot enter this stream because they have no ledger row.
    const entries = buildRevenueEntries({
      orders: (orders.data ?? []) as never,
      vouchers: (vouchers.data ?? []) as never,
      legacySales: (sales.data ?? []) as never,
    });
    const revenueToday = totalsFor(entries, startToday).net;
    const revenue7 = totalsFor(entries, start7).net;
    const revenue30 = totalsFor(entries, start30).net;

    const routerRows = (routers.data ?? []) as Array<{
      id: string;
      name: string;
      site_id: string | null;
      connection_mode?: string | null;
      is_virtual?: boolean | null;
    }>;

    const physicalRouters = routerRows.filter(
      (router) => router.is_virtual !== true && router.connection_mode !== "sandbox",
    );
    let activeSessions: number | null = null;
    if (physicalRouters.length) {
      const { loadRouterConn } = await import("./router-conn.server");
      const { routerAPI } = await import("./mikrotik.server");
      const liveReads = await Promise.allSettled(
        physicalRouters.map(async (router) => {
          const connection = await loadRouterConn(supabase, router.id);
          return routerAPI.activeUsers(connection);
        }),
      );
      const successfulReads = liveReads.filter(
        (read): read is PromiseFulfilledResult<Record<string, string>[]> =>
          read.status === "fulfilled",
      );
      if (successfulReads.length) {
        activeSessions = successfulReads.reduce(
          (total, read) => total + (read.value?.length ?? 0),
          0,
        );
      }
    }

    const { bucketRevenueEntriesByDay } = await import("./revenue-daily");
    const revenueDaily7 = bucketRevenueEntriesByDay(entries, dayStarts);

    return {
      revenue: { today: revenueToday, last7: revenue7, last30: revenue30, currency: "MMK" },
      revenueDaily7,
      activeSessions,
      sites: ((sites.data ?? []) as Array<{ id: string; name: string }>).map((s) => ({
        id: s.id,
        name: s.name,
        routerIds: routerRows.filter((r) => r.site_id === s.id).map((r) => r.id),
      })),
    };
  });
