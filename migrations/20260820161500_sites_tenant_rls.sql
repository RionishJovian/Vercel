-- Sites RLS: tenant-scoped via effective_owner (fixes cross-owner leak + staff blind spot).
-- Aligns with router_connections / voucher policies.

DROP POLICY IF EXISTS "sites_select_own_or_owner" ON public.sites;
DROP POLICY IF EXISTS "sites_insert_own" ON public.sites;
DROP POLICY IF EXISTS "sites_update_own_or_owner" ON public.sites;
DROP POLICY IF EXISTS "sites_delete_own_or_owner" ON public.sites;
DROP POLICY IF EXISTS "sites tenant select" ON public.sites;
DROP POLICY IF EXISTS "sites tenant insert" ON public.sites;
DROP POLICY IF EXISTS "sites tenant update" ON public.sites;
DROP POLICY IF EXISTS "sites tenant delete" ON public.sites;

CREATE POLICY "sites tenant select" ON public.sites
  FOR SELECT TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    OR public.is_platform_admin(auth.uid())
  );

CREATE POLICY "sites tenant insert" ON public.sites
  FOR INSERT TO authenticated
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.has_role(auth.uid(), 'read_only'::public.app_role)
    AND NOT public.has_role(auth.uid(), 'expired'::public.app_role)
    AND NOT public.has_role(auth.uid(), 'pending'::public.app_role)
  );

CREATE POLICY "sites tenant update" ON public.sites
  FOR UPDATE TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    OR public.is_platform_admin(auth.uid())
  )
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.has_role(auth.uid(), 'read_only'::public.app_role)
    AND NOT public.has_role(auth.uid(), 'expired'::public.app_role)
  );

CREATE POLICY "sites tenant delete" ON public.sites
  FOR DELETE TO authenticated
  USING (
    (
      owner_id = public.effective_owner(auth.uid())
      OR public.is_platform_admin(auth.uid())
    )
    AND NOT public.has_role(auth.uid(), 'read_only'::public.app_role)
    AND NOT public.has_role(auth.uid(), 'expired'::public.app_role)
  );
