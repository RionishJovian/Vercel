import useSWR from "swr";
import { fetcher } from "./api";

export interface FleetPeer { device_id?: string; hardware_name?: string; state?: string; source?: string; last_contact_at?: string; checked_at?: string; state_since?: string }
export interface FleetData { peers?: FleetPeer[]; total_peers?: number; up?: number; stale?: number; offline?: number; unknown?: number; overall_state?: string }

/** Every page's router status. The server decides it in one place; nothing here changes it. */
export function useFleet(options?: { shouldRetryOnError?: boolean }) {
  return useSWR<FleetData>("/fleet/health", fetcher, { refreshInterval: 30000, revalidateOnFocus: true, ...options });
}
