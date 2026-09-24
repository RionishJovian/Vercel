-- Helper: is_expired(uuid) returns true when the user has the expired role
CREATE OR REPLACE FUNCTION private.is_expired(_user_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1 FROM public.user_roles
    WHERE user_id = _user_id AND role = 'expired'::public.app_role
  );
$$;

CREATE OR REPLACE FUNCTION public.is_expired(_user_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SET search_path = public
AS $$ SELECT private.is_expired(_user_id); $$;

REVOKE ALL ON FUNCTION public.is_expired(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.is_expired(uuid) TO authenticated, service_role;
REVOKE ALL ON FUNCTION private.is_expired(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION private.is_expired(uuid) TO authenticated, service_role;

-- router_connections
DROP POLICY IF EXISTS "router_connections tenant write" ON public.router_connections;
CREATE POLICY "router_connections tenant write" ON public.router_connections
  FOR ALL TO authenticated
  USING (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  )
  WITH CHECK (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );

-- portal_settings
DROP POLICY IF EXISTS "portal_settings tenant write" ON public.portal_settings;
CREATE POLICY "portal_settings tenant write" ON public.portal_settings
  FOR ALL TO authenticated
  USING (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  )
  WITH CHECK (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );

-- portal_plans
DROP POLICY IF EXISTS "portal_plans tenant write" ON public.portal_plans;
CREATE POLICY "portal_plans tenant write" ON public.portal_plans
  FOR ALL TO authenticated
  USING (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  )
  WITH CHECK (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );

-- terminal_templates
DROP POLICY IF EXISTS "terminal_templates tenant insert" ON public.terminal_templates;
CREATE POLICY "terminal_templates tenant insert" ON public.terminal_templates
  FOR INSERT TO authenticated
  WITH CHECK (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );
DROP POLICY IF EXISTS "terminal_templates tenant update" ON public.terminal_templates;
CREATE POLICY "terminal_templates tenant update" ON public.terminal_templates
  FOR UPDATE TO authenticated
  USING (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  )
  WITH CHECK (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );
DROP POLICY IF EXISTS "terminal_templates tenant delete" ON public.terminal_templates;
CREATE POLICY "terminal_templates tenant delete" ON public.terminal_templates
  FOR DELETE TO authenticated
  USING (
    ((owner_id = auth.uid()) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );

-- portal_deploy_audit (only insert has WITH CHECK)
DROP POLICY IF EXISTS "portal_deploy_audit tenant insert" ON public.portal_deploy_audit;
CREATE POLICY "portal_deploy_audit tenant insert" ON public.portal_deploy_audit
  FOR INSERT TO authenticated
  WITH CHECK (
    (((owner_id = auth.uid()) AND (user_id = auth.uid())) OR has_role(auth.uid(), 'owner'::app_role))
    AND NOT public.is_expired(auth.uid())
  );