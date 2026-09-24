// Server-only role / quota guards shared by server functions.
import { canManageVoucherPrintLayouts, hasTenantPrimaryRole } from "./app-role";
import type { DatabaseClient } from "./database.types";
import { extractErrorText, toErrorMessage } from "./error-message";
import { FEATURE_DENIED } from "./operator-features";
import { BASE_DEVICE_QUOTA } from "./services/entitlements";

export type DeviceKind = "routers" | "controllers" | "sites";

const TABLE_FOR: Record<DeviceKind, string> = {
  routers: "router_connections",
  controllers: "unifi_controllers",
  sites: "sites",
};

const LABEL_FOR: Record<DeviceKind, string> = {
  routers: "router",
  controllers: "access-point controller",
  sites: "site",
};

export async function getRoles(supabase: DatabaseClient, userId: string): Promise<string[]> {
  const { data } = await supabase.from("user_roles").select("role").eq("user_id", userId);
  return ((data ?? []) as Array<{ role: string }>).map((r) => r.role);
}

/** Tenant owner row (`user_roles.owner`) — full access inside one business. */
export function isPrivileged(roles: string[]): boolean {
  return hasTenantPrimaryRole(roles);
}

/** Team Magic Developer (platform_admins) or tenant owner row. */
export function isPrivilegedAccount(roles: string[], isPlatformAdmin = false): boolean {
  return isPrivileged(roles) || isPlatformAdmin;
}

export async function isPlatformAdminUser(
  supabase: DatabaseClient,
  userId: string,
): Promise<boolean> {
  try {
    const { data, error } = await supabase.rpc("is_platform_admin", { _user_id: userId });
    if (!error && typeof data === "boolean") return data;
  } catch {
    // fall through to table lookup
  }
  try {
    const { data } = await supabase
      .from("platform_admins")
      .select("user_id")
      .eq("user_id", userId)
      .maybeSingle();
    return Boolean(data?.user_id);
  } catch {
    return false;
  }
}

/**
 * Activation gate. A pending account cannot mutate the platform or routers.
 * Privileged tenant owner bypass so Rank 1 Developers are never locked out.
 *
 * There are no account expiration rules. The Spark never expires, and a lapsed
 * Sovereign Tier Pass only reduces capacity — the voucher allowance and device
 * quota fall back to Spark levels. It never locks an operator out of their own
 * routers, because the hotspot keeps serving guests regardless.
 *
 * Name retained deliberately: 98 call sites reference it, and this is still the
 * gate every mutating server function passes through.
 */
export async function requireNotExpired(supabase: DatabaseClient, userId: string): Promise<void> {
  const [roles, isPlatformAdmin] = await Promise.all([
    getRoles(supabase, userId),
    isPlatformAdminUser(supabase, userId),
  ]);
  if (isPrivilegedAccount(roles, isPlatformAdmin)) return;
  if (roles.includes("pending")) {
    throw new Error(
      "Your account is not activated yet. You have read-only access until it is activated.",
    );
  }
}

export async function requirePrivileged(supabase: DatabaseClient, userId: string): Promise<void> {
  const [roles, isPlatformAdmin] = await Promise.all([
    getRoles(supabase, userId),
    isPlatformAdminUser(supabase, userId),
  ]);
  if (!isPrivilegedAccount(roles, isPlatformAdmin)) {
    throw new Error("Only a Primary café owner or a Developer can do this.");
  }
}

/** Active café Users and Verified Agents may manage counter receipt layouts. */
export async function requireVoucherPrintLayoutManager(
  supabase: DatabaseClient,
  userId: string,
): Promise<void> {
  await requireNotExpired(supabase, userId);
  const [roles, isPlatformAdmin] = await Promise.all([
    getRoles(supabase, userId),
    isPlatformAdminUser(supabase, userId),
  ]);
  if (!canManageVoucherPrintLayouts(roles, isPlatformAdmin)) {
    throw new Error(
      "Only an active café User, a Primary, or a Developer can manage voucher print layouts.",
    );
  }
}

/**
 * Operations feature gate. Tenant owners always pass. Vouchers, portal deploy,
 * and own-router configuration are on for every active role. Other features
 * need a Users grant. Pending and expired accounts are refused by
 * requireNotExpired.
 */
export async function requireFeature(
  supabase: DatabaseClient,
  userId: string,
  feature: import("./operator-features").GrantableFeature,
): Promise<void> {
  const { assertCanOperateFeature } = await import("./operator-grants.functions");
  await assertCanOperateFeature(supabase, userId, feature);
}

export async function hasFeature(
  supabase: DatabaseClient,
  userId: string,
  feature: import("./operator-features").GrantableFeature,
): Promise<boolean> {
  try {
    await requireFeature(supabase, userId, feature);
    return true;
  } catch {
    return false;
  }
}

