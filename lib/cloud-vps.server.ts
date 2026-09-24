// Server-only client for the VPS "Router Manager" API.
//
// Architecture:
//   MikroTik --WireGuard--> VPS (wg server + Router Manager API)
//     --HTTPS--> this app's server functions --> browser
//
// REST never goes browser→VPS: every call is a server function that has
// already authenticated the user and verified the router belongs to them.
// WebFig is the exception — it is a full RouterOS UI, so the browser opens
// hub /peers/{id}/open-webfig (unsigned, same origin as the REST data plane).
// Signing secrets still never leave the backend.
//
// Transport: this module shares ONE signed transport with the WireGuard peer
// engine (src/lib/wireguard/vps.server.ts). There is no second credential set
// and no bearer-token path — every call is HMAC-signed, short-lived, scoped to
// a tenant + router and idempotent.

import { readVpsConfig, isVpsConfigured, hubPublicOrigin, vpsCall } from "./wireguard/vps.server";
import { hubWebfigLaunchUrl } from "./webfig";
import { routerosClockPrepSnippet } from "./router-clock";
import { cloudPeerStatusFromHandshake } from "./cloud-peer-status";

export type CloudPeer = {
  peerId: string;
  address: string; // WireGuard tunnel IP, e.g. 10.90.0.7
  publicKey: string; // router-side public key
  privateKey: string; // returned ONCE at provisioning, never stored in plaintext
  serverPublicKey: string;
  endpoint: string; // host:port of the VPS WireGuard listener
  allowedIps: string;
  dns?: string | null;
};

export type CloudPeerStatus = {
  status: "online" | "offline" | "connecting" | "error";
  lastHandshakeAt: string | null;
  lastSeenAt: string | null;
  rxBytes?: number | null;
  txBytes?: number | null;
  error?: string | null;
};

/** Tenant + router the call is made on behalf of. Never browser-supplied. */
export type CloudScope = { tenantId: string; routerId: string };

export function isCloudConfigured(): boolean {
  return isVpsConfigured();
}

export async function vpsRequest<T = unknown>(
  scope: CloudScope,
  path: string,
  init?: {
    method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
    scopeName?: import("./wireguard/vps.server").HubScope;
    requestedBy?: string;
    body?: unknown;
    timeoutMs?: number;
  },
): Promise<T> {
  const method = init?.method ?? "GET";
  const requestedBy = init?.requestedBy ?? scope.tenantId;
  const hubScope =
    init?.scopeName ??
    (method === "GET" ? "peers:inspect" : method === "DELETE" ? "peers:remove" : "peers:inspect");
  const res = await vpsCall<T>({
    method,
    path,
    scope: hubScope,
    tenantId: scope.tenantId,
    routerId: scope.routerId,
    requestedBy,
    ...(init?.body === undefined ? {} : { body: init.body }),
    ...(init?.timeoutMs === undefined ? {} : { timeoutMs: init.timeoutMs }),
  });
  if (!res.ok) throw new Error(res.error);
  return res.data;
}

/** Live peer handshake/status from the hub control plane. */
export async function peerStatus(scope: CloudScope, peerId: string): Promise<CloudPeerStatus> {
  const raw = await vpsRequest<{
    state?: string;
    lastHandshakeAt?: string | null;
    rxBytes?: number | null;
    txBytes?: number | null;
  }>(scope, `/v1/peers/${encodeURIComponent(peerId)}`, {
    method: "GET",
    scopeName: "peers:inspect",
    requestedBy: scope.tenantId,
  });
  const state = String(raw.state ?? "");
  const status = cloudPeerStatusFromHandshake(state, raw.lastHandshakeAt);
  return {
    status,
    lastHandshakeAt: raw.lastHandshakeAt ?? null,
    lastSeenAt: raw.lastHandshakeAt ?? null,
    rxBytes: raw.rxBytes ?? null,
    txBytes: raw.txBytes ?? null,
  };
}

/**
 * REST base the app uses for a cloud router. Data plane lives on the hub
 * origin at /peers/{id}/rest (unsigned Basic auth to RouterOS), not under
 * /internal/provisioner.
 */
export function cloudRestBase(peerId: string): string {
  return `${hubPublicOrigin()}/peers/${encodeURIComponent(peerId)}/rest`;
}

