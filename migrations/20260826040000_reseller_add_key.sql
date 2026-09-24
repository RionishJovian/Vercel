-- Account-bound, consumable Add reseller key (item 1001).
-- A 5 Magic Coin purchase creates one 30-day key. Creating a reseller consumes
-- exactly one unexpired key in the same transaction, so a client cannot reuse,
-- transfer, or bypass it through PostgREST.

CREATE TABLE IF NOT EXISTS public.reseller_add_key_activations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  key_id integer NOT NULL DEFAULT 1001 CHECK (key_id = 1001),
  key_name text NOT NULL DEFAULT 'Inv reseller misc key' CHECK (key_name = 'Inv reseller misc key'),
  price_coins numeric(12,2) NOT NULL DEFAULT 5 CHECK (price_coins = 5),
  attributes text NOT NULL DEFAULT 'Unlock the add reseller x1' CHECK (attributes = 'Unlock the add reseller x1'),
  account_bound boolean NOT NULL DEFAULT true CHECK (account_bound = true),
  non_transferable boolean NOT NULL DEFAULT true CHECK (non_transferable = true),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  consumed_reseller_id uuid REFERENCES public.voucher_resellers(id) ON DELETE RESTRICT,
  CHECK (expires_at > created_at),
  CHECK ((consumed_at IS NULL) = (consumed_reseller_id IS NULL))
);

CREATE INDEX IF NOT EXISTS reseller_add_key_active_lookup_idx
  ON public.reseller_add_key_activations(user_id, expires_at)
  WHERE consumed_at IS NULL;

ALTER TABLE public.reseller_add_key_activations ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.reseller_add_key_activations FROM PUBLIC, anon, authenticated;
GRANT SELECT ON public.reseller_add_key_activations TO authenticated;
CREATE POLICY reseller_add_key_activations_read_own
  ON public.reseller_add_key_activations
  FOR SELECT TO authenticated
  USING (user_id = auth.uid());

ALTER TABLE public.magic_coin_transactions
  ADD COLUMN IF NOT EXISTS reseller_add_key_activation_id uuid
  REFERENCES public.reseller_add_key_activations(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_reseller_add_key_once
  ON public.magic_coin_transactions(reseller_add_key_activation_id)
  WHERE reseller_add_key_activation_id IS NOT NULL;

CREATE OR REPLACE FUNCTION public.get_reseller_add_keys()
RETURNS jsonb
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT jsonb_build_object(
    'key_id', 1001,
    'name', 'Inv reseller misc key',
    'price_coins', 5,
    'valid_days', 30,
    'attributes', 'Unlock the add reseller x1',
    'account_bound', true,
    'non_transferable', true,
    'active_count', COUNT(*) FILTER (WHERE consumed_at IS NULL AND expires_at > now()),
    'expires_at', MIN(expires_at) FILTER (WHERE consumed_at IS NULL AND expires_at > now())
  )
  FROM public.reseller_add_key_activations
  WHERE user_id = auth.uid();
$$;

CREATE OR REPLACE FUNCTION public.purchase_reseller_add_key()
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_balance numeric(12,2);
  v_activation public.reseller_add_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  IF public.is_platform_admin(v_user)
     OR public.has_role(v_user, 'primary'::public.app_role)
     OR public.has_role(v_user, 'agent'::public.app_role) THEN
    RAISE EXCEPTION 'RESELLER_ADD_KEY_NOT_REQUIRED';
  END IF;

  v_owner := public.effective_owner(v_user);
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user FOR UPDATE;
  IF v_balance < 5 THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;

  INSERT INTO public.reseller_add_key_activations (user_id, owner_id, expires_at)
  VALUES (v_user, v_owner, now() + interval '30 days')
  RETURNING * INTO v_activation;

  v_balance := v_balance - 5;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, reseller_add_key_activation_id, note, actor_user_id
  ) VALUES (
    v_user, v_owner, -5, v_balance, 'feature_key_purchase', v_activation.id,
    'Purchased Inv reseller misc key (Add reseller x1, 30 days, account-bound)', v_user
  );

  RETURN jsonb_build_object(
    'ok', true, 'key_id', 1001, 'price_coins', 5, 'balance', v_balance,
    'expires_at', v_activation.expires_at
  );
END;
$$;

CREATE OR REPLACE FUNCTION public.create_reseller_with_key(
  _shop_name text,
  _contact_name text,
  _location text DEFAULT NULL,
  _phone text DEFAULT NULL
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_key public.reseller_add_key_activations%ROWTYPE;
  v_reseller_id uuid;
  v_privileged boolean;
  v_keys_remaining integer;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  IF public.owner_operations_is_trial_account(v_user) THEN
    RAISE EXCEPTION 'RESELLER_OPERATION_TRIAL_LOCKED';
  END IF;
  IF char_length(btrim(COALESCE(_shop_name, ''))) NOT BETWEEN 1 AND 120
     OR char_length(btrim(COALESCE(_contact_name, ''))) NOT BETWEEN 1 AND 120 THEN
    RAISE EXCEPTION 'RESELLER_NAME_REQUIRED';
  END IF;
  IF char_length(COALESCE(_location, '')) > 240 OR char_length(COALESCE(_phone, '')) > 60 THEN
    RAISE EXCEPTION 'RESELLER_FIELD_TOO_LONG';
  END IF;

  v_owner := public.effective_owner(v_user);
  v_privileged := public.is_platform_admin(v_user)
    OR public.has_role(v_user, 'primary'::public.app_role)
    OR public.has_role(v_user, 'agent'::public.app_role);

  IF NOT v_privileged THEN
    SELECT * INTO v_key
    FROM public.reseller_add_key_activations
    WHERE user_id = v_user AND key_id = 1001 AND consumed_at IS NULL AND expires_at > now()
    ORDER BY expires_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED;
    IF v_key.id IS NULL THEN RAISE EXCEPTION 'RESELLER_ADD_KEY_REQUIRED'; END IF;
  END IF;

  INSERT INTO public.voucher_resellers (owner_id, shop_name, contact_name, location, phone)
  VALUES (
    v_owner, btrim(_shop_name), btrim(_contact_name), NULLIF(btrim(_location), ''), NULLIF(btrim(_phone), '')
  )
  RETURNING id INTO v_reseller_id;

  IF NOT v_privileged THEN
    UPDATE public.reseller_add_key_activations
    SET consumed_at = now(), consumed_reseller_id = v_reseller_id
    WHERE id = v_key.id;
  END IF;

  SELECT COUNT(*) INTO v_keys_remaining
  FROM public.reseller_add_key_activations
  WHERE user_id = v_user AND consumed_at IS NULL AND expires_at > now();

  RETURN jsonb_build_object('id', v_reseller_id, 'keys_remaining', v_keys_remaining);
END;
$$;

REVOKE ALL ON FUNCTION public.get_reseller_add_keys(), public.purchase_reseller_add_key(),
  public.create_reseller_with_key(text, text, text, text) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.get_reseller_add_keys(), public.purchase_reseller_add_key(),
  public.create_reseller_with_key(text, text, text, text) TO authenticated, service_role;
