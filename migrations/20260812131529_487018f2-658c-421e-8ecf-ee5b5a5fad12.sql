DROP POLICY IF EXISTS "owner_accounts self read" ON public.owner_accounts;
CREATE POLICY "Owners can view owner_accounts"
  ON public.owner_accounts FOR SELECT
  TO authenticated
  USING (public.has_role(auth.uid(), 'owner'));

DROP POLICY IF EXISTS "roles owner manage" ON public.user_roles;
CREATE POLICY "roles owner manage" ON public.user_roles
  FOR ALL TO authenticated
  USING (public.has_role(auth.uid(), 'owner'))
  WITH CHECK (public.has_role(auth.uid(), 'owner'));

DROP POLICY IF EXISTS "roles self read" ON public.user_roles;
CREATE POLICY "roles self read" ON public.user_roles
  FOR SELECT TO authenticated
  USING (user_id = auth.uid() OR public.has_role(auth.uid(), 'owner'));