import QRCode from "qrcode";
import { toast } from "sonner";
import {
  a4CutSheetDensity,
  clampA4RowsPerPage,
  clampThermalRowsPerStrip,
  normalizeA4Columns,
  normalizeThermalColumns,
  thermalCutStripDensity,
  type A4CutSheetColumns,
  type ThermalCutStripColumns,
  type VoucherPrintLayout,
} from "./voucher-print-layout";

export { a4CutSheetDensity, thermalCutStripDensity } from "./voucher-print-layout";

export type PrintableVoucher = {
  code: string;
  profile: string;
  priceMmk?: number;
  expiresAt?: string | null;
};

const escapeHtml = (value: string) =>
  value.replace(
    /[&<>'"]/g,
    (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[char]!,
  );

/** Opens the browser print dialog only; it never sends voucher data anywhere. */
export async function printVoucherThermalReceipt(
  voucher: PrintableVoucher,
  layout: VoucherPrintLayout,
) {
  const win = window.open("", "_blank", "width=420,height=760");
  if (!win) {
    toast.error("Popup blocked. Allow popups to print a voucher receipt.");
    return;
  }
  let qr = "";
  if (layout.show_qr) {
    try {
      qr = await QRCode.toDataURL(voucher.code, {
        width: 260,
        margin: 1,
        errorCorrectionLevel: "M",
      });
    } catch {
      // The voucher code remains the primary credential if QR generation fails.
    }
  }
  const price = voucher.priceMmk ? `${voucher.priceMmk.toLocaleString()} MMK` : "Included";
  const expiry = voucher.expiresAt
    ? new Date(voucher.expiresAt).toLocaleString()
    : "Starts when first used";
  const width = layout.paper_width_mm === 58 ? 52 : 74;
  const lines = [
    `Plan: ${escapeHtml(voucher.profile)}`,
    ...(layout.show_price ? [`Price: ${escapeHtml(price)}`] : []),
    `Wi-Fi: ${escapeHtml(layout.wifi_name)}`,
    ...(layout.show_expiry ? [`Expiry: ${escapeHtml(expiry)}`] : []),
    `Support: ${escapeHtml(layout.support_contact)}`,
  ].join("<br>");
  win.document
    .write(`<!doctype html><html><head><meta charset="utf-8"><title>Voucher receipt</title><style>
    @page { size:${layout.paper_width_mm}mm auto; margin:3mm; } body{width:${width}mm;margin:0 auto;font-family:ui-monospace,Menlo,monospace;color:#111;font-size:12px;text-align:center}.rule{border-top:1px dashed #111;margin:8px 0}.code{font-size:24px;font-weight:800;letter-spacing:2px;margin:10px 0}.left{text-align:left}.muted{font-size:10px;color:#444}.qr{width:34mm;height:34mm;image-rendering:auto}@media print{body{width:${width}mm}}
  </style></head><body><b>${escapeHtml(layout.business_name)}</b><div class="muted">Wi-Fi voucher receipt</div><div class="rule"></div>${qr ? `<img class="qr" src="${qr}" alt="Voucher QR code"/>` : ""}<div class="code">${escapeHtml(voucher.code)}</div><div class="left">${lines}</div><div class="rule"></div><div class="muted">${escapeHtml(layout.terms)}</div><div class="muted">Connect to Wi-Fi, then enter this voucher code in the captive portal.</div><script>window.onload=()=>setTimeout(()=>window.print(),150)</script></body></html>`);
  win.document.close();
}

/** Batch print uses the A4 cut-sheet layout (equal tiles + dotted scissor lines). */
export async function printVoucherBatch(vouchers: PrintableVoucher[], layout: VoucherPrintLayout) {
  await printVoucherA4CutSheet(
    vouchers.map((voucher) => ({
      code: voucher.code,
      profile: voucher.profile,
      priceMmk: voucher.priceMmk,
      expiresAt: voucher.expiresAt,
    })),
    {
      title: layout.business_name || layout.wifi_name || "Wi-Fi vouchers",
      showPrice: layout.show_price,
      showExpiry: layout.show_expiry,
      columns: layout.a4_columns,
      rowsPerPage: layout.a4_rows_per_page,
    },
  );
}

/** Batch thermal print packs many codes on the roll (cut-strip), not one tall receipt. */
export async function printVoucherThermalBatch(
  vouchers: PrintableVoucher[],
  layout: VoucherPrintLayout,
) {
  await printVoucherThermalCutStrip(vouchers, layout);
}

/** Default columns when the layout has not chosen one yet (legacy helper). */
export function a4CutSheetColumns(count: number): 2 | 3 {
  return count > 10 ? 3 : 2;
}

export type A4CutSheetVoucher = {
  code: string;
  profile: string;
  priceMmk?: number | null;
  expiresAt?: string | null;
};

export type A4CutSheetOptions = {
  title?: string;
  showPrice?: boolean;
  showExpiry?: boolean;
  columns?: A4CutSheetColumns | 2 | 3;
  /** Equal tile rows that should fit on one A4 page. */
  rowsPerPage?: number;
};

/**
 * A4 multi-up sheet with equal tiles and dotted scissor cut lines.
 * Columns + rows-per-page control how densely codes pack onto each page.
 */
export async function printVoucherA4CutSheet(
  vouchers: A4CutSheetVoucher[],
  options?: A4CutSheetOptions,
) {
  if (!vouchers.length) {
    toast.error("No vouchers to print.");
    return;
  }
  const win = window.open("", "_blank", "width=900,height=1100");
  if (!win) {
    toast.error("Popup blocked. Allow popups to print the A4 cut sheet.");
    return;
  }
  const columns = normalizeA4Columns(options?.columns ?? a4CutSheetColumns(vouchers.length));
  const rowsPerPage = clampA4RowsPerPage(options?.rowsPerPage ?? 12);
  const density = a4CutSheetDensity(rowsPerPage);
  const title = escapeHtml(options?.title?.trim() || "Wi-Fi vouchers");
  const showPrice = options?.showPrice !== false && density.showMeta;
  const showExpiry = options?.showExpiry !== false && density.showMeta;
  const showPlan = density.showMeta;
  const perPage = columns * rowsPerPage;
  const tiles = vouchers
    .map((voucher) => {
      const price =
        voucher.priceMmk != null && Number.isFinite(Number(voucher.priceMmk))
          ? `${Number(voucher.priceMmk).toLocaleString()} MMK`
          : "";
      const expiry = voucher.expiresAt
        ? new Date(voucher.expiresAt).toLocaleString()
        : "";
      return `<article class="tile">
  <div class="brand">${title}</div>
  <div class="label">Wi-Fi voucher · cut on dots</div>
  <div class="code">${escapeHtml(voucher.code)}</div>
  ${showPlan ? `<div class="meta">Plan: ${escapeHtml(voucher.profile)}</div>` : ""}
  ${showPrice && price ? `<div class="meta">Price: ${escapeHtml(price)}</div>` : ""}
  ${showExpiry && expiry ? `<div class="meta">Expiry: ${escapeHtml(expiry)}</div>` : ""}
</article>`;
    })
    .join("");

  win.document.write(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>A4 voucher cut sheet</title>
<style>
  @page { size: A4 portrait; margin: 8mm; }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    color: #111;
    background: #fff;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  }
  .hint {
    font-size: 10px;
    color: #555;
    margin: 0 0 2mm;
    display: flex;
    justify-content: space-between;
    gap: 8px;
  }
  .sheet {
    display: grid;
    grid-template-columns: repeat(${columns}, minmax(0, 1fr));
    grid-auto-rows: ${density.rowHeightMm.toFixed(3)}mm;
    gap: 0;
    border-top: 1px dotted #222;
    border-left: 1px dotted #222;
  }
  .tile {
    height: ${density.rowHeightMm.toFixed(3)}mm;
    padding: ${density.padMm.toFixed(2)}mm ${Math.max(1.2, density.padMm).toFixed(2)}mm;
    text-align: center;
    border-right: 1px dotted #222;
    border-bottom: 1px dotted #222;
    break-inside: avoid;
    page-break-inside: avoid;
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: center;
    overflow: hidden;
  }
  .tile::after {
    content: "✂";
    position: absolute;
    right: -0.55em;
    top: 50%;
    transform: translateY(-50%);
    font-size: 8px;
    color: #666;
    line-height: 1;
  }
  .tile:nth-child(${columns}n)::after { content: none; }
  .brand {
    font-size: ${density.brandFontPx.toFixed(1)}px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #555;
    line-height: 1.1;
  }
  .label {
    font-size: ${Math.max(5, density.metaFontPx - 0.5).toFixed(1)}px;
    color: #777;
    margin-top: 0.4mm;
    line-height: 1.1;
  }
  .code {
    font: 800 ${density.codeFontPx.toFixed(1)}px/1.1 ui-monospace, Menlo, Consolas, monospace;
    letter-spacing: 0.06em;
    margin: 0.8mm 0 0.5mm;
    word-break: break-all;
  }
  .meta {
    font-size: ${density.metaFontPx.toFixed(1)}px;
    color: #555;
    margin-top: 0.3mm;
    line-height: 1.15;
  }
  @media print {
    .hint { display: none; }
    .tile::after { color: #333; }
  }
</style>
</head>
<body>
  <div class="hint">
    <span>A4 cut sheet · equal tiles · cut on the dotted lines</span>
    <span>${vouchers.length} voucher${vouchers.length === 1 ? "" : "s"} · ${columns}×${rowsPerPage} ≈ ${perPage}/page</span>
  </div>
  <div class="sheet">${tiles}</div>
  <script>window.onload=()=>setTimeout(()=>window.print(),180)</script>
</body>
</html>`);
  win.document.close();
}

export type ThermalCutStripOptions = {
  title?: string;
  showPrice?: boolean;
  showExpiry?: boolean;
  paperWidthMm?: 58 | 80;
  columns?: ThermalCutStripColumns | 1 | 2;
  /** Tile density rows (controls height / fonts). Strip grows down the roll. */
  rowsPerStrip?: number;
};

/**
 * Thermal multi-up strip: equal tiles across the roll width with dashed cut lines.
 * Density mirrors A4 packing so cafés print many codes instead of one tall receipt.
 */
export async function printVoucherThermalCutStrip(
  vouchers: PrintableVoucher[],
  layoutOrOptions?: VoucherPrintLayout | ThermalCutStripOptions,
) {
  if (!vouchers.length) {
    toast.error("No vouchers to print.");
    return;
  }
  const isLayout =
    layoutOrOptions != null &&
    typeof layoutOrOptions === "object" &&
    "paper_width_mm" in layoutOrOptions &&
    "thermal_columns" in layoutOrOptions;
  const layout = isLayout ? (layoutOrOptions as VoucherPrintLayout) : null;
  const options = !isLayout ? (layoutOrOptions as ThermalCutStripOptions | undefined) : undefined;

  const paperWidthMm = (layout?.paper_width_mm ?? options?.paperWidthMm ?? 80) === 58 ? 58 : 80;
  const columns = normalizeThermalColumns(
    layout?.thermal_columns ?? options?.columns ?? (paperWidthMm === 58 ? 1 : 2),
    paperWidthMm,
  );
  const rowsPerStrip = clampThermalRowsPerStrip(
    layout?.thermal_rows_per_strip ?? options?.rowsPerStrip ?? 8,
  );
  const density = thermalCutStripDensity(rowsPerStrip);
  const title = escapeHtml(
    (layout?.business_name || layout?.wifi_name || options?.title || "Wi-Fi vouchers").trim() ||
      "Wi-Fi vouchers",
  );
  const showPrice =
    (layout ? layout.show_price : options?.showPrice !== false) && density.showMeta;
  const showExpiry =
    (layout ? layout.show_expiry : options?.showExpiry !== false) && density.showMeta;
  const showPlan = density.showMeta;
  const bodyWidth = paperWidthMm === 58 ? 52 : 74;

  const win = window.open("", "_blank", "width=420,height=900");
  if (!win) {
    toast.error("Popup blocked. Allow popups to print the thermal strip.");
    return;
  }

  const tiles = vouchers
    .map((voucher) => {
      const price =
        voucher.priceMmk != null && Number.isFinite(Number(voucher.priceMmk))
          ? `${Number(voucher.priceMmk).toLocaleString()} MMK`
          : "";
      const expiry = voucher.expiresAt ? new Date(voucher.expiresAt).toLocaleString() : "";
      return `<article class="tile">
  <div class="brand">${title}</div>
  <div class="label">Wi-Fi · cut</div>
  <div class="code">${escapeHtml(voucher.code)}</div>
  ${showPlan ? `<div class="meta">${escapeHtml(voucher.profile)}</div>` : ""}
  ${showPrice && price ? `<div class="meta">${escapeHtml(price)}</div>` : ""}
  ${showExpiry && expiry ? `<div class="meta">${escapeHtml(expiry)}</div>` : ""}
</article>`;
    })
    .join("");

  win.document.write(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>Thermal voucher strip</title>
<style>
  @page { size: ${paperWidthMm}mm auto; margin: 2mm; }
  * { box-sizing: border-box; }
  body {
    margin: 0 auto;
    width: ${bodyWidth}mm;
    color: #111;
    background: #fff;
    font-family: ui-monospace, Menlo, Consolas, monospace;
  }
  .hint {
    font-size: 9px;
    color: #555;
    margin: 0 0 1.5mm;
    display: flex;
    justify-content: space-between;
    gap: 6px;
  }
  .strip {
    display: grid;
    grid-template-columns: repeat(${columns}, minmax(0, 1fr));
    grid-auto-rows: ${density.rowHeightMm.toFixed(3)}mm;
    gap: 0;
    border-top: 1px dashed #222;
    border-left: 1px dashed #222;
  }
  .tile {
    height: ${density.rowHeightMm.toFixed(3)}mm;
    padding: ${density.padMm.toFixed(2)}mm ${Math.max(0.8, density.padMm).toFixed(2)}mm;
    text-align: center;
    border-right: 1px dashed #222;
    border-bottom: 1px dashed #222;
    break-inside: avoid;
    page-break-inside: avoid;
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: center;
    overflow: hidden;
  }
  .tile::after {
    content: "✂";
    position: absolute;
    right: -0.45em;
    top: 50%;
    transform: translateY(-50%);
    font-size: 7px;
    color: #666;
    line-height: 1;
  }
  .tile:nth-child(${columns}n)::after { content: none; }
  .brand {
    font-size: ${density.brandFontPx.toFixed(1)}px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: #555;
    line-height: 1.1;
  }
  .label {
    font-size: ${Math.max(5, density.metaFontPx - 0.5).toFixed(1)}px;
    color: #777;
    margin-top: 0.3mm;
    line-height: 1.1;
  }
  .code {
    font: 800 ${density.codeFontPx.toFixed(1)}px/1.1 ui-monospace, Menlo, Consolas, monospace;
    letter-spacing: 0.05em;
    margin: 0.6mm 0 0.4mm;
    word-break: break-all;
  }
  .meta {
    font-size: ${density.metaFontPx.toFixed(1)}px;
    color: #555;
    margin-top: 0.25mm;
    line-height: 1.15;
  }
  @media print {
    .hint { display: none; }
    body { width: ${bodyWidth}mm; }
  }
</style>
</head>
<body>
  <div class="hint">
    <span>Thermal ${paperWidthMm} mm · cut on dashes</span>
    <span>${vouchers.length} voucher${vouchers.length === 1 ? "" : "s"} · ${columns}×${rowsPerStrip} dens.</span>
  </div>
  <div class="strip">${tiles}</div>
  <script>window.onload=()=>setTimeout(()=>window.print(),180)</script>
</body>
</html>`);
  win.document.close();
}

