import useSWR from "swr";
import { fetcher } from "./api";
import { asArray, type Router } from "./types";
import { useHubStatus, type HubStatus } from "./hubStatus";

export interface FleetPeer { device_id?: string; hardware_name?: string; state?: string }
export interface FleetData { peers?: FleetPeer[]; total_peers?: number; up?: number; stale?: number; offline?: number; unknown?: number; overall_state?: string }

const SEVERITY: Record<string, number> = { unknown: 0, up: 1, stale: 2, offline: 3 };

/**
 * The server's fleet health only knows what recorded heartbeats say, and a router
 * that is connected through Magic Hub may have none. Where the Hub's live handshake
 * answers, it decides the state, so every page agrees with the Router tab; where it
 * cannot answer, the recorded state stands unchanged.
 */
export function mergeFleet(fleet: FleetData | undefined, hub: Record<string, HubStatus>): FleetData | undefined {
  if (!fleet) return fleet;
  const peers = asArray<FleetPeer>(fleet.peers).map((p) => {
    const h = p.device_id ? hub[p.device_id] : undefined;
    return h && h.state !== "unknown" ? { ...p, state: h.state === "up" ? "up" : "offline" } : p;
  });
  const count = (s: string) => peers.filter((p) => String(p.state ?? "unknown").toLowerCase() === s).length;
  const counted = { up: count("up"), stale: count("stale"), offline: count("offline") };
  const overall = peers.reduce((worst, p) => { const s = String(p.state ?? "unknown").toLowerCase(); return (SEVERITY[s] ?? 0) > (SEVERITY[worst] ?? 0) ? s : worst; }, "unknown");
  return { ...fleet, peers, ...counted, unknown: peers.length - counted.up - counted.stale - counted.offline, total_peers: fleet.total_peers ?? peers.length, overall_state: peers.length ? overall : fleet.overall_state };
}

/** Fleet health with the live Magic Hub handshake applied. Same shape as SWR's result. */
export function useFleet(options?: { shouldRetryOnError?: boolean }) {
  const fleet = useSWR<FleetData>("/fleet/health", fetcher, { refreshInterval: 30000, revalidateOnFocus: true, ...options });
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const gateways = asArray<Router>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY").map((r) => r.device_id);
  const hub = useHubStatus(gateways);
  return { ...fleet, data: mergeFleet(fleet.data, hub) };
}
