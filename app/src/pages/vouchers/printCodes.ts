import { mmk } from "../../lib/types";

export interface Layout {
  business_name: string; wifi_name: string; support_contact: string; terms: string;
  show_price: boolean; show_expiry: boolean; show_qr: boolean; a4_columns: number; a4_rows_per_page: number;
  thermal_columns: number; thermal_rows_per_strip: number; paper_width_mm: number;
}
export const DEFAULT_LAYOUT: Layout = { business_name: "", wifi_name: "", support_contact: "", terms: "", show_price: true, show_expiry: true, show_qr: false, a4_columns: 3, a4_rows_per_page: 8, thermal_columns: 1, thermal_rows_per_strip: 1, paper_width_mm: 210 };
export interface PrintVoucher { code: string; price_mmk?: number; duration_minutes?: number }

// Printed cards are read by guests, so labels are Burmese and English.
function validity(minutes: number): string {
  if (minutes && minutes % 1440 === 0) return `${minutes / 1440} ရက် / day${minutes / 1440 > 1 ? "s" : ""}`;
  if (minutes && minutes % 60 === 0) return `${minutes / 60} နာရီ / hour${minutes / 60 > 1 ? "s" : ""}`;
  return minutes ? `${minutes} မိနစ် / min` : "";
}

/** Renders the sheet as plain text nodes (never innerHTML) so voucher data cannot inject markup, then opens the print dialog. */
export function printCodes(vouchers: PrintVoucher[], layout: Partial<Layout>, firstUse: string): void {
  const L = { ...DEFAULT_LAYOUT, ...layout };
  const root = document.createElement("div");
  root.className = "print-root";
  root.style.gridTemplateColumns = `repeat(${Math.max(1, Math.min(8, Number(L.a4_columns) || 3))},1fr)`;
  const line = (text: string, cls?: string) => { const s = document.createElement(cls === "code" ? "b" : "span"); if (cls) s.className = cls; s.textContent = text; return s; };
  for (const v of vouchers) {
    const card = document.createElement("div");
    card.className = "card";
    if (L.business_name) card.append(line(L.business_name, "biz"));
    if (L.wifi_name) card.append(line("Wi-Fi: " + L.wifi_name));
    card.append(line(v.code, "code"));
    if (L.show_price) card.append(line(mmk(v.price_mmk ?? 0) + " MMK"));
    if (L.show_expiry && v.duration_minutes) card.append(line(validity(v.duration_minutes) + " · " + firstUse));
    if (L.terms) card.append(line(L.terms, "terms"));
    if (L.support_contact) card.append(line(L.support_contact));
    root.append(card);
  }
  document.body.append(root);
  document.body.classList.add("printing");
  const done = () => { document.body.classList.remove("printing"); root.remove(); window.removeEventListener("afterprint", done); };
  window.addEventListener("afterprint", done);
  window.print();
  setTimeout(() => { if (root.isConnected) done(); }, 60000);
}
