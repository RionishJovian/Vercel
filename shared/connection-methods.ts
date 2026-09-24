import { isPrivilegedAccount } from "./app-role";

/**
 * Single source of truth for how operators reach a board.
 * Magic Hub (Cloud Remote dial-out) + Local Connector.
 * Inbound HTTPS Quick Setup was removed from the product.
 *
 * RemoteAccessChooser and the Routers "Connection method" fieldset MUST
 * derive from this list so the same page never shows two different counts.
 */

export type ConnMethodId = "hub" | "connector";

export type ConnectionMethodDef = {
  id: ConnMethodId;
  title: string;
  /** Short body for the Add-router fieldset and chooser cards. */
  body: string;
  badge: string;
  tint: string;
  featured?: boolean;
  /** Chooser navigation target. */
  to: string;
  search?: Record<string, string>;
};

export const CONNECTION_METHODS: ConnectionMethodDef[] = [
  {
    id: "hub",
    title: "Magic Hub",
    body: "Cloud Remote for Starlink / CGNAT. After Add router a paste-script window opens.",
    badge: "Cloud Remote",
    tint: "radial-gradient(circle at 30% 30%, #fbbf24, #f97316 60%, transparent)",
    featured: true,
    to: "/app/routers",
    search: { method: "hub" },
  },
  {
    id: "connector",
    title: "Local Connector",
    body: "A paired agent on the site reaches the router's LAN IP. Works behind CGNAT (Starlink, mobile ISPs) with no port forwarding.",
    badge: "LAN bridge",
    tint: "radial-gradient(circle at 30% 30%, #34d399, #0ea5e9 60%, transparent)",
    to: "/app/connectors",
  },
];

export function connectionMethodsForRole(_isStaff: boolean): ConnectionMethodDef[] {
  return CONNECTION_METHODS;
}

/** Primary café owner or platform Developer — used for other staff-only surfaces. */
export function isStaffRoles(roles: string[] | undefined | null, isPlatformAdmin = false): boolean {
  return isPrivilegedAccount(roles, isPlatformAdmin);
}
