export const VOUCHER_PRINT_WIDTHS = [58, 80] as const;
export type VoucherPrintWidth = (typeof VOUCHER_PRINT_WIDTHS)[number];

/** A4 cut-sheet columns. Wider grids pack more codes per page. */
export const A4_CUT_SHEET_COLUMNS = [2, 3, 4, 5] as const;
export type A4CutSheetColumns = (typeof A4_CUT_SHEET_COLUMNS)[number];

/** How many equal tile rows fit on one A4 page (portrait). */
export const A4_ROWS_PER_PAGE_MIN = 4;
export const A4_ROWS_PER_PAGE_MAX = 50;
export const A4_ROWS_PER_PAGE_DEFAULT = 12;

/** Thermal roll columns. 58 mm fits 1; 80 mm can pack 2 side-by-side. */
export const THERMAL_CUT_STRIP_COLUMNS = [1, 2] as const;
export type ThermalCutStripColumns = (typeof THERMAL_CUT_STRIP_COLUMNS)[number];

/**
 * Rows used for thermal tile density (preview strip + print tile height).
 * The roll is continuous; this is packing density, not a hard page height.
 */
export const THERMAL_ROWS_PER_STRIP_MIN = 4;
export const THERMAL_ROWS_PER_STRIP_MAX = 40;
export const THERMAL_ROWS_PER_STRIP_DEFAULT = 8;

/** One tenant-owned counter-print layout. It is deliberately separate from portal presentation. */
export type VoucherPrintLayout = {
  business_name: string;
  wifi_name: string;
  support_contact: string;
  terms: string;
  paper_width_mm: VoucherPrintWidth;
  show_qr: boolean;
  show_price: boolean;
  show_expiry: boolean;
  /** A4 cut-sheet columns (2–5). */
  a4_columns: A4CutSheetColumns;
  /** Equal tile rows that should fit on one A4 page (4–50). */
  a4_rows_per_page: number;
  /** Thermal cut-strip columns (1–2). */
  thermal_columns: ThermalCutStripColumns;
  /** Tile density rows for the thermal strip preview / print (4–40). */
  thermal_rows_per_strip: number;
};

export const DEFAULT_VOUCHER_PRINT_LAYOUT: VoucherPrintLayout = {
  business_name: "Wi-Fi",
  wifi_name: "Hotspot",
  support_contact: "Ask the front desk",
  terms: "Enter this code in the captive portal. One voucher is for its assigned access plan.",
  paper_width_mm: 80,
  show_qr: true,
  show_price: true,
  show_expiry: true,
  a4_columns: 3,
  a4_rows_per_page: A4_ROWS_PER_PAGE_DEFAULT,
  thermal_columns: 2,
  thermal_rows_per_strip: THERMAL_ROWS_PER_STRIP_DEFAULT,
};

export function clampA4RowsPerPage(value: unknown): number {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return A4_ROWS_PER_PAGE_DEFAULT;
  return Math.min(A4_ROWS_PER_PAGE_MAX, Math.max(A4_ROWS_PER_PAGE_MIN, n));
}

export function normalizeA4Columns(value: unknown): A4CutSheetColumns {
  const n = Number(value);
  return (A4_CUT_SHEET_COLUMNS as readonly number[]).includes(n)
    ? (n as A4CutSheetColumns)
    : DEFAULT_VOUCHER_PRINT_LAYOUT.a4_columns;
}

export function clampThermalRowsPerStrip(value: unknown): number {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return THERMAL_ROWS_PER_STRIP_DEFAULT;
  return Math.min(THERMAL_ROWS_PER_STRIP_MAX, Math.max(THERMAL_ROWS_PER_STRIP_MIN, n));
}

export function normalizeThermalColumns(
  value: unknown,
  paperWidthMm: VoucherPrintWidth = 80,
): ThermalCutStripColumns {
  const n = Number(value);
  if ((THERMAL_CUT_STRIP_COLUMNS as readonly number[]).includes(n)) {
    // 58 mm is too narrow for two readable code tiles.
    if (paperWidthMm === 58 && n === 2) return 1;
    return n as ThermalCutStripColumns;
  }
  return paperWidthMm === 58 ? 1 : DEFAULT_VOUCHER_PRINT_LAYOUT.thermal_columns;
}

/** Rough vouchers-per-page estimate for the live preview copy. */
export function a4VouchersPerPage(columns: A4CutSheetColumns, rowsPerPage: number): number {
  return columns * clampA4RowsPerPage(rowsPerPage);
}

