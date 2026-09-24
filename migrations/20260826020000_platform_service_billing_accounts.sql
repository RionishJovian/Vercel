-- Global bank accounts for MikroMagic Services purchases (Tier Passes and
-- Magic Coins). These are deliberately separate from tenant guest-checkout
-- accounts: a tenant Primary must never be able to redirect platform billing.

CREATE TABLE IF NOT EXISTS public.platform_service_bank_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slot integer NOT NULL CHECK (slot IN (1, 2)),
  holder_name text NOT NULL DEFAULT '',
  bank_name text NOT NULL DEFAULT '',
  account_number text NOT NULL DEFAULT '',
  enabled boolean NOT NULL DEFAULT false,
  sort integer NOT NULL DEFAULT 0,
  configured_by uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (slot)
);

CREATE OR REPLACE FUNCTION public.platform_service_bank_accounts_touch()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_platform_service_bank_accounts_touch ON public.platform_service_bank_accounts;
CREATE TRIGGER trg_platform_service_bank_accounts_touch
  BEFORE UPDATE ON public.platform_service_bank_accounts
  FOR EACH ROW EXECUTE FUNCTION public.platform_service_bank_accounts_touch();

REVOKE EXECUTE ON FUNCTION public.platform_service_bank_accounts_touch() FROM PUBLIC, anon, authenticated;

ALTER TABLE public.platform_service_bank_accounts ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.platform_service_bank_accounts FROM PUBLIC, anon, authenticated;
GRANT ALL ON public.platform_service_bank_accounts TO service_role;
