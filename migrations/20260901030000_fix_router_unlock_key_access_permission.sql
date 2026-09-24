-- The authenticated router-access wrapper derives the owner from auth.uid().
-- It must be able to call the private owner-scoped helper without exposing
-- that helper (which accepts an arbitrary owner id) to browser roles.
CREATE OR REPLACE FUNCTION public.router_unlock_key_accessible(_router_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
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
GRANT EXECUTE ON FUNCTION public.router_unlock_key_accessible(uuid)
  TO authenticated, service_role;
