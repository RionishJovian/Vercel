/**
 * Canonical `app_role` string literals — keep DB and app in sync.
 *
 * `primary` — Application owner / producer (formerly `owner`); does not own a café.
 * `client` (User) — café shop owner running their own RouterBOARD Hotspot business.
 * `agent` (Verified Agent) — hired by the Application owner to recruit café Users;
 *   Magic Coins commission on monthly/annual purchases from referred Users.
 * Developer (`platform_admins`) builds features and also does not own a café.
 *
 * `user_roles.owner_id` records direct parentage; operational data remains
 * self-scoped through `effective_owner()` for client and agent accounts.
 * Verified Agent referrals use `account_referrals.agent_id` for Magic Coins
 * (recruitment attribution — not café ownership).
 */
export const TENANT_PRIMARY_ROLE = "primary" as const;
export const TENANT_CLIENT_ROLE = "client" as const;
export const TENANT_AGENT_ROLE = "agent" as const;
export const TENANT_EXPIRED_ROLE = "expired" as const;
export const TENANT_PENDING_ROLE = "pending" as const;

export type TenantPrimaryRole = typeof TENANT_PRIMARY_ROLE;

export function isTenantPrimaryRole(role: string): role is TenantPrimaryRole {
  return role === TENANT_PRIMARY_ROLE;
}

export function hasTenantPrimaryRole(roles: readonly string[] | null | undefined): boolean {
  return (roles ?? []).includes(TENANT_PRIMARY_ROLE);
}

export function hasTenantClientRole(roles: readonly string[] | null | undefined): boolean {
  return (roles ?? []).includes(TENANT_CLIENT_ROLE);
}

/**
 * Rank 1 Developer (`platform_admins`) or Rank 2 Primary (`user_roles.primary`).
 * Prefer this over `hasTenantPrimaryRole` alone whenever platform staff should
 * keep access — Developer outranks Primary and is platform-wide.
 */
export function isPrivilegedAccount(
  roles: readonly string[] | null | undefined,
  isPlatformAdmin = false,
): boolean {
  return isPlatformAdmin || hasTenantPrimaryRole(roles);
}

/**
 * Counter receipt layouts are tenant business settings. Active café Users,
 * Verified Agents, Primary, and Developer accounts may manage layouts.
 * Pending and expired accounts remain excluded.
 */
export function canManageVoucherPrintLayouts(
  roles: readonly string[] | null | undefined,
  isPlatformAdmin = false,
): boolean {
  const list = roles ?? [];
  if (list.includes(TENANT_PENDING_ROLE) || list.includes(TENANT_EXPIRED_ROLE)) return false;
  return (
    isPrivilegedAccount(list, isPlatformAdmin) ||
    hasTenantClientRole(list) ||
    list.includes(TENANT_AGENT_ROLE)
  );
}

/** Ledger reconciliation can alter voucher accounting, so it is owner-only. */
export function canReconcileVoucherLedger(
  roles: readonly string[] | null | undefined,
  isPlatformAdmin = false,
): boolean {
  return isPrivilegedAccount(roles, isPlatformAdmin);
}
