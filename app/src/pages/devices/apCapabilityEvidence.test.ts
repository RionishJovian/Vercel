import { describe, expect, it } from "vitest";
import { apCapabilityEvidence, AP_CAPABILITY_NAMES } from "./apCapabilityEvidence";

describe("AP capability evidence", () => {
  it("never promotes a missing or vendor-only hint to a management capability", () => {
    for (const name of AP_CAPABILITY_NAMES) {
      expect(apCapabilityEvidence([], name)).toEqual({ verified: false, evidence: null });
      expect(apCapabilityEvidence([{ name, status: "unknown", evidence: "reported Ruijie" }], name).verified).toBe(false);
    }
  });
  it("requires a specific provenance before reporting a capability observed", () => {
    expect(apCapabilityEvidence([{ name: "ssid_configuration", status: "observed" }], "ssid_configuration").verified).toBe(false);
    expect(apCapabilityEvidence([{ name: "ssid_configuration", status: "observed", evidence: "   " }], "ssid_configuration").verified).toBe(false);
    expect(apCapabilityEvidence([{ name: "ssid_configuration", status: "observed", evidence: "controller:verified-adapter" }], "ssid_configuration"))
      .toEqual({ verified: true, evidence: "controller:verified-adapter" });
  });
  it("does not transfer evidence between different features", () => {
    const caps = [{ name: "ap_clients", status: "observed", evidence: "controller:read-only" }];
    expect(apCapabilityEvidence(caps, "ap_clients").verified).toBe(true);
    expect(apCapabilityEvidence(caps, "radio_optimization").verified).toBe(false);
    expect(apCapabilityEvidence(caps, "third_party_management").verified).toBe(false);
  });
  it("does not mistake unsupported or case-varied statuses for verified", () => {
    for (const status of ["unsupported", "unknown", "OBSERVED", ""]) {
      expect(apCapabilityEvidence([{ name: "radio_optimization", status, evidence: "controller:x" }], "radio_optimization").verified).toBe(false);
    }
  });
});
