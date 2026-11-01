import { productForName, productPhotoUrl } from "./mikrotikProducts";

/** Only the hAP ax2 is the lab router; an ax3 or any other model must not borrow its photo. */
export const isHapAx2 = (name?: string) => productForName(name)?.id === "hap-ax2";

/** The model photo when the name identifies one exactly; otherwise a plain library symbol. Never a guess at live state. */
export function deviceImage(d: { hardware_name?: string; deployment_layer?: string }): string {
  const photo = productForName(d.hardware_name);
  if (photo) return productPhotoUrl(photo, import.meta.env.BASE_URL);
  return import.meta.env.BASE_URL + "topology/" + (d.deployment_layer === "MAIN_GATEWAY" ? "device.svg" : "ap.svg");
}
