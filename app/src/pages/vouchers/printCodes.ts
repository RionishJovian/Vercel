import { mmk } from "../../lib/types";
import { qrPath } from "./qr";

export interface Layout {
  business_name: string; wifi_name: string; support_contact: string; terms: string;
  show_price: boolean; show_expiry: boolean; show_qr: boolean; a4_columns: number; a4_rows_per_page: number;
  thermal_columns: number; thermal_rows_per_strip: number; paper_width_mm: number;
}
export const DEFAULT_LAYOUT: Layout = { business_name: "", wifi_name: "", support_contact: "", terms: "", show_price: true, show_expiry: true, show_qr: false, a4_columns: 3, a4_rows_per_page: 8, thermal_columns: 1, thermal_rows_per_strip: 1, paper_width_mm: 210 };
export interface PrintVoucher { code: string; price_mmk?: number; duration_minutes?: number; plan_label?: string }

export const isThermal = (paperWidthMm: number) => paperWidthMm < 150;

// Printed cards are read by guests, so labels are Burmese and English.
function validity(minutes: number): string {
  if (minutes && minutes % 1440 === 0) return `${minutes / 1440} ရက် / day${minutes / 1440 > 1 ? "s" : ""}`;
  if (minutes && minutes % 60 === 0) return `${minutes / 60} နာရီ / hour${minutes / 60 > 1 ? "s" : ""}`;
  return minutes ? `${minutes} မိနစ် / min` : "";
}

export interface Receipt { business: string; code: string; qr: boolean; rows: Array<[string, string]>; terms: string }

/** What one receipt says. The live preview and the printed receipt both draw this, so they cannot disagree. */
export function receipt(v: PrintVoucher, layout: Partial<Layout>, firstUse: string): Receipt {
  const L = { ...DEFAULT_LAYOUT, ...layout };
  const rows: Array<[string, string]> = [];
  if (v.plan_label) rows.push(["ပလန် / Plan", v.plan_label]);
  if (L.show_price) rows.push(["စျေးနှုန်း / Price", mmk(v.price_mmk ?? 0) + " MMK"]);
  if (L.wifi_name) rows.push(["Wi-Fi", L.wifi_name]);
  if (L.show_expiry && v.duration_minutes) rows.push(["သက်တမ်း / Valid", validity(v.duration_minutes) + " · " + firstUse]);
  if (L.support_contact) rows.push(["ဆက်သွယ်ရန် / Support", L.support_contact]);
  return { business: L.business_name, code: v.code, qr: L.show_qr, rows, terms: L.show_expiry ? L.terms : "" };
}

const SVG = "http://www.w3.org/2000/svg";
function qrNode(code: string): SVGSVGElement {
  const { size, d } = qrPath(code);
  const svg = document.createElementNS(SVG, "svg");
  svg.setAttribute("viewBox", `0 0 ${size} ${size}`);
  svg.setAttribute("class", "qr");
  const path = document.createElementNS(SVG, "path");
  path.setAttribute("d", d);
  svg.append(path);
  return svg;
}

/** Renders the receipts as plain text nodes (never innerHTML) so voucher data cannot inject markup, then opens the print dialog. */
export function printCodes(vouchers: PrintVoucher[], layout: Partial<Layout>, firstUse: string): void {
  const L = { ...DEFAULT_LAYOUT, ...layout };
  const thermal = isThermal(Number(L.paper_width_mm));
  const root = document.createElement("div");
  root.className = "print-root" + (thermal ? " thermal" : "");
  if (thermal) {
    root.style.width = `${Number(L.paper_width_mm)}mm`;
    const page = document.createElement("style");
    page.textContent = "@page { margin: 0 }";
    root.append(page);
  } else {
    root.style.gridTemplateColumns = `repeat(${Math.max(1, Math.min(8, Number(L.a4_columns) || 3))},1fr)`;
  }
  const line = (text: string, cls?: string) => { const s = document.createElement(cls === "code" ? "b" : "span"); if (cls) s.className = cls; s.textContent = text; return s; };
  for (const v of vouchers) {
    const r = receipt(v, L, firstUse);
    const card = document.createElement("div");
    card.className = "card";
    if (r.business) card.append(line(r.business, "biz"));
    if (r.qr) card.append(qrNode(r.code));
    card.append(line(r.code, "code"));
    for (const [label, value] of r.rows) card.append(line(`${label}: ${value}`));
    if (r.terms) card.append(line(r.terms, "terms"));
    root.append(card);
  }
  document.body.append(root);
  document.body.classList.add("printing");
  const done = () => { document.body.classList.remove("printing"); root.remove(); window.removeEventListener("afterprint", done); };
  window.addEventListener("afterprint", done);
  window.print();
  setTimeout(() => { if (root.isConnected) done(); }, 60000);
}
