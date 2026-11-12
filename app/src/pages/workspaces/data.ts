import useSWR from "swr";
import { api, ApiError, fetcher } from "../../lib/api";
import { useFleet, type FleetData } from "../../lib/useFleet";
import { asArray, num, type VoucherReport } from "../../lib/types";
import type { RouterRow } from "../routers";
import type { Tone } from "../../components/ui";

/** Shared read models for the workspace screens. Every figure comes from an existing API; nothing is estimated. */

export function useRouters() {
  const r = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  const all = r.data ? asArray<RouterRow>(r.data.routers) : null;
  return { ...r, all, gateways: all ? all.filter((x) => x.deployment_layer === "MAIN_GATEWAY") : null };
}

export const useVoucherReport = () => useSWR<VoucherReport>("/voucher-report", fetcher);

/** The newest router check the fleet monitor reports, as evidence time for the whole fleet. */
export function fleetCheckedAt(f?: FleetData): string | undefined {
  let best = 0, at: string | undefined;
  for (const p of f?.peers ?? []) {
    const v = Date.parse(p.checked_at ?? p.last_contact_at ?? "");
    if (Number.isFinite(v) && v > best) { best = v; at = p.checked_at ?? p.last_contact_at; }
  }
  return at;
}

/** Fleet summary in the shared status model. No routers or no evidence is "idle", never "offline". */
export function useFleetSummary() {
  const fleet = useFleet();
  const d = fleet.data;
  const total = num(d?.total_peers), up = num(d?.up) ?? 0, stale = num(d?.stale) ?? 0, offline = num(d?.offline) ?? 0;
  const unknown = total === null ? 0 : Math.max(num(d?.unknown) ?? 0, total - up - stale - offline);
  let tone: Tone = "idle", key = "st.unknown";
  if (fleet.error) { tone = "idle"; key = "st.noEvidence"; }
  else if (total === 0) { tone = "idle"; key = "st.noRouters"; }
  else if (total !== null) {
    if (offline > 0) { tone = "bad"; key = "st.offline"; }
    else if (stale > 0) { tone = "warn"; key = "st.stale"; }
    else if (unknown > 0) { tone = "idle"; key = "st.unknown"; }
    else if (up === total) { tone = "ok"; key = "st.online"; }
  }
  return { fleet, total, up, stale, offline, unknown, tone, key, checkedAt: fleetCheckedAt(d) };
}

export interface BroadbandAccount {
  id: string; username: string; profile: string; pending_operation?: string; observed_at?: string;
  observation: { first_use: number; expires_at: number; suspended: boolean; last_clock: number } | null;
  session?: { id: string; download_bytes: number; upload_bytes: number; uptime_seconds: number } | null; session_observed_at?: string;
}
export interface SubscriberRow extends BroadbandAccount { routerId: string; routerName: string }
export interface PppoeSummary {
  rows: SubscriberRow[]; total: number; connected: number; suspended: number; expired: number; pending: number;
  writeEnabled: boolean | null; failed: Array<{ routerName: string; reason: string }>; newestObservation?: string;
}

/** Subscriber service state from the router record. Connection is only claimed from a recorded session. */
export function subscriberState(a: BroadbandAccount, now = Date.now() / 1000): { key: string; tone: Tone } {
  if (a.pending_operation) return { key: "isp.s.pending", tone: "info" };
  if (!a.observation) return { key: "isp.s.unverified", tone: "idle" };
  if (a.observation.suspended) return { key: "isp.s.suspended", tone: "warn" };
  if (a.observation.expires_at && a.observation.expires_at < now) return { key: "isp.s.expired", tone: "bad" };
  return { key: "isp.s.active", tone: "ok" };
}

/** PPPoE subscriber records across the business's gateways (normally one router per business). */
export function usePppoe(gateways: RouterRow[] | null) {
  const key = gateways && gateways.length ? ["pppoe-summary", ...gateways.map((g) => g.device_id)] : null;
  return useSWR<PppoeSummary>(key, async () => {
    const rows: SubscriberRow[] = [], failed: PppoeSummary["failed"] = [];
    let writeEnabled: boolean | null = null;
    await Promise.all((gateways ?? []).map(async (g) => {
      const name = g.hardware_name || g.device_id;
      try {
        const r = await api.get<{ accounts?: BroadbandAccount[]; write_enabled?: boolean }>(`/routers/${encodeURIComponent(g.device_id)}/broadband/accounts`);
        writeEnabled = (writeEnabled ?? false) || !!r.write_enabled;
        for (const a of asArray<BroadbandAccount>(r.accounts)) rows.push({ ...a, routerId: g.device_id, routerName: name });
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) throw e;
        failed.push({ routerName: name, reason: e instanceof ApiError ? e.message : "network-unreachable" });
      }
    }));
    const now = Date.now() / 1000;
    let newest = 0, newestObservation: string | undefined;
    for (const r of rows) { const v = Date.parse(r.session_observed_at ?? r.observed_at ?? ""); if (Number.isFinite(v) && v > newest) { newest = v; newestObservation = r.session_observed_at ?? r.observed_at; } }
    const states = rows.map((r) => subscriberState(r, now).key);
    return {
      rows, failed, writeEnabled, newestObservation, total: rows.length,
      connected: rows.filter((r) => !!r.session).length,
      suspended: states.filter((s) => s === "isp.s.suspended").length,
      expired: states.filter((s) => s === "isp.s.expired").length,
      pending: states.filter((s) => s === "isp.s.pending").length,
    };
  }, { shouldRetryOnError: false });
}

export const useBroadbandReport = () => useSWR<{ amount_mmk: number; today_mmk: number; payment_count: number; pending_count: number }>("/broadband/report", fetcher, { shouldRetryOnError: false });

export interface Incident { id: string; severity?: string; kind?: string; subject_label?: string; subject_id?: string; detail?: string; opened_at?: string; suggested_action?: { kind?: string } }
export const useIncidents = () => useSWR<{ incidents?: Incident[] }>("/incidents", fetcher, { shouldRetryOnError: false });
export const incidentTone = (s?: string): Tone => (({ critical: "bad", warning: "warn", warn: "warn" } as Record<string, Tone>)[String(s).toLowerCase()] ?? "info");

export const bytes = (n?: number) => {
  if (typeof n !== "number" || !Number.isFinite(n)) return "—";
  const u = ["B", "KB", "MB", "GB", "TB"]; let i = 0, v = n;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v < 10 && i ? v.toFixed(1) : Math.round(v)} ${u[i]}`;
};
export const duration = (s?: number) => {
  if (typeof s !== "number" || !Number.isFinite(s)) return "—";
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
};
