// Gateway Bootstrap planner — pure. Given an intent + a discovered snapshot it
// returns a plan: tagged ADD steps only, plus preflight findings. No I/O, so it
// is fully unit-testable and identical on server and client.
//
// Hard rules encoded here:
//  - never guess, create, or replace WAN (PPPoE/static/DHCP) settings;
//  - never remove or modify operator-owned objects (no remove/modify steps);
//  - reject WAN/guest subnet overlap, address conflicts, invalid CIDRs,
//    WAN-member bridges, and guest ports already owned by another bridge.

import { makeTag, parseTag, tagsForIntent } from "../provisioning/tags";
import type { StepAction } from "../provisioning/types";
import {
  containsIp,
  hostOf,
  networkCidr,
  networksOverlap,
  parseIpv4,
  parseNetwork,
  parseRange,
} from "./cidr";
import type {
  BootstrapFinding,
  BootstrapPlanStep,
  BootstrapSnapshot,
  BootstrapVerification,
  GatewayBootstrapIntent,
  GatewayBootstrapPlan,
} from "./types";

export const INTENT = "gateway-bootstrap";
export const POOL_NAME = "mm-gw-pool";
export const DHCP_SERVER_NAME = "mm-gw-dhcp";

/** Typed confirmation required before any write. */
export const APPLY_PHRASE = "APPLY";
export function confirmApply(typed: string): boolean {
  return (typed ?? "").trim() === APPLY_PHRASE;
}

export function vlanIfaceName(vlanId: number): string {
  return `mm-gw-vlan${vlanId}`;
}

/** Interface the guest gateway IP / DHCP server live on. */
export function guestInterface(intent: GatewayBootstrapIntent): string {
  return intent.strategy === "vlan" ? vlanIfaceName(intent.vlanId ?? 0) : intent.bridge.trim();
}

/** Per-bridge foundation summary for the Inspect UI. */
export function bridgeFoundations(snapshot: BootstrapSnapshot): Array<{
  name: string;
  gatewayIp: string | null;
  hasDhcp: boolean;
  hasWanMember: boolean;
  suitable: boolean;
}> {
  return snapshot.bridges.map((name) => {
    const addr = snapshot.addresses.find((a) => a.iface === name);
    const hasDhcp = snapshot.dhcpServers.some((d) => d.iface === name && !d.disabled);
    const hasWanMember = snapshot.bridgePorts.some(
      (p) => p.bridge === name && isWanLike(p.iface, snapshot),
    );
    return {
      name,
      gatewayIp: addr?.address ?? null,
      hasDhcp,
      hasWanMember,
      suitable: !hasWanMember,
    };
  });
}

function isWanLike(iface: string, snapshot: BootstrapSnapshot): boolean {
  return (
    snapshot.addresses.some((a) => a.iface === iface) ||
    snapshot.dhcpClients.some((d) => d.iface === iface)
  );
}

/** True when an enabled srcnat masquerade already covers the guest subnet. */
function natCovered(intent: GatewayBootstrapIntent, snapshot: BootstrapSnapshot): boolean {
  const guest = parseNetwork(intent.gatewayCidr);
  if (!guest) return false;
  return snapshot.natRules.some((r) => {
    if (r.disabled || r.chain !== "srcnat" || r.action !== "masquerade") return false;
    if (r.outInterface && r.outInterface !== intent.wanInterface) return false;
    if (!r.srcAddress) return true; // masquerade everything
    const src = parseNetwork(r.srcAddress);
    return !!src && src.network <= guest.network && src.broadcast >= guest.broadcast;
  });
}

// --------------------------------------------------------------------------
// Desired steps
// --------------------------------------------------------------------------

type DesiredStep = {
  section: string;
  title: string;
  payload: unknown;
  render: (tag: string) => string;
  /** Pre-resolved skip when an equivalent object already exists unmanaged. */
  skipReason?: string;
};

