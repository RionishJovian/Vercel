-- Thermal cut-strip density for voucher print layouts (columns × rows packing on the roll).
ALTER TABLE public.voucher_print_layouts
  ADD COLUMN IF NOT EXISTS thermal_columns integer NOT NULL DEFAULT 2
    CHECK (thermal_columns IN (1, 2)),
  ADD COLUMN IF NOT EXISTS thermal_rows_per_strip integer NOT NULL DEFAULT 8
    CHECK (thermal_rows_per_strip BETWEEN 4 AND 40);
