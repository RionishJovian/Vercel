-- Least-privilege hardening for connector audit / rate-limit / allowance tables.

-- 1) connector_bootstrap_audit: reads are tenant-scoped for signed-in users,
--    writes happen only through trusted server code (service role).
REVOKE ALL ON public.connector_bootstrap_audit FROM anon;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON public.connector_bootstrap_audit FROM authenticated;
GRANT SELECT ON public.connector_bootstrap_audit TO authenticated;
GRANT ALL ON public.connector_bootstrap_audit TO service_role;

-- 2) connector_rate_limits: internal counters only, no client access at all.
REVOKE ALL ON public.connector_rate_limits FROM anon;
REVOKE ALL ON public.connector_rate_limits FROM authenticated;
GRANT ALL ON public.connector_rate_limits TO service_role;

-- 3) device_allowances: signed-in tenant staff only, never anonymous.
REVOKE ALL ON public.device_allowances FROM anon;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.device_allowances TO authenticated;
GRANT ALL ON public.device_allowances TO service_role;

-- 4) Tenant boundary: allowance writes must come from a non-expired
--    owner/admin of that exact tenant.
DROP POLICY IF EXISTS "allowances managed by owners" ON public.device_allowances;
CREATE POLICY "allowances managed by owners"
  ON public.device_allowances FOR ALL TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    AND (
      public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
      OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, owner_id)
    )
    AND NOT public.is_expired(auth.uid())
  )
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND (
      public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
      OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, owner_id)
    )
    AND NOT public.is_expired(auth.uid())
  );