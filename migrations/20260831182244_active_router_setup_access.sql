-- Keep the first/default physical router usable for active tenant setup.
-- Router keys remain required only for routers beyond the included allowance.

ALTER TABLE public.device_allowances
  ADD COLUMN IF NOT EXISTS included_router_id uuid
  REFERENCES public.router_connections(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS device_allowances_included_router_idx
  ON public.device_allowances(included_router_id)
  WHERE included_router_id IS NOT NULL;

UPDATE public.device_allowances
SET routers = GREATEST(COALESCE(routers, 1), 1),
    updated_at = now()
WHERE routers IS NULL OR routers < 1;

-- Backfill missing allowance rows and durable first-router identities.
INSERT INTO public.device_allowances (owner_id, routers, included_router_id)
SELECT r.owner_id, 1, r.id
FROM (
  SELECT DISTINCT ON (owner_id) owner_id, id
  FROM public.router_connections
  WHERE owner_id IS NOT NULL
    AND connection_mode IS DISTINCT FROM 'sandbox'
    AND COALESCE(is_virtual, false) = false
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
    AND COALESCE(is_virtual, false) = false
  ORDER BY owner_id, created_at NULLS FIRST, id
) r
WHERE da.owner_id = r.owner_id
  AND da.included_router_id IS NULL;

CREATE OR REPLACE FUNCTION public.router_unlock_key_accessible(_router_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT CASE
    WHEN auth.uid() IS NULL THEN false
    WHEN public.is_platform_admin(auth.uid())
      OR public.has_role(auth.uid(), 'primary'::public.app_role)
      OR public.has_role(auth.uid(), 'agent'::public.app_role) THEN true
    WHEN NOT EXISTS (
      SELECT 1
      FROM public.router_connections target
      WHERE target.id = _router_id
        AND target.owner_id = public.effective_owner(auth.uid())
    ) THEN false
    WHEN EXISTS (
      SELECT 1
      FROM public.device_allowances da
      WHERE da.owner_id = public.effective_owner(auth.uid())
        AND da.included_router_id = _router_id
    ) THEN true
    WHEN EXISTS (
      SELECT 1
      FROM (
        SELECT
          r.id,
          r.owner_id,
          row_number() OVER (
            PARTITION BY r.owner_id
            ORDER BY r.created_at NULLS FIRST, r.id
          ) AS physical_position
        FROM public.router_connections r
        WHERE r.owner_id = public.effective_owner(auth.uid())
          AND r.connection_mode IS DISTINCT FROM 'sandbox'
          AND COALESCE(r.is_virtual, false) = false
      ) ranked
      LEFT JOIN public.device_allowances da ON da.owner_id = ranked.owner_id
      WHERE ranked.id = _router_id
        AND ranked.physical_position <= GREATEST(COALESCE(da.routers, 1), 1)
    ) THEN true
    WHEN NOT EXISTS (
      SELECT 1
      FROM public.router_unlock_key_activations k
      WHERE k.consumed_router_id = _router_id
    ) THEN true
    ELSE EXISTS (
      SELECT 1
      FROM public.router_unlock_key_activations k
      WHERE k.consumed_router_id = _router_id
        AND k.expires_at > now()
    )
  END;
$$;

CREATE OR REPLACE FUNCTION public.get_router_unlock_keys()
RETURNS jsonb
LANGUAGE sql
STABLE
SECURITY DEFINER
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
    SELECT
      x.consumed_router_id AS router_id,
      CASE
        WHEN EXISTS (
          SELECT 1
          FROM public.device_allowances da
          WHERE da.owner_id = public.effective_owner(auth.uid())
            AND da.included_router_id = x.consumed_router_id
        )
        OR EXISTS (
          SELECT 1
          FROM (
            SELECT
              r.id,
              r.owner_id,
              row_number() OVER (
                PARTITION BY r.owner_id
                ORDER BY r.created_at NULLS FIRST, r.id
              ) AS physical_position
            FROM public.router_connections r
            WHERE r.owner_id = public.effective_owner(auth.uid())
              AND r.connection_mode IS DISTINCT FROM 'sandbox'
              AND COALESCE(r.is_virtual, false) = false
          ) ranked
          LEFT JOIN public.device_allowances da ON da.owner_id = ranked.owner_id
          WHERE ranked.id = x.consumed_router_id
            AND ranked.physical_position <= GREATEST(COALESCE(da.routers, 1), 1)
        )
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
  ) OR EXISTS (
    SELECT 1
    FROM (
      SELECT
        r.id,
        r.owner_id,
        row_number() OVER (
          PARTITION BY r.owner_id
          ORDER BY r.created_at NULLS FIRST, r.id
        ) AS physical_position
      FROM public.router_connections r
      WHERE r.owner_id = v_owner
        AND r.connection_mode IS DISTINCT FROM 'sandbox'
        AND COALESCE(r.is_virtual, false) = false
    ) ranked
    LEFT JOIN public.device_allowances da ON da.owner_id = ranked.owner_id
    WHERE ranked.id = _router_id
      AND ranked.physical_position <= GREATEST(COALESCE(da.routers, 1), 1)
  ) THEN
    RAISE EXCEPTION 'BASIC_ROUTER_NOT_LOCKABLE';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM public.router_unlock_key_activations
    WHERE consumed_router_id = _router_id
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

REVOKE ALL ON FUNCTION public.router_unlock_key_accessible(uuid) FROM PUBLIC, anon;
REVOKE ALL ON FUNCTION public.get_router_unlock_keys() FROM PUBLIC, anon;
REVOKE ALL ON FUNCTION public.reactivate_router_with_key(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.router_unlock_key_accessible(uuid) TO authenticated, service_role;
GRANT EXECUTE ON FUNCTION public.get_router_unlock_keys() TO authenticated, service_role;
GRANT EXECUTE ON FUNCTION public.reactivate_router_with_key(uuid) TO authenticated, service_role;