function desiredSteps(intent: GatewayBootstrapIntent, snapshot: BootstrapSnapshot): DesiredStep[] {
  const out: DesiredStep[] = [];
  const bridge = intent.bridge.trim();
  const guestIface = guestInterface(intent);
  const subnet = networkCidr(intent.gatewayCidr) ?? intent.gatewayCidr.trim();
  const gwIp = hostOf(intent.gatewayCidr) ?? "";
  const dns = intent.dnsServers.length > 0 ? intent.dnsServers.join(",") : gwIp;

  if (intent.strategy === "new-bridge") {
    out.push({
      section: "/interface/bridge",
      title: `Create guest bridge "${bridge}"`,
      payload: { r: "bridge", bridge },
      render: (tag) => `/interface bridge add name="${bridge}" comment="${tag}"`,
    });
  }

  if (intent.strategy === "vlan") {
    const name = vlanIfaceName(intent.vlanId ?? 0);
    out.push({
      section: "/interface/vlan",
      title: `Guest VLAN ${intent.vlanId} on ${bridge}`,
      payload: { r: "vlan", name, vlanId: intent.vlanId, bridge },
      render: (tag) =>
        `/interface vlan add name="${name}" vlan-id=${intent.vlanId} interface="${bridge}" comment="${tag}"`,
    });
  }

  for (const port of intent.guestPorts) {
    out.push({
      section: "/interface/bridge/port",
      title: `Join ${port} into ${bridge}`,
      payload: { r: "bridge-port", bridge, port },
      render: (tag) =>
        `/interface bridge port add bridge="${bridge}" interface="${port}" comment="${tag}"`,
      skipReason: snapshot.bridgePorts.some((p) => p.bridge === bridge && p.iface === port)
        ? "Port is already a member of this bridge."
        : undefined,
    });
  }

  out.push({
    section: "/ip/address",
    title: `Guest gateway ${intent.gatewayCidr} on ${guestIface}`,
    payload: { r: "address", iface: guestIface, cidr: intent.gatewayCidr.trim() },
    render: (tag) =>
      `/ip address add address=${intent.gatewayCidr.trim()} interface="${guestIface}" comment="${tag}"`,
    skipReason: snapshot.addresses.some(
      (a) => a.iface === guestIface && a.address.trim() === intent.gatewayCidr.trim(),
    )
      ? "Gateway address already exists on this interface — left untouched."
      : undefined,
  });

  const existingDhcp = snapshot.dhcpServers.find((d) => d.iface === guestIface && !d.disabled);
  const dhcpSkip = existingDhcp
    ? `DHCP server "${existingDhcp.name}" already serves ${guestIface} — left untouched.`
    : undefined;

  out.push({
    section: "/ip/pool",
    title: `DHCP pool ${intent.dhcpRange}`,
    payload: { r: "pool", name: POOL_NAME, ranges: intent.dhcpRange.trim() },
    render: (tag) =>
      `/ip pool add name=${POOL_NAME} ranges=${intent.dhcpRange.trim()} comment="${tag}"`,
    skipReason:
      dhcpSkip ??
      (snapshot.pools.some(
        (p) => p.name === POOL_NAME && p.ranges.trim() === intent.dhcpRange.trim(),
      )
        ? "Identical pool already exists."
        : undefined),
  });

  out.push({
    section: "/ip/dhcp-server/network",
    title: `DHCP network ${subnet} (gateway ${gwIp})`,
    payload: { r: "dhcp-network", address: subnet, gateway: gwIp, dns },
    render: (tag) =>
      `/ip dhcp-server network add address=${subnet} gateway=${gwIp} dns-server=${dns} comment="${tag}"`,
    skipReason:
      dhcpSkip ??
      (snapshot.dhcpNetworks.some((n) => n.address.trim() === subnet)
        ? "A DHCP network for this subnet already exists — left untouched."
        : undefined),
  });

  out.push({
    section: "/ip/dhcp-server",
    title: `DHCP server on ${guestIface}`,
    payload: { r: "dhcp-server", name: DHCP_SERVER_NAME, iface: guestIface, pool: POOL_NAME },
    render: (tag) =>
      `/ip dhcp-server add name=${DHCP_SERVER_NAME} interface="${guestIface}" address-pool=${POOL_NAME} disabled=no comment="${tag}"`,
    skipReason: dhcpSkip,
  });

  out.push({
    section: "/ip/firewall/nat",
    title: `Masquerade guest subnet out ${intent.wanInterface}`,
    payload: { r: "nat", src: subnet, out: intent.wanInterface },
    render: (tag) =>
      `/ip firewall nat add chain=srcnat src-address=${subnet} out-interface="${intent.wanInterface}" action=masquerade comment="${tag}"`,
    skipReason: natCovered(intent, snapshot)
      ? "An existing masquerade rule already covers the guest subnet — left untouched."
      : undefined,
  });

  // Narrowly scoped guest isolation: guests may talk inside their own subnet
  // and out to the internet, but not into other private LANs. Appended rules —
  // the limitation is surfaced as an info finding.
  const isolation: Array<{ dst: string; action: "accept" | "drop"; title: string }> = [
    { dst: subnet, action: "accept", title: "Allow guests inside their own subnet" },
    { dst: "10.0.0.0/8", action: "drop", title: "Block guests from 10.0.0.0/8 LANs" },
    { dst: "172.16.0.0/12", action: "drop", title: "Block guests from 172.16.0.0/12 LANs" },
    { dst: "192.168.0.0/16", action: "drop", title: "Block guests from 192.168.0.0/16 LANs" },
  ];
  for (const rule of isolation) {
    out.push({
      section: "/ip/firewall/filter",
      title: rule.title,
      payload: { r: "isolation", src: subnet, dst: rule.dst, action: rule.action },
      render: (tag) =>
        `/ip firewall filter add chain=forward src-address=${subnet} dst-address=${rule.dst} action=${rule.action} comment="${tag}"`,
    });
  }

  return out;
}

