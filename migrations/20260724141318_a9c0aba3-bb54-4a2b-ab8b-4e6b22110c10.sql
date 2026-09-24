
-- Ensure users without a client mapping still get a stable effective owner (themselves)
CREATE OR REPLACE FUNCTION private.effective_owner(_user_id uuid)
RETURNS uuid
LANGUAGE sql
STABLE SECURITY DEFINER
SET search_path TO 'public'
AS $$
  SELECT COALESCE(
    CASE WHEN private.has_role(_user_id, 'owner') THEN _user_id END,
    (SELECT owner_id FROM public.user_roles
       WHERE user_id = _user_id AND role = 'client'
       ORDER BY created_at LIMIT 1),
    _user_id
  );
$$;

-- Relax router_connections policies to accept either the caller's own id
-- or their effective owner id, so signed-in accounts can always save.
DROP POLICY IF EXISTS "routers scope manage" ON public.router_connections;
DROP POLICY IF EXISTS "routers scope read" ON public.router_connections;

CREATE POLICY "routers scope read"
  ON public.router_connections FOR SELECT
  TO authenticated
  USING (owner_id = auth.uid() OR owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "routers scope manage"
  ON public.router_connections FOR ALL
  TO authenticated
  USING (owner_id = auth.uid() OR owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = auth.uid() OR owner_id = public.effective_owner(auth.uid()));
