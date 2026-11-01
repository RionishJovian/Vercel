/** The model photo when the name identifies one; otherwise a plain library symbol. Never a guess at live state. */
export function deviceImage(d: { hardware_name?: string; deployment_layer?: string }): string {
  const base = import.meta.env.BASE_URL + "topology/";
  if (/hap\s*ax/i.test(d.hardware_name ?? "")) return base + "hap-ax2.webp";
  return base + (d.deployment_layer === "MAIN_GATEWAY" ? "device.svg" : "ap.svg");
}
