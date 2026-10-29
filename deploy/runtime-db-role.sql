-- Explicit operator-run provisioning; never an automatic app migration.
-- Keep the database owner/migration login separate from this serving login.
-- The base service pool needs cross-tenant DML for sign-in, platform accounts
-- and workers. Express that access through explicit table grants/RLS policies,
-- not SUPERUSER/BYPASSRLS. Tenant transactions SET ROLE authenticated and lose
-- this service policy. New schema migrations must rerun this provisioning.
BEGIN;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='mega_magic_runtime') THEN
  CREATE ROLE mega_magic_runtime NOLOGIN;
 END IF;
END $$;
ALTER ROLE mega_magic_runtime NOSUPERUSER NOCREATEDB NOCREATEROLE
 NOREPLICATION NOBYPASSRLS NOINHERIT;
GRANT authenticated TO mega_magic_runtime;
GRANT USAGE ON SCHEMA public TO mega_magic_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO mega_magic_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO mega_magic_runtime;
DO $$ DECLARE t record; BEGIN
 EXECUTE format('GRANT CONNECT ON DATABASE %I TO mega_magic_runtime',current_database());
 FOR t IN SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname='public' AND c.relkind IN ('r','p') AND c.relrowsecurity
 LOOP
  EXECUTE format('DROP POLICY IF EXISTS mm_runtime_service ON public.%I',t.relname);
  EXECUTE format('CREATE POLICY mm_runtime_service ON public.%I FOR ALL TO mega_magic_runtime USING (true) WITH CHECK (true)',t.relname);
 END LOOP;
 IF has_schema_privilege('mega_magic_runtime','public','CREATE') THEN
  RAISE EXCEPTION 'runtime must not inherit CREATE on the public schema';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.roleid
   JOIN pg_roles u ON u.oid=m.member WHERE u.rolname='mega_magic_runtime' AND r.rolname<>'authenticated') THEN
  RAISE EXCEPTION 'unexpected runtime role membership';
 END IF;
END $$;
-- Set a generated password and LOGIN separately through protected operator I/O.
COMMIT;
