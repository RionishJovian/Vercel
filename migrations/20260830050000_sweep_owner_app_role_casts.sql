-- CRITICAL: after app_role.owner → primary, any leftover
--   'owner'::app_role
-- cast throws 22P02 and blocks Primary/Developer reads & writes.
--
-- Idempotent. Paste twin: .lovable/sql/fix-owner-app-role-rls-sweep.sql
-- in Lovable Cloud SQL Editor, then republish.

-- 1) Enum rename (no-op when already primary).
DO $enum$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_enum e
    JOIN pg_type t ON t.oid = e.enumtypid
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE n.nspname = 'public' AND t.typname = 'app_role' AND e.enumlabel = 'owner'
  ) AND NOT EXISTS (
    SELECT 1 FROM pg_enum e
    JOIN pg_type t ON t.oid = e.enumtypid
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE n.nspname = 'public' AND t.typname = 'app_role' AND e.enumlabel = 'primary'
  ) THEN
    ALTER TYPE public.app_role RENAME VALUE 'owner' TO 'primary';
  END IF;
END
$enum$;

CREATE SCHEMA IF NOT EXISTS private;

-- Helper: rewrite SQL that still casts the dead enum label.
CREATE OR REPLACE FUNCTION private.rewrite_owner_app_role_expr(expr text)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
AS $fn$
DECLARE
  out text := expr;
BEGIN
  IF out IS NULL THEN
    RETURN NULL;
  END IF;
  out := replace(out, '''owner''::public.app_role', '''primary''::public.app_role');
  out := replace(out, '''owner''::app_role', '''primary''::app_role');
  out := replace(out, 'has_role(auth.uid(), ''owner'')', 'has_role(auth.uid(), ''primary'')');
  out := replace(out, 'has_role(_user_id, ''owner'')', 'has_role(_user_id, ''primary'')');
  out := replace(out, 'has_role(_uid, ''owner'')', 'has_role(_uid, ''primary'')');
  out := replace(out, 'has_tenant_role(auth.uid(), ''owner''', 'has_tenant_role(auth.uid(), ''primary''');
  out := replace(out, 'NEW.role = ''owner''::', 'NEW.role = ''primary''::');
  out := replace(out, 'role = ''owner''::public.app_role', 'role = ''primary''::public.app_role');
  out := replace(out, 'role = ''owner''::app_role', 'role = ''primary''::app_role');
  RETURN out;
END;
$fn$;

-- 2) Rewrite RLS policies in public + storage that still cast 'owner'.
DO $policies$
DECLARE
  pol record;
  new_qual text;
  new_check text;
  role_list text;
  create_sql text;
  needs_rewrite boolean;
BEGIN
  FOR pol IN
    SELECT schemaname, tablename, policyname, permissive, roles, cmd, qual, with_check
    FROM pg_policies
    WHERE schemaname IN ('public', 'storage')
      AND (
        coalesce(qual, '') LIKE '%''owner''%'
        OR coalesce(with_check, '') LIKE '%''owner''%'
      )
  LOOP
    needs_rewrite :=
      coalesce(pol.qual, '') LIKE '%''owner''::%'
      OR coalesce(pol.with_check, '') LIKE '%''owner''::%'
      OR coalesce(pol.qual, '') LIKE '%has_role(%''owner''%'
      OR coalesce(pol.with_check, '') LIKE '%has_role(%''owner''%'
      OR coalesce(pol.qual, '') LIKE '%has_tenant_role(%''owner''%'
      OR coalesce(pol.with_check, '') LIKE '%has_tenant_role(%''owner''%'
      OR coalesce(pol.qual, '') LIKE '%role = ''owner''::%'
      OR coalesce(pol.with_check, '') LIKE '%role = ''owner''::%';
    IF NOT needs_rewrite THEN
      CONTINUE;
    END IF;

    new_qual := private.rewrite_owner_app_role_expr(pol.qual);
    new_check := private.rewrite_owner_app_role_expr(pol.with_check);

    role_list := array_to_string(
      ARRAY(SELECT format('%I', r) FROM unnest(pol.roles) AS r),
      ', '
    );

    EXECUTE format(
      'DROP POLICY IF EXISTS %I ON %I.%I',
      pol.policyname, pol.schemaname, pol.tablename
    );

    create_sql := format(
      'CREATE POLICY %I ON %I.%I AS %s FOR %s TO %s',
      pol.policyname,
      pol.schemaname,
      pol.tablename,
      CASE WHEN pol.permissive = 'PERMISSIVE' THEN 'PERMISSIVE' ELSE 'RESTRICTIVE' END,
      pol.cmd,
      role_list
    );
    IF new_qual IS NOT NULL THEN
      create_sql := create_sql || ' USING (' || new_qual || ')';
    END IF;
    IF new_check IS NOT NULL THEN
      create_sql := create_sql || ' WITH CHECK (' || new_check || ')';
    END IF;
    EXECUTE create_sql;
  END LOOP;
END
$policies$;

-- 3) Rewrite function bodies that still cast 'owner'::app_role.
DO $funcs$
DECLARE
  r record;
  def text;
  new_def text;
BEGIN
  FOR r IN
    SELECT p.oid
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname IN ('public', 'private')
      AND p.prokind IN ('f', 'p')
      AND p.proname <> 'rewrite_owner_app_role_expr'
  LOOP
    BEGIN
      def := pg_get_functiondef(r.oid);
    EXCEPTION WHEN OTHERS THEN
      CONTINUE;
    END;
    IF def IS NULL THEN
      CONTINUE;
    END IF;
    IF def NOT LIKE '%''owner''::%app_role%'
       AND def NOT LIKE '%has_role(%''owner''%'
       AND def NOT LIKE '%has_tenant_role(%''owner''%'
       AND def NOT LIKE '%NEW.role = ''owner''%'
    THEN
      CONTINUE;
    END IF;
    new_def := private.rewrite_owner_app_role_expr(def);
    IF new_def IS DISTINCT FROM def THEN
      EXECUTE new_def;
    END IF;
  END LOOP;
END
$funcs$;

-- 4) Known trigger that compared NEW.role to owner (blocks every user_roles insert).
CREATE OR REPLACE FUNCTION public.trg_seed_terminal_templates_on_owner()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NEW.role = 'primary'::public.app_role THEN
    PERFORM public.seed_terminal_templates(COALESCE(NEW.owner_id, NEW.user_id));
  END IF;
  RETURN NEW;
END;
$$;

-- 5) effective_owner must use primary.
CREATE OR REPLACE FUNCTION private.effective_owner(_user_id uuid)
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT CASE
    WHEN private.has_role(_user_id, 'primary'::public.app_role) THEN _user_id
    ELSE (
      SELECT owner_id FROM public.user_roles
      WHERE user_id = _user_id AND role = 'client'::public.app_role
      LIMIT 1
    )
  END;
$$;

CREATE OR REPLACE FUNCTION public.effective_owner(_user_id uuid)
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public
AS $$
  SELECT private.effective_owner(_user_id);
$$;

-- 6) Sanity: remaining policy casts (should return 0 rows after a good run).
-- SELECT schemaname, tablename, policyname
-- FROM pg_policies
-- WHERE schemaname IN ('public','storage')
--   AND (coalesce(qual,'') LIKE '%''owner''::%app_role%'
--     OR coalesce(with_check,'') LIKE '%''owner''::%app_role%');