/** Browser launch URL for WebFig through the hub (cookie + /webfig/ bounce). */
export function cloudWebfigLaunchUrl(peerId: string): string {
  return hubWebfigLaunchUrl(hubPublicOrigin(), peerId);
}

/* ------------------------------------------------------------------ *
 * Controlled operations
 * ------------------------------------------------------------------ *
 * Only these operations can be triggered from the UI. Arbitrary RouterOS
 * paths are never accepted from the frontend (owners get a separate,
 * explicitly privileged escape hatch in cloud-router.functions.ts).
 */

export type CloudOp =
  | "router_status"
  | "system_resource"
  | "interfaces"
  | "hotspot_active"
  | "hotspot_users"
  | "hotspot_user_create"
  | "hotspot_user_enable"
  | "hotspot_user_disable"
  | "hotspot_user_remove"
  | "hotspot_kick";

type OpSpec = {
  method: "GET" | "POST" | "DELETE" | "PATCH";
  path: (p: Record<string, string>) => string;
  body?: (p: Record<string, string>) => unknown;
  /** Params required from the caller (validated before dispatch). */
  requires?: string[];
  mutating?: boolean;
};

export const CLOUD_OPS: Record<CloudOp, OpSpec> = {
  router_status: { method: "GET", path: () => "/system/identity" },
  system_resource: { method: "GET", path: () => "/system/resource" },
  interfaces: { method: "GET", path: () => "/interface" },
  hotspot_active: { method: "GET", path: () => "/ip/hotspot/active" },
  hotspot_users: { method: "GET", path: () => "/ip/hotspot/user" },
  hotspot_user_create: {
    method: "POST",
    path: () => "/ip/hotspot/user",
    requires: ["name", "password"],
    body: (p) => ({
      name: p["name"],
      password: p["password"],
      ...(p["profile"] ? { profile: p["profile"] } : {}),
      ...(p["limitUptime"] ? { "limit-uptime": p["limitUptime"] } : {}),
      ...(p["comment"] ? { comment: p["comment"] } : {}),
    }),
    mutating: true,
  },
  hotspot_user_enable: {
    method: "PATCH",
    path: (p) => `/ip/hotspot/user/${encodeURIComponent(p["id"] ?? "")}`,
    requires: ["id"],
    body: () => ({ disabled: "false" }),
    mutating: true,
  },
  hotspot_user_disable: {
    method: "PATCH",
    path: (p) => `/ip/hotspot/user/${encodeURIComponent(p["id"] ?? "")}`,
    requires: ["id"],
    body: () => ({ disabled: "true" }),
    mutating: true,
  },
  hotspot_user_remove: {
    method: "DELETE",
    path: (p) => `/ip/hotspot/user/${encodeURIComponent(p["id"] ?? "")}`,
    requires: ["id"],
    mutating: true,
  },
  hotspot_kick: {
    method: "DELETE",
    path: (p) => `/ip/hotspot/active/${encodeURIComponent(p["id"] ?? "")}`,
    requires: ["id"],
    mutating: true,
  },
};

/** Run one whitelisted operation against a cloud router through the VPS. */
export async function runCloudOp(
  scope: CloudScope,
  peerId: string,
  op: CloudOp,
  params: Record<string, string> = {},
): Promise<unknown> {
  const spec = CLOUD_OPS[op];
  if (!spec) throw new Error(`Unsupported operation: ${op}`);
  for (const key of spec.requires ?? []) {
    if (!params[key]) throw new Error(`Missing "${key}" for operation ${op}`);
  }
  return vpsRequest(scope, `/peers/${encodeURIComponent(peerId)}/rest${spec.path(params)}`, {
    method: spec.method,
    body: spec.body ? spec.body(params) : undefined,
  });
}

