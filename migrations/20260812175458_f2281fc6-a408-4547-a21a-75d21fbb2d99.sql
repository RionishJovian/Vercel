ALTER TABLE public.voucher_codes
  ALTER COLUMN deploy_version TYPE text USING deploy_version::text;