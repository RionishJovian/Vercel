DROP POLICY IF EXISTS "routers owner manage" ON public.router_connections;
CREATE POLICY "routers scope manage" ON public.router_connections
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));