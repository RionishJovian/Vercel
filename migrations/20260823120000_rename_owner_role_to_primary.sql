-- Rename app_role.owner → primary (tenant billing/admin row).
-- Safe to re-run: skips enum rename when primary already exists.
-- Paste in Lovable Cloud SQL Editor, then republish.

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

CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE is_first boolean;
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (
    NEW.id,
    COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1))
  );

  SELECT NOT EXISTS (
    SELECT 1 FROM public.user_roles WHERE role = 'primary'::public.app_role
  ) INTO is_first;

  IF is_first THEN
    INSERT INTO public.user_roles (user_id, role, owner_id)
    VALUES (NEW.id, 'primary'::public.app_role, NEW.id);
  ELSE
    INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
    VALUES (
      NEW.id,
      'client'::public.app_role,
      (SELECT user_id FROM public.user_roles WHERE role = 'primary'::public.app_role ORDER BY created_at LIMIT 1),
      now() + INTERVAL '7 days'
    );
  END IF;

  PERFORM public.seed_owner_defaults(NEW.id);
  RETURN NEW;
END;
$$;

-- Rewrite RLS policies that still reference the old enum literal 'owner'.
DO $policies$
DECLARE
  pol record;
  new_qual text;
  new_check text;
  role_list text;
  create_sql text;
BEGIN
  FOR pol IN
    SELECT schemaname, tablename, policyname, permissive, roles, cmd, qual, with_check
    FROM pg_policies
    WHERE schemaname = 'public'
      AND (
        coalesce(qual, '') LIKE '%''owner''%'
        OR coalesce(with_check, '') LIKE '%''owner''%'
      )
  LOOP
    new_qual := pol.qual;
    new_check := pol.with_check;

    IF new_qual IS NOT NULL THEN
      new_qual := replace(new_qual, '''owner''::public.app_role', '''primary''::public.app_role');
      new_qual := replace(new_qual, '''owner''::app_role', '''primary''::app_role');
      new_qual := replace(new_qual, 'has_role(auth.uid(), ''owner'')', 'has_role(auth.uid(), ''primary'')');
      new_qual := replace(new_qual, 'has_role(_user_id, ''owner'')', 'has_role(_user_id, ''primary'')');
      new_qual := replace(new_qual, 'has_role(_uid, ''owner'')', 'has_role(_uid, ''primary'')');
    END IF;

    IF new_check IS NOT NULL THEN
      new_check := replace(new_check, '''owner''::public.app_role', '''primary''::public.app_role');
      new_check := replace(new_check, '''owner''::app_role', '''primary''::app_role');
      new_check := replace(new_check, 'has_role(auth.uid(), ''owner'')', 'has_role(auth.uid(), ''primary'')');
      new_check := replace(new_check, 'has_role(_user_id, ''owner'')', 'has_role(_user_id, ''primary'')');
      new_check := replace(new_check, 'has_role(_uid, ''owner'')', 'has_role(_uid, ''primary'')');
    END IF;

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

-- list_features privileged branch (owner → primary).
CREATE OR REPLACE FUNCTION private.list_features(_user_id uuid)
RETURNS text[]
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  collected text[] := ARRAY[]::text[];
  extra text[];
BEGIN
  IF public.has_role(_user_id, 'primary'::public.app_role) THEN
    RETURN ARRAY[
      'hybrid_light','guest_commerce','vouchers','portal_deploy','cash_sales',
      'bank_edit','reboot','alerts','syslog_ai','poe','telegram'
    ];
  END IF;
  IF public.has_role(_user_id, 'pending'::public.app_role)
     OR public.has_role(_user_id, 'expired'::public.app_role) THEN
    RETURN ARRAY[]::text[];
  END IF;

  collected := ARRAY['vouchers','portal_deploy']::text[];

  IF to_regclass('public.operator_feature_grants') IS NOT NULL THEN
    EXECUTE $q$
      SELECT COALESCE(array_agg(DISTINCT feature), ARRAY[]::text[])
      FROM public.operator_feature_grants
      WHERE user_id = $1
    $q$ INTO extra USING _user_id;
    collected := collected || COALESCE(extra, ARRAY[]::text[]);
  END IF;

  IF to_regclass('public.operator_feature_role_defaults') IS NOT NULL THEN
    EXECUTE $q$
      SELECT COALESCE(array_agg(DISTINCT d.feature), ARRAY[]::text[])
      FROM public.user_roles ur
      JOIN public.operator_feature_role_defaults d ON d.role = ur.role
      WHERE ur.user_id = $1
    $q$ INTO extra USING _user_id;
    collected := collected || COALESCE(extra, ARRAY[]::text[]);
  END IF;

  IF to_regclass('public.portal_mode_grants') IS NOT NULL THEN
    EXECUTE $q$
      SELECT COALESCE(array_agg(DISTINCT mode), ARRAY[]::text[])
      FROM public.portal_mode_grants
      WHERE user_id = $1
    $q$ INTO extra USING _user_id;
    collected := collected || COALESCE(extra, ARRAY[]::text[]);
  END IF;

  IF to_regclass('public.portal_mode_role_defaults') IS NOT NULL THEN
    EXECUTE $q$
      SELECT COALESCE(array_agg(DISTINCT d.mode), ARRAY[]::text[])
      FROM public.user_roles ur
      JOIN public.portal_mode_role_defaults d ON d.role = ur.role
      WHERE ur.user_id = $1
    $q$ INTO extra USING _user_id;
    collected := collected || COALESCE(extra, ARRAY[]::text[]);
  END IF;

  IF to_regclass('public.feature_user_grants') IS NOT NULL THEN
    EXECUTE $q$
      SELECT COALESCE(array_agg(DISTINCT feature), ARRAY[]::text[])
      FROM public.feature_user_grants
      WHERE user_id = $1 AND enabled
    $q$ INTO extra USING _user_id;
    collected := collected || COALESCE(extra, ARRAY[]::text[]);
  END IF;

  IF to_regclass('public.feature_role_defaults') IS NOT NULL THEN
    EXECUTE $q$
      SELECT COALESCE(array_agg(DISTINCT d.feature), ARRAY[]::text[])
      FROM public.user_roles ur
      JOIN public.feature_role_defaults d
        ON d.role = ur.role AND d.enabled
      WHERE ur.user_id = $1
        AND d.owner_id = COALESCE(ur.owner_id, public.effective_owner($1))
    $q$ INTO extra USING _user_id;
    collected := collected || COALESCE(extra, ARRAY[]::text[]);
  END IF;

  SELECT COALESCE(array_agg(DISTINCT x), ARRAY[]::text[])
    INTO extra
    FROM unnest(collected) AS x
   WHERE x IS NOT NULL AND x <> '';
  RETURN COALESCE(extra, ARRAY[]::text[]);
END;
$$;

REVOKE ALL ON FUNCTION private.list_features(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION private.list_features(uuid) TO authenticated, service_role;
