-- Fix CRITICAL tenant BOLA: can_manage_router_tenant treated every café
-- owner/admin as global. Cross-tenant router access is limited to
-- platform_admins; same-tenant members still use effective_owner + not expired.
-- tunnel_hubs already uses is_platform_admin (20260815180000) — leave alone.

CREATE OR REPLACE FUNCTION private.can_manage_router_tenant(_user_id uuid, _tenant_owner_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path TO 'public'
AS $function$
  SELECT
    private.is_platform_admin(_user_id)
    OR (
      _tenant_owner_id IS NOT NULL
      AND public.effective_owner(_user_id) = _tenant_owner_id
      AND NOT public.is_expired(_user_id)
    );
$function$;

REVOKE ALL ON FUNCTION private.can_manage_router_tenant(uuid, uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION private.can_manage_router_tenant(uuid, uuid) TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.can_manage_router_tenant(_user_id uuid, _tenant_owner_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path TO 'public'
AS $function$
  SELECT private.can_manage_router_tenant(_user_id, _tenant_owner_id);
$function$;

REVOKE ALL ON FUNCTION public.can_manage_router_tenant(uuid, uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.can_manage_router_tenant(uuid, uuid) TO authenticated, service_role;
