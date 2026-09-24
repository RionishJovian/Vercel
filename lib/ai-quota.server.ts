// Server-only: shared manual AI scan quota logic.
//
// Primary café owners and Rank 1 Developers are unlimited. Standard accounts get a hard monthly
// allowance that refills on the 1st (app time, UTC+06:30). Owners can override
// the monthly grant per account in Users.

import { DEFAULT_MONTHLY_AI_SCAN_LIMIT, computeAiScanQuota } from "./ai-scan-quota";
import { TENANT_PRIMARY_ROLE } from "./app-role";
import type { DatabaseClient } from "./database.types";

export async function aiScanQuota(context: { supabase: DatabaseClient; userId: string }) {
  const { appStartOfMonth, appMonthLabel, appMonthIndex } = await import("./time");
  const g = await import("./guards.server");
  const [{ data: isOwner }, isPlatformAdmin] = await Promise.all([
    context.supabase.rpc("has_role", { _user_id: context.userId, _role: TENANT_PRIMARY_ROLE }),
    g.isPlatformAdminUser(context.supabase, context.userId),
  ]);
  const { data: isExpired } = await context.supabase.rpc("is_expired", {
    _user_id: context.userId,
  });
  const unlimited = Boolean(isOwner) || isPlatformAdmin;

  const periodStart = appStartOfMonth();
  const periodStartIso = new Date(periodStart).toISOString();

  const [{ data: override }, { count }] = await Promise.all([
    context.supabase
      .from("ai_scan_limits")
      .select("monthly_limit")
      .eq("user_id", context.userId)
      .maybeSingle(),
    context.supabase
      .from("fleet_scan_runs")
      .select("id", { count: "exact", head: true })
      .eq("triggered_by", context.userId)
      .gte("generated_at", periodStartIso),
  ]);

  const monthlyGrant =
    typeof override?.monthly_limit === "number"
      ? override.monthly_limit
      : DEFAULT_MONTHLY_AI_SCAN_LIMIT;

  const math = computeAiScanQuota({
    plus: false,
    monthlyGrant,
    used: count ?? 0,
    monthIndex: appMonthIndex(),
  });

  return {
    unlimited,
    expired: Boolean(isExpired),
    plus: false,
    plusBonus: 0,
    monthlyGrant: math.monthlyGrant,
    carry: math.carry,
    limit: math.limit,
    used: math.used,
    remaining: unlimited ? null : math.remaining,
    periodStart,
    periodLabel: appMonthLabel(),
  };
}

/** Throws a user-facing error when the caller has no manual scans left. */
export function assertScanAllowed(quota: Awaited<ReturnType<typeof aiScanQuota>>) {
  if (quota.expired) throw new Error("Expired accounts cannot run AI scans.");
  if (!quota.unlimited && (quota.remaining ?? 0) <= 0) {
    if (quota.limit === 0) {
      throw new Error(
        "AI scans are disabled for your account. Ask the app owner to raise your scan limit.",
      );
    }
    if (quota.carry) {
      throw new Error(
        `You have used all ${quota.limit} AI scans accrued for ${quota.periodLabel} (${quota.monthlyGrant}/month, unused carries until 1 January). Ask the app owner to approve more, or wait for the next month's grant.`,
      );
    }
    throw new Error(
      `You have used all ${quota.limit} AI scans for ${quota.periodLabel}. Ask the app owner or developer to approve more, or wait for the refill on the 1st.`,
    );
  }
}
