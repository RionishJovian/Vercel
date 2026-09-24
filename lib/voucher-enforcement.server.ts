import type { RouterConn } from "./mikrotik.server";
import { isDefaultTrialUser, isHotspotTrialUser } from "./hotspot-voucher-users";

export type EnforceableVoucher = {
  id: string;
  code: string;
  owner_id: string;
  router_id: string | null;
  status: string;
  expires_at: string | null;
};

export type RouterHotspotRow = Record<string, string>;

export type EnforcementTarget = {
  voucherId: string;
  routerId: string;
  voucherCode: string;
  userId: string | null;
  activeId: string | null;
  userDisabled: boolean;
};

export type EnforcementOutcome = "already_enforced" | "targeted";

export function terminalVoucherReason(
  voucher: Pick<EnforceableVoucher, "status" | "expires_at">,
  now = Date.now(),
): "expired" | "cancelled" | null {
  const status = voucher.status.trim().toLowerCase();
  if (status === "cancelled" || status === "deleted") return "cancelled";
  if (status === "expired") return "expired";
  if (voucher.expires_at && Date.parse(voucher.expires_at) <= now) return "expired";
  return null;
}

/** Match a terminal Magic voucher to its own RouterOS user/session only. */
export function enforcementTarget(input: {
  voucher: EnforceableVoucher;
  routerId: string;
  users: RouterHotspotRow[];
  active: RouterHotspotRow[];
  now?: number;
}): { reason: "expired" | "cancelled"; target: EnforcementTarget } | null {
  const reason = terminalVoucherReason(input.voucher, input.now);
  if (!reason || input.voucher.router_id !== input.routerId) return null;

  const user = input.users.find((row) => row.name === input.voucher.code);
  const active = input.active.find((row) => row.user === input.voucher.code);
  if (isDefaultTrialUser(input.voucher.code) || isHotspotTrialUser(user)) return null;

  return {
    reason,
    target: {
      voucherId: input.voucher.id,
      routerId: input.routerId,
      voucherCode: input.voucher.code,
      userId: user?.[".id"] ?? null,
      activeId: active?.[".id"] ?? null,
      userDisabled: user?.disabled?.toLowerCase() === "yes",
    },
  };
}

function isAlreadyGone(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /\b(?:404|not found|no such item|no such entry)\b/i.test(message);
}

/** Disconnect runtime access, then disable (never delete) the persistent user. */
export async function enforceVoucherTarget(input: {
  conn: RouterConn;
  target: EnforcementTarget;
  removeActive: (conn: RouterConn, id: string) => Promise<unknown>;
  patchUser: (
    conn: RouterConn,
    id: string,
    patch: Record<string, string | number | undefined>,
  ) => Promise<unknown>;
}): Promise<EnforcementOutcome> {
  let changed = false;
  if (input.target.activeId) {
    try {
      await input.removeActive(input.conn, input.target.activeId);
      changed = true;
    } catch (error) {
      if (!isAlreadyGone(error)) throw error;
    }
  }
  if (input.target.userId && !input.target.userDisabled) {
    await input.patchUser(input.conn, input.target.userId, { disabled: "yes" });
    changed = true;
  }
  return changed ? "targeted" : "already_enforced";
}
