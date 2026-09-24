-- Router keys apply only beyond the current included physical-router allowance.
-- Historical key activation rows must never lock a router that is now included.
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
    WHEN EXISTS (
      SELECT 1
      FROM (
        SELECT
          r.id,
          r.owner_id,
          row_number() OVER (
            PARTITION BY r.owner_id
            ORDER BY r.created_at, r.id
          ) AS physical_position
        FROM public.router_connections r
        WHERE r.owner_id = (
          SELECT target.owner_id
          FROM public.router_connections target
          WHERE target.id = _router_id
        )
          AND r.connection_mode IS DISTINCT FROM 'sandbox'
          AND COALESCE(r.is_virtual, false) = false
      ) ranked
      LEFT JOIN public.device_allowances da ON da.owner_id = ranked.owner_id
      WHERE ranked.id = _router_id
        AND ranked.physical_position <= COALESCE(da.routers, 1)
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

REVOKE ALL ON FUNCTION public.router_unlock_key_accessible(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.router_unlock_key_accessible(uuid) TO authenticated, service_role;
