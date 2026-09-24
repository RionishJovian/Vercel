export type VoucherPlanIdentity = {
  plan_label?: string | null;
  plan_key?: string | null;
};

/** Prefer the customer-facing ledger label over RouterOS implementation names. */
export function voucherPlanLabel(
  ledger: VoucherPlanIdentity,
  routerProfile?: string | null,
): string {
  for (const value of [ledger.plan_label, ledger.plan_key, routerProfile]) {
    const label = value?.trim();
    if (label) return label;
  }
  return "—";
}
