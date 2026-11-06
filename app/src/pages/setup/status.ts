/** Quick setup progress, worked out from what the app and router actually report. A click never marks a step done. */
export type StepKey = "site" | "connect" | "hotspot" | "guest" | "install";
export type StepState = "done" | "current" | "todo";
export interface GatewayLike { device_id: string; management_connected?: boolean }
export interface SetupInput {
  sites: number;
  gateways: GatewayLike[];
  /** The router remembered from an earlier page; ignored unless it is still a gateway. */
  selected: string;
  /** Shared router status, keyed by device id. */
  states: ReadonlyMap<string | undefined, string>;
  /** Hotspot login profiles read from the router. Undefined while unread or when the read failed. */
  hotspotProfiles?: number;
  /** When the guest page was last saved (`updated_at` of the saved portal). Empty or the zero time means never saved. */
  portalSavedAt?: string;
  /** When a verified install of the guest page was last recorded for this router. */
  portalInstalledAt?: string;
}
const when = (v?: string): number => { const n = v ? Date.parse(v) : NaN; return Number.isNaN(n) || n <= Date.parse("1971-01-01") ? 0 : n; };
export interface SetupStatus {
  router?: GatewayLike;
  /** The router answers on the Hub tunnel right now. Separate from `connect`, which means management access is verified. */
  live: boolean;
  steps: Array<{ key: StepKey; state: StepState }>;
}

export function setupStatus(i: SetupInput): SetupStatus {
  const router = i.gateways.find((g) => g.device_id === i.selected) ?? (i.gateways.length === 1 ? i.gateways[0] : undefined);
  const siteDone = i.sites > 0;
  const connected = router?.management_connected === true;
  const hotspotDone = connected && (i.hotspotProfiles ?? 0) > 0;
  const saved = when(i.portalSavedAt), installed = when(i.portalInstalledAt);
  const guestDone = hotspotDone && saved > 0;
  // Installed means a verified install was recorded after the latest save; a later edit needs installing again.
  const installDone = guestDone && installed > 0 && installed >= saved;
  return {
    router,
    live: !!router && i.states.get(router.device_id) === "up",
    steps: [
      { key: "site", state: siteDone ? "done" : "current" },
      { key: "connect", state: !siteDone ? "todo" : connected ? "done" : "current" },
      // Done only when the router itself reports a Hotspot profile; a form submission alone proves nothing.
      { key: "hotspot", state: !connected ? "todo" : hotspotDone ? "done" : "current" },
      { key: "guest", state: !hotspotDone ? "todo" : guestDone ? "done" : "current" },
      { key: "install", state: !guestDone ? "todo" : installDone ? "done" : "current" },
    ],
  };
}
