/** Pure helpers for first-login (“ticket activated”) detection. */

export const TICKET_ACTIVATED_KIND = "ticket_activated";

function hasPositiveCounter(value: unknown): boolean {
  const number = Number(value);
  return Number.isFinite(number) && number > 0;
}

/** RouterOS returns zero uptime in several equivalent string formats. */
export function hasPositiveRouterUptime(value: unknown): boolean {
  if (typeof value === "number") return value > 0;
  const text = String(value ?? "").trim().toLowerCase();
  if (!text || ["0", "0s", "none", "never", "00:00:00"].includes(text)) return false;

  if (/^\d+(?::\d{1,2}){1,3}$/.test(text)) {
    return text.split(":").some((part) => Number(part) > 0);
  }

  const units = [...text.matchAll(/(\d+(?:\.\d+)?)(ms|s|m|h|d|w)/g)];
  return units.length > 0 && units.some(([, amount]) => Number(amount) > 0);
}

export function voucherHasBeenUsed(input: {
  active?: Record<string, string> | null;
  user?: Record<string, string> | null;
}): boolean {
  const user = input.user;
  const usedBytes = hasPositiveCounter(user?.["bytes-in"]) || hasPositiveCounter(user?.["bytes-out"]);
  return (
    Boolean(input.active) ||
    usedBytes ||
    hasPositiveRouterUptime(user?.["uptime"])
  );
}

export function ticketActivationNotice(input: {
  code: string;
  planLabel: string;
  mac?: string | null;
}): { kind: typeof TICKET_ACTIVATED_KIND; title: string; body: string } {
  const mac = input.mac?.trim();
  return {
    kind: TICKET_ACTIVATED_KIND,
    title: `Ticket activated — ${input.planLabel}`,
    body: mac ? `${input.code} came online (${mac}).` : `${input.code} came online.`,
  };
}
