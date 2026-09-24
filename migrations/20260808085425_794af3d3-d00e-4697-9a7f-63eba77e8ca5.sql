DROP POLICY IF EXISTS "allowances managed by owners" ON public.device_allowances;
CREATE POLICY "allowances managed by owners" ON public.device_allowances
  FOR ALL TO authenticated
  USING (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin'))
  WITH CHECK (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin'));

DROP POLICY IF EXISTS "allowances readable in scope" ON public.device_allowances;
CREATE POLICY "allowances readable in scope" ON public.device_allowances
  FOR SELECT TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    OR public.has_role(auth.uid(), 'owner')
    OR public.has_role(auth.uid(), 'admin')
  );

DROP POLICY IF EXISTS "requests decided by owners" ON public.device_requests;
CREATE POLICY "requests decided by owners" ON public.device_requests
  FOR UPDATE TO authenticated
  USING (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin'))
  WITH CHECK (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin'));

DROP POLICY IF EXISTS "requests readable in scope" ON public.device_requests;
CREATE POLICY "requests readable in scope" ON public.device_requests
  FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
    OR public.has_role(auth.uid(), 'owner')
    OR public.has_role(auth.uid(), 'admin')
  );