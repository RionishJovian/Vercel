// Switch / PoE port model shared by every vendor driver and the UI.
//
// Pure and browser-safe. The safety rules here are the last line of defence
// before a power-cycle: they refuse to cut power to the port the app is
// reaching the device through, and to uplinks feeding other infrastructure.

import type { DeviceDescriptor } from "./vendors";
import { capability } from "./vendors";

export type PortRole = "access" | "trunk" | "uplink" | "unknown";

export type PoeState = "on" | "off" | "fault" | "unsupported" | "unknown";

export type SwitchPort = {
  /** Driver-native identifier used for writes. */
  ref: string;
  name: string;
  index: number;
  enabled: boolean;
  link: "up" | "down" | "unknown";
  /** Negotiated speed in Mbps, null when the link is down or unreadable. */
  speedMbps: number | null;
  duplex: "full" | "half" | null;
  role: PortRole;
  vlan: number | null;
  poe: PoeState;
  /** Draw in watts, null when the switch cannot report it. */
  poeWatts: number | null;
  /** MAC of an access point associated with this port, when known. */
  apMac: string | null;
  apName: string | null;
  /** True when this port carries the app's own management path. */
  isManagement: boolean;
  description: string | null;
};

export type PoeBudget = {
  /** Total budget in watts, null when the switch does not report one. */
  totalWatts: number | null;
  usedWatts: number | null;
};

export type SwitchPortView = {
  ports: SwitchPort[];
  budget: PoeBudget;
  /** True when the numbers come from the device; false when cached/unknown. */
  measured: boolean;
  fetchedAt: string;
};

export type PoeGuard = { allowed: true } | { allowed: false; reason: string };

/**
 * Decide whether a PoE power-cycle on this port is safe. Refuses when the
 * vendor cannot do it, when the port is the management path, when the port is
 * an uplink/trunk, and when PoE is not actually delivering power.
 */
export function poeCycleGuard(device: DeviceDescriptor, port: SwitchPort): PoeGuard {
  const cap = capability(device, "poeCycle");
  if (!cap.supported) return { allowed: false, reason: cap.reason };
  if (port.isManagement) {
    return {
      allowed: false,
      reason:
        "This port carries the app's own connection to the switch — cycling it would cut you off.",
    };
  }
  if (port.role === "uplink" || port.role === "trunk") {
    return {
      allowed: false,
      reason: "Uplink and trunk ports feed other devices, so power-cycling them is blocked.",
    };
  }
  if (port.poe === "unsupported") {
    return { allowed: false, reason: "This port does not deliver PoE." };
  }
  if (port.poe === "off") {
    return { allowed: false, reason: "PoE is already off on this port." };
  }
  if (port.poe === "unknown") {
    return {
      allowed: false,
      reason: "We cannot read this port's PoE state, so we will not change it.",
    };
  }
  return { allowed: true };
}

/** Typed confirmation text the owner must reproduce before a power-cycle. */
export function poeConfirmationPhrase(port: SwitchPort): string {
  return port.name;
}

export function checkPoeConfirmation(port: SwitchPort, typed: string): void {
  if (typed.trim() !== poeConfirmationPhrase(port)) {
    throw new Error(`Type the port name "${poeConfirmationPhrase(port)}" exactly to confirm.`);
  }
}

export function summarizePorts(view: SwitchPortView | null | undefined) {
  const ports = view?.ports ?? [];
  return {
    total: ports.length,
    up: ports.filter((p) => p.link === "up").length,
    poeOn: ports.filter((p) => p.poe === "on").length,
    aps: ports.filter((p) => p.apMac).length,
    usedWatts: view?.budget.usedWatts ?? null,
    totalWatts: view?.budget.totalWatts ?? null,
  };
}
