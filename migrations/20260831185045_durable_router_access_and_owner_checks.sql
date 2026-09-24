-- Make included_router_id the sole source of truth for the free router slot.
-- Keep authenticated checks user-scoped and give CRON_SECRET-protected jobs a
-- service-role-only owner-scoped check instead of treating auth.uid() as a
-- scheduler identity.

CREATE OR REPLACE FUNCTION private.router_unlock_key_accessible_for_owner(
  _router_id uuid,
  _owner_id uuid
)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT CASE
    WHEN _owner_id IS NULL THEN false
    WHEN NOT EXISTS (
      SELECT 1
      FROM public.router_connections r
      WHERE r.id = _router_id
        AND r.owner_id = _owner_id
    ) THEN false
    WHEN EXISTS (
      SELECT 1
      FROM public.device_allowances da
      WHERE da.owner_id = _owner_id
        AND da.included_router_id = _router_id
    ) THEN true
    WHEN NOT EXISTS (
      SELECT 1
      FROM public.router_unlock_key_activations k
      WHERE k.owner_id = _owner_id
        AND k.consumed_router_id = _router_id
    ) THEN true
    ELSE EXISTS (
      SELECT 1
      FROM public.router_unlock_key_activations k
      WHERE k.owner_id = _owner_id
        AND k.consumed_router_id = _router_id
        AND k.expires_at > now()
    )
  END;
$$;

REVOKE ALL ON FUNCTION private.router_unlock_key_accessible_for_owner(uuid, uuid)
  FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION private.router_unlock_key_accessible_for_owner(uuid, uuid)
  TO service_role;

-- This wrapper is intentionally invoker-security and callable only by the
-- service role. The cron routes authenticate CRON_SECRET before invoking it.
CREATE OR REPLACE FUNCTION public.router_unlock_key_accessible_for_owner(
  _router_id uuid,
  _owner_id uuid
)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public, private
AS $$
  SELECT private.router_unlock_key_accessible_for_owner(_router_id, _owner_id);
$$;

REVOKE ALL ON FUNCTION public.router_unlock_key_accessible_for_owner(uuid, uuid)
  FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION public.router_unlock_key_accessible_for_owner(uuid, uuid)
  TO service_role;

-- The authenticated path derives the owner from auth.uid(). Platform admins
-- retain their existing support access, while tenant access resolves only
-- through the durable included identity or a matching paid activation.
CREATE OR REPLACE FUNCTION public.router_unlock_key_accessible(_router_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public, private
AS $$
  SELECT CASE
    WHEN auth.uid() IS NULL THEN false
    WHEN public.is_platform_admin(auth.uid())
      OR public.has_role(auth.uid(), 'primary'::public.app_role)
      OR public.has_role(auth.uid(), 'agent'::public.app_role) THEN true
    ELSE private.router_unlock_key_accessible_for_owner(
      _router_id,
      public.effective_owner(auth.uid())
    )
  END;
$$;

REVOKE ALL ON FUNCTION public.router_unlock_key_accessible(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.router_unlock_key_accessible(uuid) TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.get_router_unlock_keys()
RETURNS jsonb
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public, private
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
    SELECT
      x.consumed_router_id AS router_id,
      CASE
        WHEN public.router_unlock_key_accessible(x.consumed_router_id)
        THEN jsonb_build_object('locked', false, 'expires_at', NULL)
        ELSE jsonb_build_object('locked', x.expires_at <= now(), 'expires_at', x.expires_at)
      END AS state
    FROM public.router_unlock_key_activations x
    WHERE x.user_id = auth.uid()
      AND x.consumed_router_id IS NOT NULL
      AND x.consumed_router_id = k.consumed_router_id
    ORDER BY x.expires_at DESC
    LIMIT 1
  ) s ON true
  WHERE k.user_id = auth.uid();
$$;

REVOKE ALL ON FUNCTION public.get_router_unlock_keys() FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.get_router_unlock_keys() TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.reactivate_router_with_key(_router_id uuid)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_key public.router_unlock_key_activations%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN
    RAISE EXCEPTION 'AUTH_REQUIRED';
  END IF;

  v_owner := public.effective_owner(v_user);

  IF NOT EXISTS (
    SELECT 1
    FROM public.router_connections
    WHERE id = _router_id
      AND owner_id = v_owner
  ) THEN
    RAISE EXCEPTION 'ROUTER_NOT_FOUND';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM public.device_allowances da
    WHERE da.owner_id = v_owner
      AND da.included_router_id = _router_id
  ) THEN
    RAISE EXCEPTION 'BASIC_ROUTER_NOT_LOCKABLE';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM public.router_unlock_key_activations
    WHERE owner_id = v_owner
      AND consumed_router_id = _router_id
  ) THEN
    RAISE EXCEPTION 'BASIC_ROUTER_NOT_LOCKABLE';
  END IF;

  IF public.router_unlock_key_accessible(_router_id) THEN
    RAISE EXCEPTION 'ROUTER_KEY_STILL_ACTIVE';
  END IF;

  SELECT *
  INTO v_key
  FROM public.router_unlock_key_activations
  WHERE user_id = v_user
    AND owner_id = v_owner
    AND consumed_at IS NULL
    AND expires_at > now()
  ORDER BY expires_at ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED;

  IF v_key.id IS NULL THEN
    RAISE EXCEPTION 'ROUTER_KEY_REQUIRED';
  END IF;

  UPDATE public.router_unlock_key_activations
  SET consumed_at = now(),
      consumed_router_id = _router_id
  WHERE id = v_key.id;

  RETURN jsonb_build_object(
    'ok', true,
    'router_id', _router_id,
    'expires_at', v_key.expires_at
  );
