-- 1. Tenant-scoped role check
CREATE OR REPLACE FUNCTION public.has_tenant_role(_user_id uuid, _role public.app_role, _owner_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SET search_path TO 'public'
AS $$
  SELECT _owner_id IS NOT NULL
     AND _owner_id = public.effective_owner(_user_id)
     AND public.has_role(_user_id, _role);
$$;

GRANT EXECUTE ON FUNCTION public.has_tenant_role(uuid, public.app_role, uuid) TO authenticated, anon, service_role;

-- 2. Rewrite tenant-data policies so the global role branch becomes tenant-scoped
DO $do$
DECLARE
  r record;
  new_qual text;
  new_check text;
  sql text;
  tables text[] := ARRAY[
    'sites','router_connections','unifi_controllers','payment_orders','payment_bank_accounts',
    'payment_receipts','payment_events','payment_order_audit','hotspot_sessions','voucher_prices',
    'voucher_sales','portal_settings','portal_plans','portal_deploy_audit','device_allowances',
    'device_requests','ai_scan_limits','ai_usage_events','terminal_templates','syslog_tokens',
    'syslog_events','sandbox_routers','sandbox_state','fleet_scan_runs','router_save_audit'
  ];
BEGIN
  FOR r IN
    SELECT p.tablename, p.policyname, p.cmd, p.permissive, p.roles, p.qual, p.with_check
      FROM pg_policies p
     WHERE p.schemaname = 'public'
       AND p.tablename = ANY(tables)
       AND (coalesce(p.qual,'') LIKE '%has_role(auth.uid()%' OR coalesce(p.with_check,'') LIKE '%has_role(auth.uid()%')
  LOOP
    new_qual := replace(replace(coalesce(r.qual,''),
      'has_role(auth.uid(), ''owner''::app_role)', 'has_tenant_role(auth.uid(), ''owner''::app_role, owner_id)'),
      'has_role(auth.uid(), ''admin''::app_role)', 'has_tenant_role(auth.uid(), ''admin''::app_role, owner_id)');
    new_check := replace(replace(coalesce(r.with_check,''),
      'has_role(auth.uid(), ''owner''::app_role)', 'has_tenant_role(auth.uid(), ''owner''::app_role, owner_id)'),
      'has_role(auth.uid(), ''admin''::app_role)', 'has_tenant_role(auth.uid(), ''admin''::app_role, owner_id)');

    EXECUTE format('DROP POLICY %I ON public.%I', r.policyname, r.tablename);

    sql := format('CREATE POLICY %I ON public.%I AS %s FOR %s TO %s',
      r.policyname, r.tablename,
      CASE WHEN r.permissive = 'PERMISSIVE' THEN 'PERMISSIVE' ELSE 'RESTRICTIVE' END,
      r.cmd,
      array_to_string(r.roles, ', '));

    IF r.qual IS NOT NULL THEN
      sql := sql || format(' USING (%s)', new_qual);
    END IF;
    IF r.with_check IS NOT NULL THEN
      sql := sql || format(' WITH CHECK (%s)', new_check);
    END IF;

    EXECUTE sql;
  END LOOP;
END
$do$;

-- 3. Storage: remove cross-tenant backup reads (server uses the service role)
DROP POLICY IF EXISTS "db backups owner admin read" ON storage.objects;

-- 4. Storage: scope payment receipt access to the owning tenant
DROP POLICY IF EXISTS "payment receipts owner read" ON storage.objects;
CREATE POLICY "payment receipts owner read"
ON storage.objects FOR SELECT TO authenticated
USING (
  bucket_id = 'payment-receipts'
  AND (
    (storage.foldername(name))[2] = (auth.uid())::text
    OR (storage.foldername(name))[1] = (public.effective_owner(auth.uid()))::text
  )
);

DROP POLICY IF EXISTS "payment receipts admin delete" ON storage.objects;
CREATE POLICY "payment receipts admin delete"
ON storage.objects FOR DELETE TO authenticated
USING (
  bucket_id = 'payment-receipts'
  AND (storage.foldername(name))[1] = (public.effective_owner(auth.uid()))::text
  AND (
    public.has_role(auth.uid(), 'owner'::public.app_role)
    OR public.has_role(auth.uid(), 'admin'::public.app_role)
  )
);