/**
 * Voucher plans / codes. Portal HTML deploy uses `portal_deploy` instead.
 * Floor feature for every active role; expired/pending still blocked.
 */
export const VOUCHER_OPERATOR_DENIED = FEATURE_DENIED.vouchers;

export async function requireVoucherOperator(
  supabase: DatabaseClient,
  userId: string,
): Promise<void> {
  await requireFeature(supabase, userId, "vouchers");
}

export async function effectiveOwner(supabase: DatabaseClient, userId: string): Promise<string> {
  const { data } = await supabase.rpc("effective_owner", { _user_id: userId });
  return (data as string | null) ?? userId;
}

export async function allowanceFor(
  supabase: DatabaseClient,
  ownerId: string,
): Promise<{ routers: number; controllers: number; sites: number }> {
  const { data } = await supabase
    .from("device_allowances")
    .select("routers, controllers, sites")
    .eq("owner_id", ownerId)
    .maybeSingle();
  return {
    routers: data?.routers ?? BASE_DEVICE_QUOTA.routers,
    controllers: data?.controllers ?? BASE_DEVICE_QUOTA.controllers,
    sites: data?.sites ?? BASE_DEVICE_QUOTA.sites,
  };
}

export async function countFor(
  supabase: DatabaseClient,
  ownerId: string,
  kind: DeviceKind,
): Promise<number> {
  if (kind === "routers") {
    const { count } = await supabase
      .from("router_connections")
      .select("id", { count: "exact", head: true })
      .eq("owner_id", ownerId)
      .neq("connection_mode", "sandbox");
    return count ?? 0;
  }
  const table = kind === "controllers" ? "unifi_controllers" : "sites";
  const { count } = await supabase
    .from(table)
    .select("id", { count: "exact", head: true })
    .eq("owner_id", ownerId);
  return count ?? 0;
}

/**
 * Throws when the account already holds its allowed number of devices of this
 * kind. Tenant owners are exempt.
 */
export async function enforceDeviceQuota(
  supabase: DatabaseClient,
  userId: string,
  kind: DeviceKind,
): Promise<void> {
  const [roles, isPlatformAdmin] = await Promise.all([
    getRoles(supabase, userId),
    isPlatformAdminUser(supabase, userId),
  ]);
  if (isPrivilegedAccount(roles, isPlatformAdmin)) return;
  const ownerId = await effectiveOwner(supabase, userId);
  const [allow, used] = await Promise.all([
    allowanceFor(supabase, ownerId),
    countFor(supabase, ownerId, kind),
  ]);
  const max = allow[kind];
  if (used < max) return;
  // A ready Router key is consumed by the database quota trigger in the same
  // transaction as the successful insert. Do not reject it here first.
  if (kind === "routers") {
    const { data: key, error } = await supabase.rpc("get_router_unlock_keys");
    if (error) throw new Error(error.message);
    const active =
      key && typeof key === "object" && !Array.isArray(key)
        ? Number((key as { active_count?: unknown }).active_count ?? 0)
        : 0;
    if (active > 0) return;
  }
  const { data: pending } = await supabase
    .from("device_requests")
    .select("id")
    .eq("owner_id", ownerId)
    .eq("kind", kind)
    .eq("status", "pending")
    .maybeSingle();
  throw new Error(
    pending
      ? `You've reached your limit of ${max} ${LABEL_FOR[kind]}(s). Your request for another one is waiting for owner approval.`
      : `You've reached your limit of ${max} ${LABEL_FOR[kind]}(s). Request another slot from the app owner.`,
  );
}

/**
 * Turns the database's atomic quota trigger error into the same friendly
 * message the pre-check produces, so users never see raw SQL text.
 */
export function friendlyDeviceError(err: unknown, kind: DeviceKind): Error {
  const raw = extractErrorText(err);
  if (raw.includes("DEVICE_QUOTA_EXCEEDED")) {
    return new Error(
      `You've reached your limit of ${LABEL_FOR[kind]}(s). Request another slot from the app owner.`,
    );
  }
  return new Error(toErrorMessage(err, `Could not save ${LABEL_FOR[kind]}.`));
}

/**
 * Retry-safe add: if an identical device row already exists for this owner we
 * return it instead of creating a duplicate. Protects against double clicks and
 * client retries where the first insert actually succeeded.
 */
export async function findDuplicate(
  supabase: DatabaseClient,
  ownerId: string,
  kind: DeviceKind,
  match: Record<string, string | number>,
): Promise<string | null> {
  const table =
    kind === "routers"
      ? "router_connections"
      : kind === "controllers"
        ? "unifi_controllers"
        : "sites";
  let query = supabase.from(table).select("id").eq("owner_id", ownerId);
  for (const [col, value] of Object.entries(match)) query = query.eq(col, value);
  const { data } = await query.limit(1).maybeSingle();
  return (data?.id as string | undefined) ?? null;
}
