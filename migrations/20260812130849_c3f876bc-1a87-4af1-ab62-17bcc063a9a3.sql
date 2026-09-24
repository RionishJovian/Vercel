DROP POLICY IF EXISTS "Owners can view owner_accounts" ON public.owner_accounts;
CREATE POLICY "owner_accounts self read" ON public.owner_accounts
  FOR SELECT TO authenticated
  USING (user_id = auth.uid() OR user_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid()));

DROP POLICY IF EXISTS "roles owner manage" ON public.user_roles;
CREATE POLICY "roles owner manage" ON public.user_roles
  FOR ALL TO authenticated
  USING (public.has_tenant_role(auth.uid(), 'owner'::app_role, owner_id) OR public.is_platform_admin(auth.uid()))
  WITH CHECK (public.has_tenant_role(auth.uid(), 'owner'::app_role, owner_id) OR public.is_platform_admin(auth.uid()));

DROP POLICY IF EXISTS "roles self read" ON public.user_roles;
CREATE POLICY "roles self read" ON public.user_roles
  FOR SELECT TO authenticated
  USING (user_id = auth.uid() OR public.has_tenant_role(auth.uid(), 'owner'::app_role, owner_id) OR public.is_platform_admin(auth.uid()));