// --------------------------------------------------------------------------
// Plan
// --------------------------------------------------------------------------

export function planGatewayBootstrap(
  intent: GatewayBootstrapIntent,
  snapshot: BootstrapSnapshot,
): GatewayBootstrapPlan {
  const desired = desiredSteps(intent, snapshot);
  const existing = snapshot.rules
    .map((r) => ({ rule: r, tag: parseTag(r.comment) }))
    .filter((x) => x.tag?.intent === INTENT);

  const steps: BootstrapPlanStep[] = [];
  const keptTags = new Set<string>();

  for (const d of desired) {
    const tag = makeTag(INTENT, d.payload);
    keptTags.add(tag);
    const managed = existing.some(
      (e) => e.rule.section === d.section && `mmagic:${INTENT}:${e.tag!.hash}` === tag,
    );
    const action: StepAction = managed || d.skipReason ? "skip" : "add";
    steps.push({
      id: `${d.section}:${tag}`,
      action,
      section: d.section,
      tag,
      title: d.title,
      command: d.render(tag),
      reason: managed
        ? "Identical Magic object with this tag already exists — nothing to do."
        : (d.skipReason ?? "Object does not exist yet and is created, tagged as MikroTik Magic."),
    });
  }

  const findings = preflightGatewayBootstrap(intent, snapshot, keptTags);
  const summary: Record<StepAction, number> = { add: 0, modify: 0, skip: 0, remove: 0 };
  for (const s of steps) summary[s.action]++;

  return {
    intentKind: INTENT,
    routerId: intent.routerId,
    intentHash: makeTag(INTENT, intent).split(":")[2]!,
    steps,
    summary,
    findings,
    noop: summary.add === 0,
    blocked: findings.some((f) => f.severity === "blocker"),
    snapshotTakenAt: snapshot.takenAt,
    sandbox: snapshot.sandbox,
  };
}

