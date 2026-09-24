-- Sapphire annual pass: 1,000,000 MMK standard, 700,000 MMK opening promo.
-- Sapphire Plus remains a separate add-on at 300,000 MMK / year (resolved in app code).

UPDATE public.pricing_promo
SET
  annual_standard_mmk = 1000000,
  annual_promo_mmk = 700000,
  updated_at = now()
WHERE id = true;
