/** Quick setup progress, worked out from what the app and router actually report. A click never marks a step done. */
export type StepKey = "site" | "connect";
export type StepState = "done" | "current" | "todo";
export interface GatewayLike { device_id: string; management_connected?: boolean }
export interface SetupInput {
  sites: number;
  gateways: GatewayLike[];
  /** The router remembered from an earlier page; ignored unless it is still a gateway. */
  selected: string;
  /** Shared router status, keyed by device id. */
  states: ReadonlyMap<string | undefined, string>;
}
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
  return {
    router,
    live: !!router && i.states.get(router.device_id) === "up",
    steps: [
      { key: "site", state: siteDone ? "done" : "current" },
      { key: "connect", state: !siteDone ? "todo" : connected ? "done" : "current" },
    ],
  };
}
