-- Magic Dude is a paid, account-bound support unlock for otherwise locked roles.
-- Purchase and access checks are server-side so a client cannot grant itself access.

CREATE TABLE IF NOT EXISTS public.magic_dude_unlock_activations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  price_coins numeric(12,2) NOT NULL DEFAULT 5 CHECK (price_coins = 5),
  attributes text NOT NULL DEFAULT 'Unlock Magic Dude' CHECK (attributes = 'Unlock Magic Dude'),
  account_bound boolean NOT NULL DEFAULT true CHECK (account_bound = true),
  non_transferable boolean NOT NULL DEFAULT true CHECK (non_transferable = true),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  CHECK (expires_at = created_at + interval '30 days')
);

CREATE INDEX IF NOT EXISTS magic_dude_unlock_active_lookup_idx
  ON public.magic_dude_unlock_activations(user_id, expires_at DESC);

ALTER TABLE public.magic_dude_unlock_activations ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.magic_dude_unlock_activations FROM PUBLIC, anon, authenticated;
GRANT SELECT ON public.magic_dude_unlock_activations TO authenticated;
CREATE POLICY magic_dude_unlock_activations_read_own
  ON public.magic_dude_unlock_activations
  FOR SELECT TO authenticated USING (user_id = auth.uid());

ALTER TABLE public.magic_coin_transactions
  ADD COLUMN IF NOT EXISTS magic_dude_unlock_activation_id uuid
  REFERENCES public.magic_dude_unlock_activations(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_magic_dude_unlock_once
  ON public.magic_coin_transactions(magic_dude_unlock_activation_id)
  WHERE magic_dude_unlock_activation_id IS NOT NULL;

CREATE OR REPLACE FUNCTION public.has_active_magic_dude_unlock()
RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
  SELECT auth.uid() IS NOT NULL AND EXISTS (
    SELECT 1 FROM public.magic_dude_unlock_activations
    WHERE user_id = auth.uid() AND expires_at > now()
  );
$$;

CREATE OR REPLACE FUNCTION public.get_magic_dude_unlock()
RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
  SELECT jsonb_build_object(
    'price_coins', 5,
    'valid_days', 30,
    'attributes', 'Unlock Magic Dude',
    'account_bound', true,
    'non_transferable', true,
    'active', EXISTS (
      SELECT 1 FROM public.magic_dude_unlock_activations
      WHERE user_id = auth.uid() AND expires_at > now()
    ),
    'expires_at', (
      SELECT expires_at FROM public.magic_dude_unlock_activations
      WHERE user_id = auth.uid() AND expires_at > now()
      ORDER BY expires_at DESC LIMIT 1
    )
  );
$$;

CREATE OR REPLACE FUNCTION public.purchase_magic_dude_unlock()
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_balance numeric(12,2);
  v_activation public.magic_dude_unlock_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  IF public.is_platform_admin(v_user)
     OR public.has_role(v_user, 'primary'::public.app_role)
     OR public.has_role(v_user, 'agent'::public.app_role) THEN
    RAISE EXCEPTION 'MAGIC_DUDE_UNLOCK_NOT_REQUIRED';
  END IF;

  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user FOR UPDATE;
  IF v_balance < 5 THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;
  IF EXISTS (
    SELECT 1 FROM public.magic_dude_unlock_activations
    WHERE user_id = v_user AND expires_at > now()
  ) THEN RAISE EXCEPTION 'MAGIC_DUDE_UNLOCK_ALREADY_ACTIVE'; END IF;

  v_owner := public.effective_owner(v_user);
  INSERT INTO public.magic_dude_unlock_activations (user_id, owner_id, expires_at)
  VALUES (v_user, v_owner, now() + interval '30 days') RETURNING * INTO v_activation;

  v_balance := v_balance - 5;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, magic_dude_unlock_activation_id, note, actor_user_id
  ) VALUES (
    v_user, v_owner, -5, v_balance, 'feature_key_purchase', v_activation.id,
    'Unlocked Magic Dude for 30 days (account-bound, non-transferable)', v_user
  );

  RETURN jsonb_build_object('ok', true, 'price_coins', 5, 'balance', v_balance, 'expires_at', v_activation.expires_at);
END;
$$;

REVOKE ALL ON FUNCTION public.has_active_magic_dude_unlock(), public.get_magic_dude_unlock(), public.purchase_magic_dude_unlock() FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.has_active_magic_dude_unlock(), public.get_magic_dude_unlock(), public.purchase_magic_dude_unlock() TO authenticated, service_role;
