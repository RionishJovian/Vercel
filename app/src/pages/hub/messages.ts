// Maps server error codes to text keys, so every failure explains what happened and what was left unchanged.
export const enrollmentUnavailableKey = (code?: string) =>
  code === "plan_required" ? "hub.un.plan" : code === "hub_lookup_disabled" ? "hub.un.lookup" : code === "hub_enrollment_disabled" ? "hub.un.enroll" : code === "automatic_management_disabled" ? "hub.un.auto" : "hub.un.unknown";

export function enrollError(status: number | undefined, code: string | undefined): string {
  return code === "hub_management_already_configured" ? "hub.err.already"
    : code === "hub_management_preflight_failed" ? "hub.err.preflight"
    : code === "hub_router_key_unavailable" ? "hub.err.key"
    : code === "hub_recovery_unavailable" ? "hub.err.recovery"
    : status === 503 ? "hub.err.unavailable"
    : status === 403 ? "hub.err.forbidden"
    : "hub.err.generic";
}
export function finishError(status: number | undefined, code: string | undefined): string {
  return code === "hub_handshake_unverified" ? "hub.fin.handshake" : code === "hub_management_unverified" ? "hub.fin.management" : status === 503 ? "hub.fin.unavailable" : "hub.fin.generic";
}
export function lookupError(status: number | undefined, code: string | undefined, message: string): string {
  return ["subscription_required", "account_pass_required"].includes(code ?? "") ? "hub.look.ended"
    : code === "plan_required" ? "hub.look.policy" : code === "hub_lookup_disabled" ? "hub.look.disabled"
    : status === 404 ? "hub.look.notFound" : message === "unauthorized" ? "hub.look.signin" : "hub.look.generic";
}