// --------------------------------------------------------------------------
// Preflight
// --------------------------------------------------------------------------

export function preflightGatewayBootstrap(
  intent: GatewayBootstrapIntent,
  snapshot: BootstrapSnapshot,
  plannedTags?: Set<string>,
): BootstrapFinding[] {
  const f: BootstrapFinding[] = [];
  const push = (
    id: string,
    severity: BootstrapFinding["severity"],
    title: string,
    detail: string,
    remedy?: string,
  ) => f.push({ id, severity, title, detail, remedy });

  const bridge = intent.bridge.trim();
  const guestIface = guestInterface(intent);
  const known = new Set(snapshot.interfaces.map((i) => i.name));

  if (snapshot.discoveryFailures?.length) {
    push(
      "discovery-incomplete",
      "blocker",
      "Router discovery is incomplete",
      `MikroMagic could not read: ${snapshot.discoveryFailures.join(", ")}. No gateway plan can be built from an incomplete router view.`,
      "Give the RouterOS account used by Magic Hub read access, then run Inspect again.",
    );
  }

  if (!snapshot.capabilities.rest) {
    push(
      "os-too-old",
      "blocker",
      `RouterOS ${snapshot.version.raw || "unknown"} cannot be provisioned remotely`,
      "Staged apply uses the RouterOS REST API, which exists from RouterOS 7.1. This router reports an older release.",
      "Upgrade the router, or copy the planned commands into a terminal session manually.",
    );
  }

  // --- WAN --------------------------------------------------------------
  if (known.size > 0 && !known.has(intent.wanInterface)) {
    push(
      "wan-missing",
      "blocker",
      `WAN interface ${intent.wanInterface} not found`,
      "The selected WAN interface does not exist on this router. Pick one from the discovered list.",
    );
  }
  if (guestIface === intent.wanInterface || bridge === intent.wanInterface) {
    push(
      "wan-is-guest",
      "blocker",
      "WAN and guest interface are the same",
      "The guest network must live on its own bridge/VLAN, never on the internet-facing interface.",
    );
  }
  const wanHasAddress = snapshot.addresses.some((a) => a.iface === intent.wanInterface);
  const wanHasDhcp = snapshot.dhcpClients.some((d) => d.iface === intent.wanInterface);
  if (!wanHasAddress && !wanHasDhcp) {
    push(
      "wan-no-addressing",
      "warning",
      "No WAN addressing detected",
      `${intent.wanInterface} has no IP address and no DHCP client. Bootstrap never configures the WAN — make sure your ISP connection (DHCP, static, or PPPoE) already works.`,
    );
  }
  if (!snapshot.routes.some((r) => r.dst === "0.0.0.0/0")) {
    push(
      "no-default-route",
      "warning",
      "No default route",
      "Without a 0.0.0.0/0 route guests will get Wi-Fi but no internet. Bootstrap does not create WAN routes.",
    );
  }

  // --- Bridge / VLAN ------------------------------------------------------
  if (intent.strategy === "existing-bridge" && !snapshot.bridges.includes(bridge)) {
    push(
      "bridge-missing",
      "blocker",
      `Bridge "${bridge}" not found`,
      "Strategy 'existing bridge' reuses a bridge that is already on the router.",
      "Pick a discovered bridge or switch to the new-bridge strategy.",
    );
  }
  if (intent.strategy === "new-bridge" && snapshot.bridges.includes(bridge)) {
    push(
      "bridge-exists",
      "blocker",
      `Bridge "${bridge}" already exists`,
      "Refusing to silently reuse a bridge you may own. Choose 'existing bridge' if it really is the guest bridge.",
    );
  }
  if (snapshot.bridgePorts.some((p) => p.bridge === bridge && p.iface === intent.wanInterface)) {
    push(
      "bridge-is-wan",
      "blocker",
      `"${bridge}" carries the WAN port`,
      `${intent.wanInterface} is a member port of ${bridge}. A guest bridge must never contain the internet-facing interface.`,
      "Create a new dedicated guest bridge instead.",
    );
  }
  if (intent.strategy === "vlan") {
    if (!intent.vlanId || intent.vlanId < 1 || intent.vlanId > 4094) {
      push("vlan-id-invalid", "blocker", "VLAN id must be 1–4094", "Set a valid 802.1Q VLAN id.");
    } else {
      const name = vlanIfaceName(intent.vlanId);
      const taken = snapshot.vlans.find((v) => v.name === name);
      if (taken && parseTag(taken.comment)?.intent !== INTENT) {
        push(
          "vlan-name-taken",
          "blocker",
          `VLAN name ${name} is already used`,
          "A VLAN interface with the generated name exists and is not managed by MikroTik Magic.",
          "Pick a different VLAN id.",
        );
      }
      if (!snapshot.bridges.includes(bridge)) {
        push(
          "vlan-bridge-missing",
          "blocker",
          `Parent bridge "${bridge}" not found`,
          "The guest VLAN rides on an existing bridge (VLAN filtering). Create the bridge first or choose another strategy.",
        );
      }
    }
  }

  // --- CIDR / range -------------------------------------------------------
  const guest = parseNetwork(intent.gatewayCidr);
  const gwIp = hostOf(intent.gatewayCidr);
  if (!guest || !gwIp) {
    push(
      "invalid-cidr",
      "blocker",
      "Invalid guest gateway CIDR",
      `"${intent.gatewayCidr}" is not a usable IPv4 address/prefix between /16 and /30 (e.g. 10.5.50.1/24).`,
    );
  } else {
    const range = parseRange(intent.dhcpRange);
    if (!range) {
      push(
        "invalid-range",
        "blocker",
        "Invalid DHCP range",
        `"${intent.dhcpRange}" should look like 10.5.50.10-10.5.50.254.`,
      );
    } else {
      if (!containsIp(guest, range.start) || !containsIp(guest, range.end)) {
        push(
          "range-outside-subnet",
          "blocker",
          "DHCP range leaves the guest subnet",
          `The pool must stay inside ${networkCidr(intent.gatewayCidr)}.`,
        );
      }
      const gw = parseIpv4(gwIp)!;
      if (gw >= range.start && gw <= range.end) {
        push(
          "range-includes-gateway",
          "blocker",
          "DHCP range includes the gateway address",
          `Exclude ${gwIp} from the pool, e.g. start it one address later.`,
        );
      }
      if (range.start <= guest.network || range.end >= guest.broadcast) {
        push(
          "range-touches-edges",
          "blocker",
          "DHCP range touches network/broadcast",
          "The pool must not contain the network or broadcast address.",
        );
      }
    }

    // WAN/guest overlap and existing-address conflicts.
    for (const a of snapshot.addresses) {
      const other = parseNetwork(a.address);
      if (!other || !networksOverlap(guest, other)) continue;
      const sameManagedAddr =
        a.iface === guestIface && a.address.trim() === intent.gatewayCidr.trim();
      if (sameManagedAddr) continue; // the no-op / skip case
      push(
        a.iface === intent.wanInterface ? "wan-guest-overlap" : "address-conflict",
        "blocker",
        a.iface === intent.wanInterface
          ? "Guest subnet overlaps the WAN"
          : `Address conflict on ${a.iface}`,
        `${a.address} on ${a.iface} overlaps the guest subnet ${networkCidr(intent.gatewayCidr)}.`,
        "Choose a different guest subnet (e.g. another private range).",
      );
    }
  }

  // --- Guest ports ----------------------------------------------------------
  for (const port of intent.guestPorts) {
    if (port === intent.wanInterface) {
      push(
        "port-is-wan",
        "blocker",
        `${port} is the WAN interface`,
        "The internet-facing port can never be a guest bridge member.",
      );
      continue;
    }
    if (known.size > 0 && !known.has(port)) {
      push("port-missing", "blocker", `Port ${port} not found`, "Pick from the discovered list.");
      continue;
    }
    const member = snapshot.bridgePorts.find((p) => p.iface === port && p.bridge !== bridge);
    if (member) {
      push(
        "port-bridged-elsewhere",
        "blocker",
        `${port} already belongs to ${member.bridge}`,
        "Moving a port between bridges is a destructive change Magic will not guess at.",
        "Remove it from the other bridge yourself first, or leave it unchecked.",
      );
    }
  }

  // --- Name collisions with operator-owned objects ---------------------------
  const pool = snapshot.pools.find((p) => p.name === POOL_NAME);
  if (
    pool &&
    parseTag(pool.comment)?.intent !== INTENT &&
    pool.ranges.trim() !== intent.dhcpRange.trim()
  ) {
    push(
      "pool-name-taken",
      "blocker",
      `Pool name ${POOL_NAME} already used`,
      "Your own DHCP pool uses the generated name with different ranges.",
    );
  }
  const server = snapshot.dhcpServers.find((d) => d.name === DHCP_SERVER_NAME);
  if (server && parseTag(server.comment)?.intent !== INTENT && server.iface !== guestIface) {
    push(
      "dhcp-name-taken",
      "blocker",
      `DHCP server name ${DHCP_SERVER_NAME} already used`,
      "A DHCP server with the generated name exists on another interface.",
    );
  }

  // --- Objects from an earlier run with different settings -------------------
  if (plannedTags) {
    const stale = tagsForIntent(
      snapshot.rules.map((r) => r.comment),
      INTENT,
    ).filter((t) => !plannedTags.has(t));
    if (stale.length > 0) {
      push(
        "stale-run",
        "blocker",
        "Objects from an earlier bootstrap run exist",
        `${stale.length} tagged object(s) use different settings. Applying on top would leave two gateways/DHCP stacks.`,
        "Run Rollback for the previous apply first, then plan again.",
      );
    }
  }

  // --- Honest limitations ------------------------------------------------------
  push(
    "isolation-append",
    "info",
    "Guest isolation rules are appended",
    "The drop rules are added at the end of the forward chain. If your own chain has an early accept-all rule, move the Magic rules above it in Winbox.",
  );

  return f;
}