/** Codes shown in one thermal density strip (columns × rows). */
export function thermalVouchersPerStrip(
  columns: ThermalCutStripColumns,
  rowsPerStrip: number,
): number {
  return columns * clampThermalRowsPerStrip(rowsPerStrip);
}

/** Scale tile typography from the chosen rows-per-page density (shared by print + live preview). */
export function a4CutSheetDensity(rowsPerPage: number): {
  rowHeightMm: number;
  codeFontPx: number;
  metaFontPx: number;
  brandFontPx: number;
  padMm: number;
  showMeta: boolean;
} {
  const rows = clampA4RowsPerPage(rowsPerPage);
  // Usable A4 portrait height after ~8mm page margins and a small hint strip.
  const usableHeightMm = 275;
  const rowHeightMm = usableHeightMm / rows;
  const codeFontPx = Math.max(7, Math.min(18, rowHeightMm * 1.55));
  const metaFontPx = Math.max(5.5, Math.min(10, rowHeightMm * 0.85));
  const brandFontPx = Math.max(5.5, Math.min(9, rowHeightMm * 0.75));
  const padMm = Math.max(0.6, Math.min(3.5, rowHeightMm * 0.12));
  return {
    rowHeightMm,
    codeFontPx,
    metaFontPx,
    brandFontPx,
    padMm,
    // Below ~9mm tiles, drop plan/price/expiry so the code stays readable.
    showMeta: rowHeightMm >= 9,
  };
}

/**
 * Thermal multi-up density. Uses a ~120 mm virtual strip height so rows control tile size
 * the same way A4 rows control page packing; print continues down the roll.
 */
export function thermalCutStripDensity(rowsPerStrip: number): {
  rowHeightMm: number;
  codeFontPx: number;
  metaFontPx: number;
  brandFontPx: number;
  padMm: number;
  showMeta: boolean;
} {
  const rows = clampThermalRowsPerStrip(rowsPerStrip);
  const usableHeightMm = 120;
  const rowHeightMm = usableHeightMm / rows;
  const codeFontPx = Math.max(8, Math.min(20, rowHeightMm * 1.65));
  const metaFontPx = Math.max(5.5, Math.min(11, rowHeightMm * 0.9));
  const brandFontPx = Math.max(5.5, Math.min(10, rowHeightMm * 0.8));
  const padMm = Math.max(0.5, Math.min(3, rowHeightMm * 0.14));
  return {
    rowHeightMm,
    codeFontPx,
    metaFontPx,
    brandFontPx,
    padMm,
    showMeta: rowHeightMm >= 10,
  };
}

export function normalizeVoucherPrintLayout(
  raw: Partial<
    Omit<
      VoucherPrintLayout,
      "paper_width_mm" | "a4_columns" | "a4_rows_per_page" | "thermal_columns" | "thermal_rows_per_strip"
    >
  > & {
    paper_width_mm?: number;
    a4_columns?: number;
    a4_rows_per_page?: number;
    thermal_columns?: number;
    thermal_rows_per_strip?: number;
  },
): VoucherPrintLayout {
  const text = (value: unknown, fallback: string) =>
    typeof value === "string" && value.trim() ? value.trim() : fallback;
  const width = Number(raw.paper_width_mm) === 58 ? 58 : 80;
  return {
    business_name: text(raw.business_name, DEFAULT_VOUCHER_PRINT_LAYOUT.business_name),
    wifi_name: text(raw.wifi_name, DEFAULT_VOUCHER_PRINT_LAYOUT.wifi_name),
    support_contact: text(raw.support_contact, DEFAULT_VOUCHER_PRINT_LAYOUT.support_contact),
    terms: text(raw.terms, DEFAULT_VOUCHER_PRINT_LAYOUT.terms),
    paper_width_mm: width,
    show_qr: raw.show_qr !== false,
    show_price: raw.show_price !== false,
    show_expiry: raw.show_expiry !== false,
    a4_columns: normalizeA4Columns(raw.a4_columns),
    a4_rows_per_page: clampA4RowsPerPage(raw.a4_rows_per_page),
    thermal_columns: normalizeThermalColumns(raw.thermal_columns, width),
    thermal_rows_per_strip: clampThermalRowsPerStrip(raw.thermal_rows_per_strip),
  };
}