/** Split hub `host:port` without breaking IPv4 or stripping a scheme if the hub sent one. */
export function parseHubEndpoint(endpoint: string): { host: string; port: string } {
  let raw = (endpoint ?? "").trim();
  raw = raw.replace(/^(udp|tcp|https?):\/\//i, "");
  const cut = raw.search(/[/?#]/);
  if (cut !== -1) raw = raw.slice(0, cut);
  const v6 = /^\[([0-9a-f:]+)\]:(\d{1,5})$/i.exec(raw);
  if (v6) return { host: v6[1] ?? "", port: v6[2] ?? "51820" };
  const colon = raw.lastIndexOf(":");
  if (colon > 0 && /^\d{1,5}$/.test(raw.slice(colon + 1))) {
    const host = raw.slice(0, colon).trim();
    const port = raw.slice(colon + 1);
    if (host) return { host, port };
  }
  if (!raw) throw new Error("Magic Hub endpoint is missing.");
  return { host: raw, port: "51820" };
}

function rosComment(value: string): string {
  return value
    .replace(/[\r\n"]/g, " ")
    .trim()
    .slice(0, 80);
}

const IPV4 = /^(\d{1,3}\.){3}\d{1,3}$/;

/**
 * Prefer a stable IPv4 endpoint in the paste script.
 * Broken LAN DNS (hub hostname → private IP) was a common Magic Hub loop;
 * env override wins, then a known production map, else the hostname as-is.
 */
export function hubEndpointForScript(
  endpoint: string,
  env: NodeJS.ProcessEnv = process.env,
): { host: string; port: string; displayHost: string } {
  const parsed = parseHubEndpoint(endpoint);
  const displayHost = parsed.host;
  if (!parsed.host) throw new Error("Magic Hub endpoint is missing.");
  if (IPV4.test(parsed.host)) {
    return { host: parsed.host, port: parsed.port, displayHost };
  }
  const envIp = (env["VPS_ROUTER_HUB_PUBLIC_IP"] ?? "").trim();
  if (IPV4.test(envIp)) {
    return { host: envIp, port: parsed.port, displayHost };
  }
  // No hard-coded hub IP: the hub can move VPS. Set VPS_ROUTER_HUB_PUBLIC_IP to
  // pin an IPv4 in the paste script; otherwise the hostname is used as-is.
  return { host: parsed.host, port: parsed.port, displayHost };

}

/** RouterOS script the customer pastes to dial out to the VPS (Magic Hub / Cloud Remote). */
export function buildCloudRouterScript(opts: {
  routerName: string;
  peer: CloudPeer;
  restPort: number;
  env?: NodeJS.ProcessEnv;
}): string {
  const { peer } = opts;
  if (!peer.privateKey?.trim()) throw new Error("Magic Hub private key is missing.");
  if (!peer.serverPublicKey?.trim()) throw new Error("Magic Hub public key is missing.");
  const { host, port, displayHost } = hubEndpointForScript(peer.endpoint, opts.env ?? process.env);
  const ip = (peer.address.split("/")[0] ?? "").trim();
  if (!IPV4.test(ip)) throw new Error("Magic Hub tunnel address is invalid.");
  const restPort =
    Number.isFinite(opts.restPort) && opts.restPort > 0 ? Math.floor(opts.restPort) : 443;
  const allowed = (peer.allowedIps || "10.77.0.1/32").trim();
  const name = rosComment(opts.routerName) || "router";
  const endpointNote =
    host === displayHost
      ? `endpoint ${host}:${port}`
      : `endpoint ${host}:${port} (${displayHost} → IP; avoids broken LAN DNS)`;
  // Interface uses /24 (not the hub's peer /32) so REST through Starlink CGNAT works.
  // Remove first so a second paste does not fail with "already have this name".
  // WAN preflight gates install: no internet ⇒ no handshake ⇒ misleading app 502s.
  // Clock prep (NTP + Yangon) runs before TLS cert signing — factory boards often sit at 1970.
  const clockPrep = routerosClockPrepSnippet({ waitSeconds: 8 });
  return `# MikroTik Magic — Magic Hub (Cloud Remote)
# Router: ${name}
# ONE paste into WinBox New Terminal. Safe to paste more than once.
# Do not change endpoint-address or keys. Do not put this in the Scripts library.
#
# MUST work first (from THIS board's WinBox, not your PC):
#   /ping 8.8.8.8 count=3
# LAN and WAN must NOT share the same IP (common break: bridge + ether1 both .49).
# After success: app → Check now → Test. App password must match WinBox.
# Ignore ping to the hub or 10.77.0.1 — ICMP is often blocked. Use last-handshake.
# ${endpointNote}

:put "=== Magic Hub preflight (WAN) ==="
/ip/route print where dst-address=0.0.0.0/0
/ip/address print

:local stop false
:if ([:len [/ip/route find dst-address=0.0.0.0/0]] = 0) do={
  :put "STOP: no default route — board has no WAN path."
  :set stop true
}

:local dupAddr ""
:foreach i in=[/ip/address find] do={
  :local a [/ip/address get $i address]
  :local c 0
  :foreach j in=[/ip/address find] do={
    :if ([/ip/address get $j address] = $a) do={ :set c ($c + 1) }
  }
  :if ($c > 1) do={ :set dupAddr $a }
}
:if ([:len $dupAddr] > 0) do={
  :put ("STOP: same IP on more than one interface: " . $dupAddr)
  :put "Fix: remove the LAN copy (or give bridge a different subnet e.g. 192.168.88.1/24), keep ether1 DHCP, add masquerade out ether1."
  :set stop true
}

:local pingOk [/ping 8.8.8.8 count=3]
:put ("ping 8.8.8.8 replies=" . $pingOk)
:if ($pingOk < 1) do={
  :put "STOP: this board cannot reach the internet (ping from WinBox failed — not your Mac/PC)."
  :put "Fix WAN first, then paste this script again."
  :set stop true
}

:if ($stop) do={
  :put "Magic Hub NOT installed. Fix the STOP lines above, then paste again."
} else={

${clockPrep}
:do { /ip/service set [find name="reverse-proxy"] disabled=yes } on-error={}
:if ([:len [/certificate find name="magic-https"]] = 0) do={
  /certificate add name=magic-https common-name=magic-hub days-valid=3650
  /certificate sign magic-https
}
:local wwwCert [/ip/service get [find name="www-ssl"] certificate]
:if ($wwwCert = "" || $wwwCert = "none") do={
  /ip/service set [find name="www-ssl"] certificate=magic-https disabled=no port=${restPort}
} else={
  /ip/service set [find name="www-ssl"] disabled=no port=${restPort}
}
:do {
  # Magic Hub uses the router's full group by product policy. Keep its complete
  # RouterOS policy set valid so the stored app credential can inspect and manage
  # the board through REST after the tunnel comes up.
  /user/group set [find name="full"] policy=local,telnet,ssh,ftp,reboot,read,write,policy,test,winbox,password,web,sniff,sensitive,api,rest-api,romon
} on-error={}

/ip/firewall/mangle remove [find comment~"magic-cloud"]
/ip/firewall/filter remove [find comment~"magic-cloud"]
/ip/address remove [find interface="magic-cloud"]
/interface/wireguard/peers remove [find interface="magic-cloud"]
/interface/wireguard remove [find name="magic-cloud"]

/interface/wireguard
add name=magic-cloud listen-port=13231 mtu=1280 private-key="${peer.privateKey}" comment="MikroTik Magic Hub"

/interface/wireguard/peers
add interface=magic-cloud public-key="${peer.serverPublicKey}" endpoint-address="${host}" endpoint-port=${port} allowed-address=${allowed} persistent-keepalive=25s comment="MikroTik Magic Hub"

/ip/address
add address=${ip}/24 interface=magic-cloud comment="MikroTik Magic Hub"

/ip/firewall/mangle
add chain=output out-interface=magic-cloud protocol=tcp tcp-flags=syn action=change-mss new-mss=1160 passthrough=yes comment="magic-cloud mss out"

/ip/firewall/filter
add chain=input in-interface=magic-cloud protocol=tcp dst-port=${restPort} action=accept comment="magic-cloud: allow REST" place-before=0
add chain=input in-interface=magic-cloud protocol=icmp action=accept comment="magic-cloud: allow ping"

/interface/wireguard disable [find name="magic-cloud"]
:delay 2s
/interface/wireguard enable [find name="magic-cloud"]
:delay 12s
/interface/wireguard/peers print detail where interface=magic-cloud
/ip/address print where interface=magic-cloud
/ip/service print where name=www-ssl

:put "Magic Hub installed. Tunnel ${ip}/24."
:put "Good: last-handshake a few seconds ago. Empty/never = still no UDP path to ${host}:${port}."
:put "Ignore hub/10.77.0.1 ping timeouts. Next: app Check now → Test."

}
`;
}

export function buildCloudRollbackScript(): string {
  return `# MikroTik Magic — remove Magic Hub (Cloud Remote)
/ip/firewall/mangle remove [find comment~"magic-cloud"]
/ip/firewall/filter remove [find comment~"magic-cloud"]
/ip/address remove [find interface="magic-cloud"]
/interface/wireguard/peers remove [find interface="magic-cloud"]
/interface/wireguard remove [find name="magic-cloud"]
:put "MikroTik Magic Hub removed."
`;
}
