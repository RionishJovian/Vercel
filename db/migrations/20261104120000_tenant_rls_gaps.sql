-- Audit finding 13. Three tenant tables had no row security, and tenant tables with row
-- security did not force it, so a non-superuser table owner would bypass their policies.
-- The service connection keeps its access through mm_runtime_service; it is created here
-- as well so there is no gap before the runner re-applies deploy/runtime-db-role.sql.
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['business_invitations','business_memberships','nats_subjects'] LOOP
    EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', t || '_tenant_isolation', t);
    EXECUTE format($p$CREATE POLICY %I ON public.%I TO authenticated
      USING (business_id = NULLIF(current_setting('app.current_business_id', true), '')::uuid)
      WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id', true), '')::uuid)$p$, t || '_tenant_isolation', t);
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO authenticated', t);
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mega_magic_runtime') THEN
      EXECUTE format('DROP POLICY IF EXISTS mm_runtime_service ON public.%I', t);
      EXECUTE format('CREATE POLICY mm_runtime_service ON public.%I FOR ALL TO mega_magic_runtime USING (true) WITH CHECK (true)', t);
    END IF;
  END LOOP;

  FOR t IN
    SELECT c.relname FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity AND NOT c.relforcerowsecurity
       AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'business_id' AND NOT a.attisdropped)
  LOOP
    EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', t);
  END LOOP;
END $$;
