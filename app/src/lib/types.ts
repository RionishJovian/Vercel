export interface Me { user?: { user_id?: string; email?: string; full_name?: string; user_role?: string }; business?: { business_id?: string; company_name?: string } }
export interface Fleet { overall_state?: "up" | "stale" | "offline"; up?: number; total_peers?: number }
export interface Router { device_id: string; hardware_name?: string; deployment_layer?: string }
export interface VoucherReport {
  activation_today: number; activation_last7: number; activation_last30: number; activation_value: number;
  available_count: number; reserved_count: number; activated_count: number; expired_count: number; revoked_count: number;
  activation_time_unknown_count: number;
  gateways?: Array<{ gateway_name?: string; available_count: number; activated_count: number; activation_value: number }>;
}
export const num = (v: unknown): number | null => (typeof v === "number" && Number.isFinite(v) ? v : null);
export const asArray = <T,>(v: unknown): T[] => (Array.isArray(v) ? (v as T[]) : []);
export const mmk = (n: number) => n.toLocaleString("en-US");
