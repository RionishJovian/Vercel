-- Launch promo option B: official launch 25 Aug 2026 through ~30 days (24 Sep 2026).
-- Inclusive window. App also date-gates on Asia/Yangon calendar day.
-- Paste in Lovable Cloud SQL Editor after merge/publish of app code.

UPDATE public.pricing_promo
SET
  starts_on = '2026-08-25',
  ends_on = '2026-09-24',
  label = 'MikroTik Magic launch gift',
  monthly_promo_mmk = 70000,
  monthly_standard_mmk = 100000,
  annual_promo_mmk = 700000,
  annual_standard_mmk = 1000000,
  active = true,
  updated_at = now()
WHERE id = true;
