import { asArray } from "./types";
import type { FleetData, FleetPeer } from "./useFleet";

/** The shared fleet status in the form the Router tab's chip shows. */
export type HubState = "up" | "down" | "unknown";
export interface HubStatus { state: HubState; at?: string }

export function hubStatusOf(peer?: FleetPeer): HubStatus {
  if (!peer) return { state: "unknown" };
  const at = peer.source === "hub_tunnel" ? peer.last_contact_at : undefined;
  if (peer.state === "up") return { state: "up", at };
  if (peer.state === "offline") return { state: "down", at };
  return { state: "unknown", at };
}

export function hubStatusFrom(fleet?: FleetData): Record<string, HubStatus> {
  return Object.fromEntries(asArray<FleetPeer>(fleet?.peers).filter((p) => p.device_id).map((p) => [p.device_id as string, hubStatusOf(p)]));
}
