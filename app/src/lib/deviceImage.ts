/** Only the hAP ax2 has a photo here; an ax3 or any other model must not borrow it. */
export const isHapAx2 = (name?: string) => /hap\s*ax(?:\^?2|²)(?![0-9])/i.test(name ?? "");

/** The model photo when the name identifies one; otherwise a plain library symbol. Never a guess at live state. */
export function deviceImage(d: { hardware_name?: string; deployment_layer?: string }): string {
  const base = import.meta.env.BASE_URL + "topology/";
  if (isHapAx2(d.hardware_name)) return base + "hap-ax2.webp";
  return base + (d.deployment_layer === "MAIN_GATEWAY" ? "device.svg" : "ap.svg");
}
