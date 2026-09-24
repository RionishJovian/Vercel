-- Account-bound WebFig unlock key (item 1000).
--
-- This is intentionally not a client-writable entitlement. The purchase RPC
-- locks the wallet, writes the immutable debit and records the 12-hour key in
-- one transaction. A key cannot be transferred because every activation is
-- permanently tied to auth.uid().

CREATE TABLE IF NOT EXISTS public.webfig_unlock_key_activations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  key_id integer NOT NULL DEFAULT 1000 CHECK (key_id = 1000),
  key_name text NOT NULL DEFAULT 'Inv hub misc key' CHECK (key_name = 'Inv hub misc key'),
  price_coins numeric(12,2) NOT NULL DEFAULT 5 CHECK (price_coins = 5),
  attributes text NOT NULL DEFAULT 'Unlock the WebFig' CHECK (attributes = 'Unlock the WebFig'),
  account_bound boolean NOT NULL DEFAULT true CHECK (account_bound = true),
  non_transferable boolean NOT NULL DEFAULT true CHECK (non_transferable = true),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS webfig_unlock_key_active_lookup_idx
  ON public.webfig_unlock_key_activations(user_id, expires_at DESC);

ALTER TABLE public.webfig_unlock_key_activations ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.webfig_unlock_key_activations FROM PUBLIC, anon, authenticated;
GRANT SELECT ON public.webfig_unlock_key_activations TO authenticated;

CREATE POLICY webfig_unlock_key_activations_read_own
  ON public.webfig_unlock_key_activations
  FOR SELECT TO authenticated
  USING (user_id = auth.uid());

ALTER TABLE public.magic_coin_transactions
  ADD COLUMN IF NOT EXISTS webfig_key_activation_id uuid
  REFERENCES public.webfig_unlock_key_activations(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_webfig_key_once
  ON public.magic_coin_transactions(webfig_key_activation_id)
  WHERE webfig_key_activation_id IS NOT NULL;

DO $$
DECLARE
  v_constraint text;
BEGIN
  SELECT conname INTO v_constraint
  FROM pg_constraint
  WHERE conrelid = 'public.magic_coin_transactions'::regclass
    AND contype = 'c'
    AND pg_get_constraintdef(oid) ILIKE '%kind%'
  LIMIT 1;
  IF v_constraint IS NOT NULL THEN
    EXECUTE format('ALTER TABLE public.magic_coin_transactions DROP CONSTRAINT %I', v_constraint);
  END IF;
END;
$$;

ALTER TABLE public.magic_coin_transactions
  ADD CONSTRAINT magic_coin_transactions_kind_check
  CHECK (kind IN (
    'agent_earn', 'agent_reversal', 'service_payment', 'service_refund',
    'feature_key_purchase', 'manual_credit', 'manual_debit'
  ));

CREATE OR REPLACE FUNCTION public.has_active_webfig_unlock_key()
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT auth.uid() IS NOT NULL AND EXISTS (
    SELECT 1
    FROM public.webfig_unlock_key_activations
    WHERE user_id = auth.uid()
      AND key_id = 1000
      AND expires_at > now()
  );
$$;

CREATE OR REPLACE FUNCTION public.get_webfig_unlock_key()
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_key public.webfig_unlock_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;

  SELECT * INTO v_key
  FROM public.webfig_unlock_key_activations
  WHERE user_id = v_user AND key_id = 1000 AND expires_at > now()
  ORDER BY expires_at DESC
  LIMIT 1;

  RETURN jsonb_build_object(
    'key_id', 1000,
    'name', 'Inv hub misc key',
    'price_coins', 5,
    'valid_hours', 12,
    'attributes', 'Unlock the WebFig',
    'account_bound', true,
    'non_transferable', true,
    'active', v_key.id IS NOT NULL,
    'expires_at', CASE WHEN v_key.id IS NULL THEN NULL ELSE v_key.expires_at END
  );
END;
$$;

CREATE OR REPLACE FUNCTION public.purchase_webfig_unlock_key()
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_balance numeric(12,2);
  v_activation public.webfig_unlock_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;

  -- These account classes are exempt from the product lock and must never be
  -- allowed to spend coins on an unnecessary key.
  IF public.is_platform_admin(v_user)
     OR public.has_role(v_user, 'primary'::public.app_role)
     OR public.has_role(v_user, 'agent'::public.app_role) THEN
    RAISE EXCEPTION 'WEBFIG_KEY_NOT_REQUIRED';
  END IF;

  v_owner := public.effective_owner(v_user);
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance
  FROM public.magic_coin_wallets
  WHERE user_id = v_user
  FOR UPDATE;

  IF EXISTS (
    SELECT 1 FROM public.webfig_unlock_key_activations
    WHERE user_id = v_user AND key_id = 1000 AND expires_at > now()
  ) THEN
    RAISE EXCEPTION 'WEBFIG_KEY_ALREADY_ACTIVE';
  END IF;
  IF v_balance < 5 THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;

  INSERT INTO public.webfig_unlock_key_activations (
    user_id, owner_id, expires_at
  ) VALUES (
    v_user, v_owner, now() + interval '12 hours'
  ) RETURNING * INTO v_activation;

  v_balance := v_balance - 5;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, webfig_key_activation_id, note, actor_user_id
  ) VALUES (
    v_user, v_owner, -5, v_balance, 'feature_key_purchase', v_activation.id,
    'Purchased Inv hub misc key (WebFig unlock, 12 hours, account-bound)', v_user
  );

  RETURN jsonb_build_object(
    'ok', true,
    'key_id', 1000,
    'price_coins', 5,
    'balance', v_balance,
    'expires_at', v_activation.expires_at
  );
END;
$$;

REVOKE ALL ON FUNCTION public.has_active_webfig_unlock_key(), public.get_webfig_unlock_key(), public.purchase_webfig_unlock_key()
  FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.has_active_webfig_unlock_key(), public.get_webfig_unlock_key(), public.purchase_webfig_unlock_key()
  TO authenticated, service_role;
