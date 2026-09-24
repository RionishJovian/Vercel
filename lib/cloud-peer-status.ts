export type FreshCloudPeerStatus = "online" | "offline" | "connecting" | "error";

export const CLOUD_PEER_FRESHNESS_MS = 5 * 60 * 1000;

/** A historical WireGuard handshake is not proof that the tunnel is online now. */
export function cloudPeerStatusFromHandshake(
  state: string,
  lastHandshakeAt: string | null | undefined,
  nowMs = Date.now(),
): FreshCloudPeerStatus {
  if (state === "disabled") return "error";
  if (!lastHandshakeAt) return "connecting";
  const handshakeMs = Date.parse(lastHandshakeAt);
  if (!Number.isFinite(handshakeMs)) return "error";
  return nowMs - handshakeMs <= CLOUD_PEER_FRESHNESS_MS ? "online" : "offline";
}