// --------------------------------------------------------------------------
// Post-apply verification (shared by both transports)
// --------------------------------------------------------------------------

export function computeVerification(
  snapshot: BootstrapSnapshot,
  intent: GatewayBootstrapIntent,
  expectedTags: string[],
): BootstrapVerification {
  const guestIface = guestInterface(intent);
  const presentTags = new Set(
    snapshot.rules
      .map((r) => parseTag(r.comment))
      .filter((t): t is NonNullable<typeof t> => !!t)
      .map((t) => `mmagic:${t.intent}:${t.hash}`),
  );
  const missingTags = expectedTags.filter((t) => !presentTags.has(t));

  const gatewayIpPresent = snapshot.addresses.some(
    (a) => a.iface === guestIface && a.address.trim() === intent.gatewayCidr.trim(),
  );
  const dhcpServerActive = snapshot.dhcpServers.some((d) => d.iface === guestIface && !d.disabled);
  const defaultRoutePresent = snapshot.routes.some((r) => r.dst === "0.0.0.0/0");
  const natRulePresent = natCovered(intent, snapshot);

  const checks = [gatewayIpPresent, dhcpServerActive, defaultRoutePresent, natRulePresent];
  return {
    reachable: true, // a completed discover is the reachability proof
    gatewayIpPresent,
    dhcpServerActive,
    defaultRoutePresent,
    natRulePresent,
    missingTags,
    ok: missingTags.length === 0 && checks.every(Boolean),
  };
}
