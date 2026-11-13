/** Reported capabilities are evidence claims, not permissions to change a device. */
export interface DeviceCapability {
  name: string;
  status: string;
  evidence?: string;
}

export const AP_CAPABILITY_NAMES = [
  "third_party_management",
  "ap_clients",
  "ssid_configuration",
  "radio_optimization",
] as const;

export type APCapabilityName = typeof AP_CAPABILITY_NAMES[number];

export function apCapabilityEvidence(
  capabilities: readonly DeviceCapability[] | undefined,
  name: APCapabilityName,
): { verified: boolean; evidence: string | null } {
  // Observed without specific source evidence cannot be treated as a verified capability.
  const matches = (capabilities ?? []).filter(cap => cap.name === name);
  const verified = matches.find(cap =>
    cap.status === "observed" && typeof cap.evidence === "string" && cap.evidence.trim().length > 0,
  );
  return verified
    ? { verified: true, evidence: verified.evidence!.trim() }
    : { verified: false, evidence: null };
}
