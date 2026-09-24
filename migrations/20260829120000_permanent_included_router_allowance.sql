-- Permanently identify the one included customer router.  This is additive:
-- paid unlock activations remain historical records and continue to govern
-- routers beyond the included allowance.
ALTER TABLE public.device_allowances
  ADD COLUMN IF NOT EXISTS included_router_id uuid
  REFERENCES public.router_connections(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS device_allowances_included_router_idx
  ON public.device_allowances(included_router_id)
  WHERE included_router_id IS NOT NULL;

-- Preserve any higher purchased allowance while ensuring every normal
-- customer has the one included slot required by the product policy.
UPDATE public.device_allowances
SET routers = GREATEST(routers, 1),
    updated_at = now()
WHERE routers < 1;

-- Establish a deterministic identity for existing physical routers without
-- changing paid-key history.  The oldest router is the included router.
INSERT INTO public.device_allowances (owner_id, routers, included_router_id)
SELECT r.owner_id, 1, r.id
FROM (
  SELECT DISTINCT ON (owner_id) owner_id, id
  FROM public.router_connections
  WHERE owner_id IS NOT NULL
    AND connection_mode IS DISTINCT FROM 'sandbox'
  ORDER BY owner_id, created_at NULLS FIRST, id
) r
ON CONFLICT (owner_id) DO NOTHING;

UPDATE public.device_allowances da
SET included_router_id = r.id,
    updated_at = now()
FROM (
  SELECT DISTINCT ON (owner_id) owner_id, id
  FROM public.router_connections
  WHERE owner_id IS NOT NULL
    AND connection_mode IS DISTINCT FROM 'sandbox'
  ORDER BY owner_id, created_at NULLS FIRST, id
) r
WHERE da.owner_id = r.owner_id
  AND da.included_router_id IS NULL;

-- Release both types of router binding in the same transaction as deletion.
-- This also protects direct SQL/admin deletions from violating the paired
-- consumed_at/consumed_router_id check constraint.
CREATE OR REPLACE FUNCTION public.release_router_allowances_before_delete()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  UPDATE public.router_unlock_key_activations
  SET consumed_router_id = NULL,
      consumed_at = NULL
  WHERE consumed_router_id = OLD.id;

  UPDATE public.device_allowances
  SET included_router_id = NULL,
      updated_at = now()
  WHERE included_router_id = OLD.id;

  RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS trg_release_router_allowances_before_delete
  ON public.router_connections;
CREATE TRIGGER trg_release_router_allowances_before_delete
BEFORE DELETE ON public.router_connections
FOR EACH ROW EXECUTE FUNCTION public.release_router_allowances_before_delete();

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
    WHEN EXISTS (
      SELECT 1
      FROM public.device_allowances da
      JOIN public.router_connections r
        ON r.id = da.included_router_id
       AND r.owner_id = da.owner_id
      WHERE da.owner_id = public.effective_owner(auth.uid())
        AND da.included_router_id = _router_id
    ) THEN true
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
      CASE WHEN EXISTS (
        SELECT 1 FROM public.device_allowances da
        WHERE da.owner_id = public.effective_owner(auth.uid())
          AND da.included_router_id = x.consumed_router_id
      ) THEN jsonb_build_object('locked', false, 'expires_at', NULL)
      ELSE jsonb_build_object('locked', x.expires_at <= now(), 'expires_at', x.expires_at)
      END AS state
    FROM public.router_unlock_key_activations x
    WHERE x.user_id = auth.uid() AND x.consumed_router_id IS NOT NULL
      AND x.consumed_router_id = k.consumed_router_id
    ORDER BY x.expires_at DESC LIMIT 1
  ) s ON true
  WHERE k.user_id = auth.uid();
$$;

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
  IF EXISTS (
    SELECT 1 FROM public.device_allowances da
    WHERE da.owner_id = v_owner AND da.included_router_id = _router_id
  ) THEN RAISE EXCEPTION 'BASIC_ROUTER_NOT_LOCKABLE'; END IF;
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

-- Reassert the quota trigger with the durable included identity.  The owner
-- advisory lock makes first-router assignment atomic under concurrent creates.
CREATE OR REPLACE FUNCTION public.enforce_device_quota_trigger()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public
AS $$
DECLARE
  _kind text := TG_ARGV[0]; _max integer; _used integer; _uid uuid := auth.uid();
  _connection_mode text; v_key uuid;
