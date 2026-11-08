/** Interpretation of evidence from a read-only gateway inventory, not a connectivity test. */
export interface DiscoverySourceHealth {
  sources: Array<{ name: string; available: boolean; count: number }>;
  observed_at?: string;
}

export type ObservationFreshness = "fresh" | "stale" | "unknown";

export function discoveryEvidenceHealth(snapshot: DiscoverySourceHealth, now = Date.now()) {
  const sources = snapshot.sources ?? [];
  const readable = sources.filter((source) => source.available).length;
  const total = sources.length;
  const observedAt = snapshot.observed_at ? Date.parse(snapshot.observed_at) : NaN;
  // Future timestamps and missing/invalid timestamps cannot establish fresh evidence.
  const ageMs = Number.isFinite(observedAt) ? now - observedAt : NaN;
  const freshness: ObservationFreshness = !Number.isFinite(ageMs) || ageMs < -60_000
    ? "unknown"
    : ageMs > 5 * 60_000 ? "stale" : "fresh";
  return { readable, total, noneReadable: readable === 0, freshness };
}
