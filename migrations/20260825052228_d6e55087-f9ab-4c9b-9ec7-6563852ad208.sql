CREATE OR REPLACE FUNCTION private.is_ruijie_privileged(_user_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1 FROM public.user_roles
    WHERE user_id = _user_id AND role::text IN ('primary','admin','dev')
  )
$$;

REVOKE ALL ON FUNCTION private.is_ruijie_privileged(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION private.is_ruijie_privileged(uuid) TO authenticated, service_role;