BEGIN
  IF NEW.owner_id IS NULL THEN RETURN NEW; END IF;
  IF _kind = 'routers' THEN
    _connection_mode := to_jsonb(NEW)->>'connection_mode';
    IF _connection_mode = 'sandbox' THEN RETURN NEW; END IF;
  END IF;
  IF _uid IS NOT NULL AND (public.is_platform_admin(_uid) OR public.has_role(_uid, 'primary'::public.app_role) OR public.has_role(_uid, 'agent'::public.app_role)) THEN RETURN NEW; END IF;
  PERFORM pg_advisory_xact_lock(hashtext(NEW.owner_id::text || ':' || _kind));
  SELECT CASE _kind WHEN 'routers' THEN da.routers WHEN 'controllers' THEN da.controllers ELSE da.sites END
    INTO _max
  FROM public.device_allowances da
  WHERE da.owner_id = NEW.owner_id
  FOR UPDATE;
  _max := GREATEST(COALESCE(_max, CASE _kind WHEN 'routers' THEN 1 WHEN 'sites' THEN 3 ELSE 15 END), CASE WHEN _kind = 'routers' THEN 1 ELSE 0 END);
  IF _kind = 'routers' THEN
    SELECT count(*) INTO _used FROM public.router_connections WHERE owner_id = NEW.owner_id AND connection_mode IS DISTINCT FROM 'sandbox';
  ELSIF _kind = 'controllers' THEN SELECT count(*) INTO _used FROM public.unifi_controllers WHERE owner_id = NEW.owner_id;
  ELSE SELECT count(*) INTO _used FROM public.sites WHERE owner_id = NEW.owner_id; END IF;
  IF _used >= _max THEN
    IF _kind = 'routers' AND _uid IS NOT NULL THEN
      SELECT id INTO v_key FROM public.router_unlock_key_activations
      WHERE user_id = _uid AND owner_id = NEW.owner_id AND consumed_at IS NULL AND expires_at > now()
      ORDER BY expires_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED;
      IF v_key IS NOT NULL THEN RETURN NEW; END IF;
    END IF;
    RAISE EXCEPTION 'DEVICE_QUOTA_EXCEEDED: this account already uses % of % allowed %.', _used, _max, _kind USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

-- A BEFORE trigger cannot safely reference NEW.id through the foreign key
-- because the row is not visible yet.  Admission is checked above; this
-- AFTER trigger commits the durable included binding or paid-key binding.
CREATE OR REPLACE FUNCTION public.bind_router_allowance_after_insert()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public
AS $$
DECLARE _uid uuid := auth.uid(); _max integer; _used integer; v_included uuid; v_key uuid;
BEGIN
  IF NEW.owner_id IS NULL OR NEW.connection_mode = 'sandbox' THEN RETURN NEW; END IF;
  IF _uid IS NOT NULL AND (public.is_platform_admin(_uid) OR public.has_role(_uid, 'primary'::public.app_role) OR public.has_role(_uid, 'agent'::public.app_role)) THEN RETURN NEW; END IF;
  PERFORM pg_advisory_xact_lock(hashtext(NEW.owner_id::text || ':routers'));
  SELECT COALESCE(da.routers, 1), da.included_router_id INTO _max, v_included
  FROM public.device_allowances da WHERE da.owner_id = NEW.owner_id FOR UPDATE;
  _max := GREATEST(COALESCE(_max, 1), 1);
  SELECT count(*) INTO _used FROM public.router_connections WHERE owner_id = NEW.owner_id AND connection_mode IS DISTINCT FROM 'sandbox';
  IF _used = 1 AND v_included IS NULL THEN
    INSERT INTO public.device_allowances (owner_id, routers, included_router_id)
    VALUES (NEW.owner_id, _max, NEW.id)
    ON CONFLICT (owner_id) DO UPDATE
      SET included_router_id = EXCLUDED.included_router_id, updated_at = now();
    RETURN NEW;
  END IF;
  IF _used > _max THEN
    SELECT id INTO v_key FROM public.router_unlock_key_activations
    WHERE user_id = _uid AND owner_id = NEW.owner_id AND consumed_at IS NULL AND expires_at > now()
    ORDER BY expires_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED;
    IF v_key IS NULL THEN RAISE EXCEPTION 'DEVICE_QUOTA_EXCEEDED: router key binding unavailable.' USING ERRCODE = 'check_violation'; END IF;
    UPDATE public.router_unlock_key_activations
    SET consumed_at = now(), consumed_router_id = NEW.id
    WHERE id = v_key;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_bind_router_allowance_after_insert ON public.router_connections;
CREATE TRIGGER trg_bind_router_allowance_after_insert
AFTER INSERT ON public.router_connections
FOR EACH ROW EXECUTE FUNCTION public.bind_router_allowance_after_insert();
