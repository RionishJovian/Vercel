-- Standard Emerald / Sapphire prices: 95,000 MMK / month, 104,500 MMK / year.
-- Grand opening promo stays 30% off (66,500 / 73,150).

UPDATE public.pricing_promo
SET
  monthly_standard_mmk = 95000,
  monthly_promo_mmk = 66500,
  annual_standard_mmk = 104500,
  annual_promo_mmk = 73150,
  updated_at = now()
WHERE id = true;
