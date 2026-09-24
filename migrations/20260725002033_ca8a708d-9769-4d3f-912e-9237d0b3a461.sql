
-- 1) Make every existing user their own tenant
UPDATE public.user_roles SET owner_id = user_id WHERE owner_id IS DISTINCT FROM user_id;

-- 2) New users become self-tenant clients (owner role reserved for super-admin bootstrap)
CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1)));

  INSERT INTO public.user_roles (user_id, role, owner_id)
  VALUES (NEW.id, 'client', NEW.id);

  RETURN NEW;
END;
$$;

-- 3) Rewrite RLS so tenant = auth.uid(); the 'owner' role (super-admin) bypasses.

-- router_connections
DROP POLICY IF EXISTS "routers scope manage" ON public.router_connections;
DROP POLICY IF EXISTS "routers scope read" ON public.router_connections;
CREATE POLICY "router_connections tenant read"
  ON public.router_connections FOR SELECT TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "router_connections tenant write"
  ON public.router_connections FOR ALL TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'))
  WITH CHECK (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));

-- portal_settings
DROP POLICY IF EXISTS "portal owner manage" ON public.portal_settings;
DROP POLICY IF EXISTS "portal read" ON public.portal_settings;
CREATE POLICY "portal_settings tenant read"
  ON public.portal_settings FOR SELECT TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "portal_settings tenant write"
  ON public.portal_settings FOR ALL TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'))
  WITH CHECK (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));

-- portal_plans
DROP POLICY IF EXISTS "plans owner manage" ON public.portal_plans;
DROP POLICY IF EXISTS "plans read" ON public.portal_plans;
CREATE POLICY "portal_plans tenant read"
  ON public.portal_plans FOR SELECT TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "portal_plans tenant write"
  ON public.portal_plans FOR ALL TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'))
  WITH CHECK (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));

-- terminal_templates
DROP POLICY IF EXISTS "owners view own templates" ON public.terminal_templates;
DROP POLICY IF EXISTS "owners insert own templates" ON public.terminal_templates;
DROP POLICY IF EXISTS "owners update own templates" ON public.terminal_templates;
DROP POLICY IF EXISTS "owners delete own templates" ON public.terminal_templates;
CREATE POLICY "terminal_templates tenant read"
  ON public.terminal_templates FOR SELECT TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "terminal_templates tenant insert"
  ON public.terminal_templates FOR INSERT TO authenticated
  WITH CHECK (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "terminal_templates tenant update"
  ON public.terminal_templates FOR UPDATE TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'))
  WITH CHECK (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "terminal_templates tenant delete"
  ON public.terminal_templates FOR DELETE TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));

-- portal_deploy_audit
DROP POLICY IF EXISTS "owners_read_own_deploys" ON public.portal_deploy_audit;
DROP POLICY IF EXISTS "owners_insert_own_deploys" ON public.portal_deploy_audit;
CREATE POLICY "portal_deploy_audit tenant read"
  ON public.portal_deploy_audit FOR SELECT TO authenticated
  USING (owner_id = auth.uid() OR user_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "portal_deploy_audit tenant insert"
  ON public.portal_deploy_audit FOR INSERT TO authenticated
  WITH CHECK ((owner_id = auth.uid() AND user_id = auth.uid()) OR public.has_role(auth.uid(), 'owner'));

-- fleet_scan_runs
DROP POLICY IF EXISTS "Owners can view their fleet scans" ON public.fleet_scan_runs;
CREATE POLICY "fleet_scan_runs tenant read"
  ON public.fleet_scan_runs FOR SELECT TO authenticated
  USING (owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));

-- router_save_audit
DROP POLICY IF EXISTS "audit_select_own" ON public.router_save_audit;
CREATE POLICY "router_save_audit tenant read"
  ON public.router_save_audit FOR SELECT TO authenticated
  USING (user_id = auth.uid() OR owner_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));
