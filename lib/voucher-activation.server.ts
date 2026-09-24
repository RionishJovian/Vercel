import type { DatabaseClient } from "./database.types";
import {
  TICKET_ACTIVATED_KIND,
  ticketActivationNotice,
  voucherHasBeenUsed,
} from "./voucher-activation";

export type HotspotRow = Record<string, string>;

type VoucherCodeRow = {
  id: string;
  code: string;
  owner_id: string;
  plan_label: string;
  price_mmk: number;
  duration_minutes: number | null;
  device_mac: string | null;
  first_seen_at: string | null;
  expires_at: string | null;
  status: string;
};

export type FirstUseMark = {
  id: string;
  first_seen_at: string;
  expires_at: string | null;
  device_mac: string | null;
};

const SKIP_NOTIFY_ROLES = new Set(["pending", "expired", "read_only"]);

async function alertsEnabled(admin: DatabaseClient, ownerId: string): Promise<boolean> {
  const { data, error } = await admin
    .from("portal_settings")
    .select("notify_ticket_activation")
    .eq("owner_id", ownerId)
    .maybeSingle();
  if (error) return false;
  return Boolean(
    (data as { notify_ticket_activation?: boolean | null } | null)?.notify_ticket_activation,
  );
}

async function tenantRecipients(admin: DatabaseClient, ownerId: string): Promise<string[]> {
  const ids = new Set<string>([ownerId]);
  const { data } = await admin
    .from("user_roles")
    .select("user_id, role")
    .or(`user_id.eq.${ownerId},owner_id.eq.${ownerId}`);
  for (const row of data ?? []) {
    if (SKIP_NOTIFY_ROLES.has(row.role)) continue;
    ids.add(row.user_id);
  }
  return [...ids];
}

async function notifyActivation(
  admin: DatabaseClient,
  ownerId: string,
  code: VoucherCodeRow,
  mac: string | null,
): Promise<void> {
  const notice = ticketActivationNotice({
    code: code.code,
    planLabel: code.plan_label,
    mac,
  });
  const recipients = await tenantRecipients(admin, ownerId);
  if (!recipients.length) return;
  await admin.from("admin_notifications").insert(
    recipients.map((recipient_id) => ({
      recipient_id,
      kind: TICKET_ACTIVATED_KIND,
      title: notice.title,
      body: notice.body,
      data: {
        code: code.code,
        plan_label: code.plan_label,
        voucher_id: code.id,
        mac,
      },
    })),
  );
}

/**
 * Mark unused voucher codes as first-used when they appear on the router.
 * When the tenant toggle is on, fan out an in-app notice only.
 */
export async function markFirstUsesFromHotspot(input: {
  supabaseAdmin: DatabaseClient;
  routerId: string;
  users: HotspotRow[];
  active: HotspotRow[];
  codes?: VoucherCodeRow[];
  nowMs?: number;
}): Promise<{ activations: FirstUseMark[] }> {
  const usersByName = new Map(input.users.map((u) => [u["name"] ?? "", u]));
  const activeByUser = new Map(input.active.map((a) => [a["user"] ?? "", a]));

  let codes = input.codes;
  if (!codes) {
    const { data } = await input.supabaseAdmin
      .from("voucher_codes")
      .select(
        "id, code, owner_id, plan_label, price_mmk, duration_minutes, device_mac, first_seen_at, expires_at, status",
      )
      .eq("router_id", input.routerId)
      .neq("status", "cancelled");
    codes = (data ?? []) as VoucherCodeRow[];
  }

  const unused = codes.filter((c) => !c.first_seen_at);
  if (unused.length === 0) return { activations: [] };

  const ownerId = unused[0]!.owner_id;
  const notify = await alertsEnabled(input.supabaseAdmin, ownerId);
  const activations: FirstUseMark[] = [];

  for (const code of unused) {
    const user = usersByName.get(code.code);
    const active = activeByUser.get(code.code);
    if (!voucherHasBeenUsed({ user, active })) continue;

    const mac = active?.["mac-address"] ?? user?.["mac-address"] ?? code.device_mac ?? null;
    // The atomic claim function ships in a migration that is not reflected in
    // the generated types yet, so the RPC name/return shape is asserted here.
    const { data: claimed, error: claimError } = await (
      input.supabaseAdmin.rpc as unknown as (
        fn: string,
        args: Record<string, unknown>,
      ) => Promise<{
        data:
          | Array<{ first_seen_at: string | null; expires_at: string | null; device_mac: string | null }>
          | { first_seen_at: string | null; expires_at: string | null; device_mac: string | null }
          | null;
        error: unknown;
      }>
    )("claim_voucher_first_use", {
      _router_id: input.routerId,
      _voucher_id: code.id,
      _device_mac: mac,
    });
    // Do not fall back to an application-side update if the atomic function is
    // unavailable. A missing security migration must fail closed, not reopen
    // the race this function is responsible for preventing.
    if (claimError) continue;
    const updated = Array.isArray(claimed) ? claimed[0] : claimed;
    if (!updated?.first_seen_at) continue;

    const mark: FirstUseMark = {
      id: code.id,
      first_seen_at: updated.first_seen_at,
      expires_at: updated.expires_at,
      device_mac: updated.device_mac,
    };
    activations.push(mark);
    if (notify) {
      await notifyActivation(input.supabaseAdmin, ownerId, code, mac).catch(() => null);
    }
  }

  return { activations };
}
