-- Account-bound Magic Switch own-peer unlock key (item 1004).
-- Unlocks connecting a MikroTik dual-boot (RouterOS) CRS switch as its OWN
-- Magic Hub peer (direct dial-in) for this account. Router-relayed switches
-- (behind a hub-connected router) remain free and unaffected.
-- 20 Magic Coins, valid 30 days, per-account, non-transferable.

CREATE TABLE IF NOT EXISTS public.switch_unlock_key_activations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  key_id integer NOT NULL DEFAULT 1004 CHECK (key_id = 1004),
  key_name text NOT NULL DEFAULT 'Magic Switch own-peer key' CHECK (key_name = 'Magic Switch own-peer key'),
  price_coins numeric(12,2) NOT NULL DEFAULT 20 CHECK (price_coins = 20),
  attributes text NOT NULL DEFAULT 'Unlock adding a CRS switch as its own Magic Hub peer' CHECK (attributes = 'Unlock adding a CRS switch as its own Magic Hub peer'),
  account_bound boolean NOT NULL DEFAULT true CHECK (account_bound = true),
  non_transferable boolean NOT NULL DEFAULT true CHECK (non_transferable = true),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  consumed_switch_id uuid,
  CHECK (expires_at > created_at),
  CHECK ((consumed_at IS NULL) = (consumed_switch_id IS NULL))
);

CREATE INDEX IF NOT EXISTS switch_unlock_key_active_lookup_idx
  ON public.switch_unlock_key_activations(user_id, expires_at)
  WHERE consumed_at IS NULL;

ALTER TABLE public.switch_unlock_key_activations ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.switch_unlock_key_activations FROM PUBLIC, anon, authenticated;
GRANT SELECT ON public.switch_unlock_key_activations TO authenticated;
CREATE POLICY switch_unlock_key_activations_read_own
  ON public.switch_unlock_key_activations FOR SELECT TO authenticated
  USING (user_id = auth.uid());

ALTER TABLE public.magic_coin_transactions
  ADD COLUMN IF NOT EXISTS switch_unlock_key_activation_id uuid
  REFERENCES public.switch_unlock_key_activations(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_switch_unlock_key_once
  ON public.magic_coin_transactions(switch_unlock_key_activation_id)
  WHERE switch_unlock_key_activation_id IS NOT NULL;

-- True when this account may use own-peer (direct Magic Hub) CRS switches.
CREATE OR REPLACE FUNCTION public.switch_unlock_key_active()
RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1 FROM public.switch_unlock_key_activations k
    WHERE k.user_id = auth.uid()
      AND k.consumed_at IS NULL
      AND k.expires_at > now()
  );
$$;

CREATE OR REPLACE FUNCTION public.get_switch_unlock_key()
RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public
AS $$
  SELECT jsonb_build_object(
    'key_id', 1004,
    'name', 'Magic Switch own-peer key',
    'price_coins', 20,
    'valid_days', 30,
    'attributes', 'Unlock adding a CRS switch as its own Magic Hub peer',
    'account_bound', true,
    'non_transferable', true,
    'active', k.expires_at IS NOT NULL,
    'expires_at', k.expires_at
  )
  FROM (SELECT 1 AS one) b
  LEFT JOIN LATERAL (
    SELECT expires_at FROM public.switch_unlock_key_activations k2
    WHERE k2.user_id = auth.uid() AND k2.consumed_at IS NULL AND k2.expires_at > now()
    ORDER BY k2.expires_at DESC LIMIT 1
  ) k ON true;
$$;

CREATE OR REPLACE FUNCTION public.purchase_switch_unlock_key()
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_balance numeric(12,2);
  v_activation public.switch_unlock_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  IF public.is_platform_admin(v_user) OR public.has_role(v_user, 'primary'::public.app_role)
     OR public.has_role(v_user, 'agent'::public.app_role) THEN
    RAISE EXCEPTION 'SWITCH_KEY_NOT_REQUIRED';
  END IF;
  IF EXISTS (SELECT 1 FROM public.switch_unlock_key_activations WHERE user_id = v_user AND consumed_at IS NULL AND expires_at > now()) THEN
    RAISE EXCEPTION 'SWITCH_KEY_ALREADY_READY';
  END IF;
  v_owner := public.effective_owner(v_user);
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user) ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user FOR UPDATE;
  IF v_balance < 20 THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;
  INSERT INTO public.switch_unlock_key_activations (user_id, owner_id, expires_at)
  VALUES (v_user, v_owner, now() + interval '30 days') RETURNING * INTO v_activation;
  v_balance := v_balance - 20;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (user_id, owner_id, delta, balance_after, kind, switch_unlock_key_activation_id, note, actor_user_id)
  VALUES (v_user, v_owner, -20, v_balance, 'feature_key_purchase', v_activation.id,
    'Purchased Magic Switch own-peer key (unlock own-peer CRS, 30 days, account-bound)', v_user);
  RETURN jsonb_build_object('ok', true, 'key_id', 1004, 'price_coins', 20, 'balance', v_balance, 'expires_at', v_activation.expires_at);
END;
$$;

GRANT EXECUTE ON FUNCTION public.purchase_switch_unlock_key() TO authenticated;
GRANT EXECUTE ON FUNCTION public.get_switch_unlock_key() TO authenticated;
GRANT EXECUTE ON FUNCTION public.switch_unlock_key_active() TO authenticated;