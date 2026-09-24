// Gateway Bootstrap domain — pure, browser-safe types shared by the planner,
// the server engine and the UI. No transport, no secrets, no side effects.
//
// Gateway Bootstrap is the optional layer *before* Hotspot Wi-Fi: for a router
// that has internet on a WAN port but no guest bridge/IP/DHCP/NAT foundation
// yet. It only ever ADDS tagged MikroTik Magic objects; existing operator
// configuration is read for validation and never modified or removed.

import type {
  Capabilities,
  CommandResult,
  OsVersion,
  Severity,
  SnapshotRule,
  StepAction,
} from "../provisioning/types";

export type { Capabilities, CommandResult, OsVersion, Severity, SnapshotRule, StepAction };

export type BootstrapStrategy = "existing-bridge" | "new-bridge" | "vlan";

/**
 * One gateway bootstrap as the operator declares it. Every field is an
 * explicit choice — nothing about the WAN (PPPoE/static/DHCP) is guessed,
 * created, or replaced.
 */
export type GatewayBootstrapIntent = {
  kind: "gateway-bootstrap";
  routerId: string;
  /** Interface that already reaches the internet (operator-selected). */
  wanInterface: string;
  strategy: BootstrapStrategy;
  /** Existing bridge to reuse, or the name for the bridge to create. */
  bridge: string;
  /** VLAN id (1–4094) when strategy is "vlan"; the VLAN rides on `bridge`. */
  vlanId?: number;
  /** Guest gateway address with prefix, e.g. "10.5.50.1/24". */
  gatewayCidr: string;
  /** DHCP pool handed to guests, e.g. "10.5.50.10-10.5.50.254". */
  dhcpRange: string;
  /** Optional LAN ports to join into the guest bridge (for external APs). */
  guestPorts: string[];
  /** DNS servers handed to guests; empty = the gateway itself. */
  dnsServers: string[];
};

// --------------------------------------------------------------------------
// Discovery (read-only inspect)
// --------------------------------------------------------------------------

export type BootstrapInterface = { name: string; type: string; running: boolean };
export type BootstrapBridgePort = { bridge: string; iface: string; dynamic: boolean };
export type BootstrapVlan = { name: string; vlanId: number; iface: string; comment?: string };
export type BootstrapAddress = {
  iface: string;
  address: string;
  dynamic: boolean;
  comment?: string;
};
export type BootstrapRoute = { dst: string; gateway: string; dynamic: boolean };
export type BootstrapDhcpClient = { iface: string };
export type BootstrapDhcpServer = {
  name: string;
  iface: string;
  disabled: boolean;
  comment?: string;
};
export type BootstrapDhcpNetwork = { address: string; gateway?: string; comment?: string };
export type BootstrapPool = { name: string; ranges: string; comment?: string };
export type BootstrapNatRule = {
  chain: string;
  action: string;
  srcAddress?: string;
  outInterface?: string;
  comment?: string;
  disabled: boolean;
};

export type BootstrapSnapshot = {
  routerId: string;
  identity: string;
  boardName: string;
  version: OsVersion;
  capabilities: Capabilities;
  interfaces: BootstrapInterface[];
  bridges: string[];
  bridgePorts: BootstrapBridgePort[];
  vlans: BootstrapVlan[];
  addresses: BootstrapAddress[];
  routes: BootstrapRoute[];
  dhcpClients: BootstrapDhcpClient[];
  dhcpServers: BootstrapDhcpServer[];
  dhcpNetworks: BootstrapDhcpNetwork[];
  pools: BootstrapPool[];
  /** srcnat rules only — enough to detect existing masquerade coverage. */
  natRules: BootstrapNatRule[];
  /** Tagged Magic objects across the sections bootstrap manages. */
  rules: SnapshotRule[];
  /** RouterOS paths that could not be read during discovery. Never plan from an incomplete view. */
  discoveryFailures?: string[];
  sandbox: boolean;
  takenAt: string;
};

// --------------------------------------------------------------------------
// Plan / preflight
// --------------------------------------------------------------------------

export type BootstrapFinding = {
  id: string;
  severity: Severity;
  title: string;
  detail: string;
  remedy?: string;
};

export type BootstrapPlanStep = {
  id: string;
  action: StepAction;
  section: string;
  tag: string;
  title: string;
  /** RouterOS command rendered for this device. */
  command: string;
  reason: string;
};

export type GatewayBootstrapPlan = {
  intentKind: "gateway-bootstrap";
  routerId: string;
  /** Hash of the whole intent — identifies the run for audit and rollback. */
  intentHash: string;
  steps: BootstrapPlanStep[];
  summary: Record<StepAction, number>;
  findings: BootstrapFinding[];
  /** true when the guest gateway already fully exists — nothing would change. */
  noop: boolean;
  blocked: boolean;
  snapshotTakenAt: string;
  sandbox: boolean;
};

// --------------------------------------------------------------------------
// Verification / outcome
// --------------------------------------------------------------------------

export type BootstrapVerification = {
  /** REST stayed reachable after the apply (re-discover succeeded). */
  reachable: boolean;
  gatewayIpPresent: boolean;
  dhcpServerActive: boolean;
  defaultRoutePresent: boolean;
  natRulePresent: boolean;
  /** Tags from this run that did not land on the device. */
  missingTags: string[];
  ok: boolean;
};

export type BootstrapApplyOutcome = {
  ok: boolean;
  intentHash: string;
  backup: { id: string; name: string } | null;
  executed: CommandResult[];
  verification: BootstrapVerification | null;
  /** true when the run's own tagged objects were removed again after failure. */
  rolledBack: boolean;
  rollbackRemoved: number;
  error?: string;
  sandbox: boolean;
};

export type BootstrapRollbackOutcome = {
  ok: boolean;
  /** Number of run tags processed (each removes its tagged objects). */
  removed: number;
  error?: string;
};

// --------------------------------------------------------------------------
// Transport boundary
// --------------------------------------------------------------------------

/**
 * Live applies use RouterOS REST (via Local Connector / Magic Hub / direct),
 * same connection loading as every other router feature. The in-memory
 * sandbox exists for unit tests only.
 */
export interface BootstrapTransport {
  readonly name: "routeros-rest" | "sandbox";
  discover(routerId: string): Promise<BootstrapSnapshot>;
  /** Save a full restorable RouterOS backup before any write. */
  backup(routerId: string, label: string): Promise<{ id: string; name: string }>;
  run(routerId: string, commands: string[]): Promise<CommandResult[]>;
  /** Re-read state after apply and check the intent landed. */
  verify(
    routerId: string,
    intent: GatewayBootstrapIntent,
    expectedTags: string[],
  ): Promise<BootstrapVerification>;
  /** Remove ONLY objects carrying one of the given run tags. */
  rollback(routerId: string, tags: string[]): Promise<BootstrapRollbackOutcome>;
}
