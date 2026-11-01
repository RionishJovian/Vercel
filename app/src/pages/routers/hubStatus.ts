import { useEffect, useState } from "react";
import { api } from "../../lib/api";

/**
 * Whether a router's Magic Hub tunnel has a recent handshake. This is the only live
 * signal for "reachable" the app has: the router row's own online flag is not kept
 * up to date, so it is never used for the label. A failed or disabled lookup is
 * "unknown", never "offline".
 */
export type HubState = "up" | "down" | "unknown";
export interface HubStatus { state: HubState; at?: string }

interface LivePeer { handshake_status?: string; last_handshake_at?: string }

export function hubStatusOf(peer: LivePeer | null): HubStatus {
  if (!peer) return { state: "unknown" };
  if (peer.handshake_status === "recent_handshake") return { state: "up", at: peer.last_handshake_at };
  if (peer.handshake_status === "inactive" || peer.handshake_status === "no_recent_handshake") return { state: "down", at: peer.last_handshake_at };
  return { state: "unknown" };
}

const REFRESH_MS = 30_000;

/** Looks up each gateway once, then every 30 s while the page is open and visible. */
export function useHubStatus(gatewayIds: string[]): Record<string, HubStatus> {
  const key = gatewayIds.join(",");
  const [status, setStatus] = useState<Record<string, HubStatus>>({});
  useEffect(() => {
    const ids = key ? key.split(",") : [];
    let live = true;
    async function lookup() {
      if (document.visibilityState === "hidden") return;
      const next = await Promise.all(ids.map(async (id): Promise<[string, HubStatus]> => {
        try { return [id, hubStatusOf(await api.get<LivePeer>(`/vps/peers/${encodeURIComponent(id)}/live`))]; }
        catch { return [id, { state: "unknown" }]; }
      }));
      if (live) setStatus(Object.fromEntries(next));
    }
    void lookup();
    const timer = window.setInterval(() => void lookup(), REFRESH_MS);
    return () => { live = false; window.clearInterval(timer); };
  }, [key]);
  return status;
}
