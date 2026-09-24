-- Sapphire Plus replaces the annual pass at 300,000 MMK / year (multi-device included).
-- Keep launch promo below the standard so the pricing page never shows a
-- "promo" higher than the list price.
UPDATE public.pricing_promo
SET
  annual_standard_mmk = 300000,
  annual_promo_mmk = LEAST(annual_promo_mmk, 250000),
  updated_at = now()
WHERE id = true;