END;
$$;

REVOKE ALL ON FUNCTION public.reactivate_router_with_key(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.reactivate_router_with_key(uuid) TO authenticated, service_role;

-- Deleting the included router must immediately promote the oldest remaining
-- physical router for that owner. The advisory lock matches router admission
-- and makes delete/insert races deterministic.
CREATE OR REPLACE FUNCTION public.release_router_allowances_before_delete()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_replacement uuid;
  v_was_included boolean;
  v_updated integer;
BEGIN
  IF OLD.owner_id IS NULL THEN
    RETURN OLD;
  END IF;

  PERFORM pg_advisory_xact_lock(hashtext(OLD.owner_id::text || ':routers'));

  UPDATE public.router_unlock_key_activations
  SET consumed_router_id = NULL,
      consumed_at = NULL
  WHERE consumed_router_id = OLD.id;

  SELECT EXISTS (
    SELECT 1
    FROM public.device_allowances da
    WHERE da.owner_id = OLD.owner_id
      AND da.included_router_id = OLD.id
  )
  INTO v_was_included;

  IF NOT v_was_included THEN
    RETURN OLD;
  END IF;

  SELECT r.id
  INTO v_replacement
  FROM public.router_connections r
  WHERE r.owner_id = OLD.owner_id
    AND r.id <> OLD.id
    AND r.connection_mode IS DISTINCT FROM 'sandbox'
    AND COALESCE(r.is_virtual, false) = false
  ORDER BY r.created_at NULLS FIRST, r.id
  LIMIT 1;

  IF v_replacement IS NULL THEN
    UPDATE public.device_allowances
    SET included_router_id = NULL,
        routers = GREATEST(COALESCE(routers, 1), 1),
        updated_at = now()
    WHERE owner_id = OLD.owner_id
      AND included_router_id = OLD.id;
    RETURN OLD;
  END IF;

  UPDATE public.device_allowances
  SET included_router_id = v_replacement,
      routers = GREATEST(COALESCE(routers, 1), 1),
      updated_at = now()
  WHERE owner_id = OLD.owner_id
    AND included_router_id = OLD.id;
  GET DIAGNOSTICS v_updated = ROW_COUNT;

  IF v_updated = 0 THEN
    INSERT INTO public.device_allowances (owner_id, routers, included_router_id)
    VALUES (OLD.owner_id, 1, v_replacement)
    ON CONFLICT (owner_id) DO UPDATE
      SET included_router_id = EXCLUDED.included_router_id,
          routers = GREATEST(COALESCE(public.device_allowances.routers, 1), 1),
          updated_at = now();
  END IF;

  RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS trg_release_router_allowances_before_delete
  ON public.router_connections;
CREATE TRIGGER trg_release_router_allowances_before_delete
BEFORE DELETE ON public.router_connections
FOR EACH ROW EXECUTE FUNCTION public.release_router_allowances_before_delete();
