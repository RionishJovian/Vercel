// IPv4 CIDR helpers for the gateway bootstrap planner. Pure, browser-safe,
// dependency-free — identical results on server and client.

export type Ipv4Network = {
  /** Network address as a 32-bit unsigned integer. */
  network: number;
  prefix: number;
  /** Total addresses in the subnet. */
  size: number;
  /** First usable host (network + 1). */
  firstHost: number;
  /** Broadcast address (last address in the subnet). */
  broadcast: number;
};

const IP_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
const CIDR_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/;

/** Dotted-quad to 32-bit int, or null when malformed. */
export function parseIpv4(ip: string): number | null {
  const m = IP_RE.exec((ip ?? "").trim());
  if (!m) return null;
  const octets = m.slice(1, 5).map(Number);
  if (octets.some((n) => !Number.isInteger(n) || n < 0 || n > 255)) return null;
  return (((octets[0]! << 24) >>> 0) + (octets[1]! << 16) + (octets[2]! << 8) + octets[3]!) >>> 0;
}

export function toDotted(value: number): string {
  return [(value >>> 24) & 255, (value >>> 16) & 255, (value >>> 8) & 255, value & 255].join(".");
}

/**
 * Parse "10.5.50.1/24" into its subnet. Returns null for malformed input.
 * Prefix must be between /16 and /30: wider is not a sane guest LAN, narrower
 * leaves no room for a gateway plus a DHCP pool.
 */
export function parseNetwork(cidr: string): Ipv4Network | null {
  const m = CIDR_RE.exec((cidr ?? "").trim());
  if (!m) return null;
  const ip = parseIpv4(m.slice(1, 5).join("."));
  if (ip === null) return null;
  const prefix = Number(m[5]);
  if (!Number.isInteger(prefix) || prefix < 16 || prefix > 30) return null;
  const size = 2 ** (32 - prefix);
  const mask = size === 4294967296 ? 0 : ~(size - 1) >>> 0;
  const network = (ip & mask) >>> 0;
  return {
    network,
    prefix,
    size,
    firstHost: (network + 1) >>> 0,
    broadcast: (network + size - 1) >>> 0,
  };
}

/** The host part of "10.5.50.1/24" → "10.5.50.1". */
export function hostOf(cidr: string): string | null {
  const i = (cidr ?? "").indexOf("/");
  const host = (i === -1 ? cidr : cidr.slice(0, i)).trim();
  return parseIpv4(host) === null ? null : host;
}

/** Canonical network form: "10.5.50.1/24" → "10.5.50.0/24". */
export function networkCidr(cidr: string): string | null {
  const net = parseNetwork(cidr);
  return net ? `${toDotted(net.network)}/${net.prefix}` : null;
}

export function containsIp(net: Ipv4Network, ip: number): boolean {
  return ip >= net.network && ip <= net.broadcast;
}

export function networksOverlap(a: Ipv4Network, b: Ipv4Network): boolean {
  return a.network <= b.broadcast && b.network <= a.broadcast;
}

/** Parse a DHCP range "10.5.50.10-10.5.50.254" (or a single address). */
export function parseRange(range: string): { start: number; end: number } | null {
  const parts = (range ?? "").trim().split("-");
  if (parts.length < 1 || parts.length > 2) return null;
  const start = parseIpv4(parts[0]!);
  const end = parts.length === 2 ? parseIpv4(parts[1]!) : start;
  if (start === null || end === null) return null;
  if (end < start) return null;
  return { start, end };
}
