-- Account-bound Router unlock key (item 1003). A key is consumed atomically by
-- the router quota trigger only when the additional router row is committed.
-- The original included router has no activation row and is never affected.

CREATE TABLE IF NOT EXISTS public.router_unlock_key_activations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  key_id integer NOT NULL DEFAULT 1003 CHECK (key_id = 1003),
  key_name text NOT NULL DEFAULT 'Inv router misc key' CHECK (key_name = 'Inv router misc key'),
  price_coins numeric(12,2) NOT NULL DEFAULT 30 CHECK (price_coins = 30),
  attributes text NOT NULL DEFAULT 'Unlock adding the new router' CHECK (attributes = 'Unlock adding the new router'),
  account_bound boolean NOT NULL DEFAULT true CHECK (account_bound = true),
  non_transferable boolean NOT NULL DEFAULT true CHECK (non_transferable = true),
  unique_router_key boolean NOT NULL DEFAULT true CHECK (unique_router_key = true),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  consumed_router_id uuid REFERENCES public.router_connections(id) ON DELETE RESTRICT,
  CHECK (expires_at > created_at),
  CHECK ((consumed_at IS NULL) = (consumed_router_id IS NULL))
);

CREATE INDEX IF NOT EXISTS router_unlock_key_active_lookup_idx
  ON public.router_unlock_key_activations(user_id, expires_at)
  WHERE consumed_at IS NULL;
CREATE INDEX IF NOT EXISTS router_unlock_key_router_history_idx
  ON public.router_unlock_key_activations(consumed_router_id, expires_at DESC)
  WHERE consumed_router_id IS NOT NULL;

ALTER TABLE public.router_unlock_key_activations ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.router_unlock_key_activations FROM PUBLIC, anon, authenticated;
GRANT SELECT ON public.router_unlock_key_activations TO authenticated;
CREATE POLICY router_unlock_key_activations_read_own
  ON public.router_unlock_key_activations FOR SELECT TO authenticated
  USING (user_id = auth.uid());

ALTER TABLE public.magic_coin_transactions
  ADD COLUMN IF NOT EXISTS router_unlock_key_activation_id uuid
  REFERENCES public.router_unlock_key_activations(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_router_unlock_key_once
  ON public.magic_coin_transactions(router_unlock_key_activation_id)
  WHERE router_unlock_key_activation_id IS NOT NULL;

CREATE OR REPLACE FUNCTION public.router_unlock_key_accessible(_router_id uuid)
RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public
AS $$
  SELECT CASE
    WHEN auth.uid() IS NULL THEN false
    WHEN public.is_platform_admin(auth.uid())
      OR public.has_role(auth.uid(), 'primary'::public.app_role)
      OR public.has_role(auth.uid(), 'agent'::public.app_role) THEN true
    WHEN NOT EXISTS (
      SELECT 1 FROM public.router_unlock_key_activations k
      WHERE k.consumed_router_id = _router_id
    ) THEN true
    ELSE EXISTS (
      SELECT 1 FROM public.router_unlock_key_activations k
      WHERE k.consumed_router_id = _router_id AND k.expires_at > now()
    )
  END;
$$;

CREATE OR REPLACE FUNCTION public.get_router_unlock_keys()
RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public
AS $$
  SELECT jsonb_build_object(
    'key_id', 1003,
    'name', 'Inv router misc key',
    'price_coins', 30,
    'valid_days', 30,
    'attributes', 'Unlock adding the new router',
    'account_bound', true,
    'non_transferable', true,
    'unique_router_key', true,
    'active_count', COUNT(*) FILTER (WHERE consumed_at IS NULL AND expires_at > now()),
    'expires_at', MIN(expires_at) FILTER (WHERE consumed_at IS NULL AND expires_at > now()),
    'router_states', COALESCE(
      jsonb_object_agg(s.router_id::text, s.state) FILTER (WHERE s.router_id IS NOT NULL),
      '{}'::jsonb
    )
  )
  FROM public.router_unlock_key_activations k
  LEFT JOIN LATERAL (
    SELECT x.consumed_router_id AS router_id,
      jsonb_build_object('locked', x.expires_at <= now(), 'expires_at', x.expires_at) AS state
    FROM public.router_unlock_key_activations x
    WHERE x.user_id = auth.uid() AND x.consumed_router_id IS NOT NULL
      AND x.consumed_router_id = k.consumed_router_id
    ORDER BY x.expires_at DESC LIMIT 1
  ) s ON true
  WHERE k.user_id = auth.uid();
$$;

CREATE OR REPLACE FUNCTION public.purchase_router_unlock_key()
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid(); v_owner uuid; v_balance numeric(12,2); v_activation public.router_unlock_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  IF public.is_platform_admin(v_user) OR public.has_role(v_user, 'primary'::public.app_role)
     OR public.has_role(v_user, 'agent'::public.app_role) THEN RAISE EXCEPTION 'ROUTER_KEY_NOT_REQUIRED'; END IF;
  IF EXISTS (SELECT 1 FROM public.router_unlock_key_activations WHERE user_id = v_user AND consumed_at IS NULL AND expires_at > now()) THEN
    RAISE EXCEPTION 'ROUTER_KEY_ALREADY_READY';
  END IF;
  v_owner := public.effective_owner(v_user);
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user) ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user FOR UPDATE;
  IF v_balance < 30 THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;
  INSERT INTO public.router_unlock_key_activations (user_id, owner_id, expires_at)
  VALUES (v_user, v_owner, now() + interval '30 days') RETURNING * INTO v_activation;
  v_balance := v_balance - 30;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (user_id, owner_id, delta, balance_after, kind, router_unlock_key_activation_id, note, actor_user_id)
  VALUES (v_user, v_owner, -30, v_balance, 'feature_key_purchase', v_activation.id,
    'Purchased Inv router misc key (unlock one additional router, 30 days, account-bound)', v_user);
  RETURN jsonb_build_object('ok', true, 'key_id', 1003, 'price_coins', 30, 'balance', v_balance, 'expires_at', v_activation.expires_at);
