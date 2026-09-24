-- Grand opening gift: 30% off Monthly & Annual, 23 Aug 2026 → 24 Sep 2026 inclusive (Asia/Yangon).
-- App date-gates on the same window. Safe to re-run.

UPDATE public.pricing_promo
SET
  starts_on = '2026-08-23',
  ends_on = '2026-09-24',
  label = 'Grand opening gift — 30% off Monthly & Annual',
  monthly_promo_mmk = 70000,
  monthly_standard_mmk = 100000,
  annual_promo_mmk = 700000,
  annual_standard_mmk = 1000000,
  active = true,
  updated_at = now()
WHERE id = true;
