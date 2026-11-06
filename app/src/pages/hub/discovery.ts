/** Phase 1 discovery helpers. Nothing here talks to a network: an address is only a suggestion until the router connects through Magic Hub. */
export const DEFAULT_ROUTER_IP = "192.168.88.1"; // MikroTik factory default

/** Where an address came from. Neither is evidence that a router exists there. */
export type CandidateSource = "suggested" | "manual";

const OCTET = /^(0|[1-9]\d{0,2})$/;

/** A usable private IPv4 address for a router's own page, normalised; null for anything else (public, loopback, broadcast, malformed). */
export function parsePrivateIPv4(input: string): string | null {
  const parts = input.trim().split(".");
  if (parts.length !== 4 || !parts.every((p) => OCTET.test(p))) return null;
  const [a, b, c, d] = parts.map(Number);
  if ([a, b, c, d].some((n) => n > 255) || d === 0 || d === 255) return null;
  const isPrivate = a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168);
  return isPrivate ? `${a}.${b}.${c}.${d}` : null;
}

export const candidateSource = (ip: string): CandidateSource => (ip.trim() === DEFAULT_ROUTER_IP ? "suggested" : "manual");
