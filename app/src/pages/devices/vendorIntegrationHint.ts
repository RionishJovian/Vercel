/** Integration status, not a claim that a discovered AP is manageable. */
export type VendorIntegration = {
  family: "tp-link-omada" | "ruijie-reyee" | "ikuai" | "huawei" | "unknown";
  label: string;
  inventory: "enrollment_required" | "not_integrated";
  configuration: "not_verified";
};

/** A neighbor-reported vendor name is advisory and must never grant write access. */
export function vendorIntegrationHint(reportedVendor: string | undefined): VendorIntegration {
  const name = (reportedVendor ?? "").trim().toLowerCase();
  const base = { configuration: "not_verified" } as const;
  if (name === "tp-link" || name === "omada" || name === "tp-link omada") {
    return { ...base, family: "tp-link-omada", label: "TP-Link Omada", inventory: "enrollment_required" };
  }
  if (name === "ruijie" || name === "reyee" || name === "ruijie/reyee") {
    return { ...base, family: "ruijie-reyee", label: "Ruijie / Reyee", inventory: "not_integrated" };
  }
  if (name === "ikuai") {
    return { ...base, family: "ikuai", label: "iKuai", inventory: "not_integrated" };
  }
  if (name === "huawei") {
    return { ...base, family: "huawei", label: "Huawei", inventory: "not_integrated" };
  }
  return { ...base, family: "unknown", label: "Unknown vendor", inventory: "not_integrated" };
}