END;
$$;

-- Re-activate only a router that was previously created with a router key.
CREATE OR REPLACE FUNCTION public.reactivate_router_with_key(_router_id uuid)
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = public
AS $$
DECLARE v_user uuid := auth.uid(); v_owner uuid; v_key public.router_unlock_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  v_owner := public.effective_owner(v_user);
  IF NOT EXISTS (SELECT 1 FROM public.router_connections WHERE id = _router_id AND owner_id = v_owner) THEN RAISE EXCEPTION 'ROUTER_NOT_FOUND'; END IF;
  IF NOT EXISTS (SELECT 1 FROM public.router_unlock_key_activations WHERE consumed_router_id = _router_id) THEN RAISE EXCEPTION 'BASIC_ROUTER_NOT_LOCKABLE'; END IF;
  IF public.router_unlock_key_accessible(_router_id) THEN RAISE EXCEPTION 'ROUTER_KEY_STILL_ACTIVE'; END IF;
  SELECT * INTO v_key FROM public.router_unlock_key_activations
  WHERE user_id = v_user AND owner_id = v_owner AND consumed_at IS NULL AND expires_at > now()
  ORDER BY expires_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED;
  IF v_key.id IS NULL THEN RAISE EXCEPTION 'ROUTER_KEY_REQUIRED'; END IF;
  UPDATE public.router_unlock_key_activations SET consumed_at = now(), consumed_router_id = _router_id WHERE id = v_key.id;
  RETURN jsonb_build_object('ok', true, 'router_id', _router_id, 'expires_at', v_key.expires_at);
END;
$$;

-- Quota enforcement remains authoritative. At the normal included limit, an
-- unconsumed router key is atomically attached to NEW.id; an aborted insert
-- rolls the key consumption back with the transaction.
CREATE OR REPLACE FUNCTION public.enforce_device_quota_trigger()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public
AS $$
DECLARE _kind text := TG_ARGV[0]; _max integer; _used integer; _uid uuid := auth.uid(); _connection_mode text; v_key uuid;
BEGIN
  IF NEW.owner_id IS NULL THEN RETURN NEW; END IF;
  IF _kind = 'routers' THEN _connection_mode := to_jsonb(NEW)->>'connection_mode'; IF _connection_mode = 'sandbox' THEN RETURN NEW; END IF; END IF;
  IF _uid IS NOT NULL AND (public.is_platform_admin(_uid) OR public.has_role(_uid, 'primary'::public.app_role) OR public.has_role(_uid, 'agent'::public.app_role)) THEN RETURN NEW; END IF;
  PERFORM pg_advisory_xact_lock(hashtext(NEW.owner_id::text || ':' || _kind));
  SELECT CASE _kind WHEN 'routers' THEN da.routers WHEN 'controllers' THEN da.controllers ELSE da.sites END INTO _max FROM public.device_allowances da WHERE da.owner_id = NEW.owner_id;
  _max := COALESCE(_max, CASE _kind WHEN 'routers' THEN 1 WHEN 'sites' THEN 3 ELSE 15 END);
  IF _kind = 'routers' THEN SELECT count(*) INTO _used FROM public.router_connections WHERE owner_id = NEW.owner_id AND connection_mode IS DISTINCT FROM 'sandbox';
  ELSIF _kind = 'controllers' THEN SELECT count(*) INTO _used FROM public.unifi_controllers WHERE owner_id = NEW.owner_id;
  ELSE SELECT count(*) INTO _used FROM public.sites WHERE owner_id = NEW.owner_id; END IF;
  IF _used >= _max THEN
    IF _kind = 'routers' AND _uid IS NOT NULL THEN
      SELECT id INTO v_key FROM public.router_unlock_key_activations
      WHERE user_id = _uid AND owner_id = NEW.owner_id AND consumed_at IS NULL AND expires_at > now()
      ORDER BY expires_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED;
      IF v_key IS NOT NULL THEN
        UPDATE public.router_unlock_key_activations SET consumed_at = now(), consumed_router_id = NEW.id WHERE id = v_key;
        RETURN NEW;
      END IF;
    END IF;
    RAISE EXCEPTION 'DEVICE_QUOTA_EXCEEDED: this account already uses % of % allowed %.', _used, _max, _kind USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

REVOKE ALL ON FUNCTION public.router_unlock_key_accessible(uuid), public.get_router_unlock_keys(), public.purchase_router_unlock_key(), public.reactivate_router_with_key(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.router_unlock_key_accessible(uuid), public.get_router_unlock_keys(), public.purchase_router_unlock_key(), public.reactivate_router_with_key(uuid) TO authenticated, service_role;
