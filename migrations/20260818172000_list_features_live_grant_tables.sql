-- Lovable Cloud SQL Editor — run 2/2 (definers + grant RLS).
-- Paste ONLY this SQL. Safe to re-run after the previous failure:
-- it does not reference public.feature_user_grants (that table is not on Cloud).

REVOKE EXECUTE ON FUNCTION public.has_tenant_role(uuid, public.app_role, uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.has_tenant_role(uuid, public.app_role, uuid) TO authenticated, service_role;

DO $revoke$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT n.nspname AS sch, p.proname, pg_get_function_identity_arguments(p.oid) AS args, p.prorettype
      FROM pg_proc p
      JOIN pg_namespace n ON n.oid = p.pronamespace
     WHERE n.nspname IN ('public', 'private')
       AND p.prosecdef
  LOOP
    EXECUTE format(
      'REVOKE ALL ON FUNCTION %I.%I(%s) FROM PUBLIC, anon',
      r.sch, r.proname, r.args
    );
    IF r.prorettype = 'trigger'::regtype THEN
      EXECUTE format(
        'REVOKE ALL ON FUNCTION %I.%I(%s) FROM authenticated',
        r.sch, r.proname, r.args
      );
    END IF;
  END LOOP;
END
$revoke$;

CREATE SCHEMA IF NOT EXISTS private;
REVOKE ALL ON SCHEMA private FROM PUBLIC, anon;
GRANT USAGE ON SCHEMA private TO authenticated, service_role;

-- Privileged body uses only tables that exist on Cloud (operator_* / portal_mode_*).
-- Dynamic SQL so CREATE FUNCTION succeeds even if one of those tables is missing.
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
  IF public.has_role(_user_id, 'owner'::public.app_role)
     OR public.has_role(_user_id, 'admin'::public.app_role) THEN
    RETURN ARRAY[
      'hybrid_light','guest_commerce','vouchers','portal_deploy','cash_sales',
      'bank_edit','reboot','alerts','syslog_ai','poe','telegram'
    ];
  END IF;
  IF public.has_role(_user_id, 'pending'::public.app_role)
     OR public.has_role(_user_id, 'expired'::public.app_role) THEN
    RETURN ARRAY[]::text[];
  END IF;

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

  -- Older unused catalog — only if that migration was applied.
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

CREATE OR REPLACE FUNCTION private.has_feature(_user_id uuid, _feature text)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT private.list_features(_user_id) @> ARRAY[_feature];
$$;

REVOKE ALL ON FUNCTION private.has_feature(uuid, text) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION private.has_feature(uuid, text) TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.list_features(_user_id uuid)
RETURNS text[]
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public
AS $$
  SELECT private.list_features(
    CASE
      WHEN public.is_platform_admin(auth.uid()) THEN COALESCE(_user_id, auth.uid())
      ELSE auth.uid()
    END
  );
$$;

CREATE OR REPLACE FUNCTION public.has_feature(_user_id uuid, _feature text)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public
AS $$
  SELECT private.has_feature(
    CASE
      WHEN public.is_platform_admin(auth.uid()) THEN COALESCE(_user_id, auth.uid())
      ELSE auth.uid()
    END,
    _feature
  );
$$;

REVOKE ALL ON FUNCTION public.list_features(uuid) FROM PUBLIC, anon;
REVOKE ALL ON FUNCTION public.has_feature(uuid, text) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.list_features(uuid) TO authenticated, service_role;
GRANT EXECUTE ON FUNCTION public.has_feature(uuid, text) TO authenticated, service_role;

DO $policies$
BEGIN
  IF to_regclass('public.operator_feature_grants') IS NOT NULL THEN
    EXECUTE 'DROP POLICY IF EXISTS "operator_feature_grants self read" ON public.operator_feature_grants';
    EXECUTE $p$
      CREATE POLICY "operator_feature_grants self read" ON public.operator_feature_grants
      FOR SELECT TO authenticated
      USING (
        user_id = auth.uid()
        OR public.is_platform_admin(auth.uid())
        OR (
          (public.has_role(auth.uid(), 'owner'::public.app_role)
            OR public.has_role(auth.uid(), 'admin'::public.app_role))
          AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
        )
      )
    $p$;
    EXECUTE 'DROP POLICY IF EXISTS "operator_feature_grants owner write" ON public.operator_feature_grants';
    EXECUTE $p$
      CREATE POLICY "operator_feature_grants owner write" ON public.operator_feature_grants
      FOR ALL TO authenticated
      USING (
        public.is_platform_admin(auth.uid())
        OR (
          (public.has_role(auth.uid(), 'owner'::public.app_role)
            OR public.has_role(auth.uid(), 'admin'::public.app_role))
          AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
        )
      )
      WITH CHECK (
        public.is_platform_admin(auth.uid())
        OR (
          (public.has_role(auth.uid(), 'owner'::public.app_role)
            OR public.has_role(auth.uid(), 'admin'::public.app_role))
          AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
        )
      )
    $p$;
  END IF;

  IF to_regclass('public.portal_mode_grants') IS NOT NULL THEN
    EXECUTE 'DROP POLICY IF EXISTS "portal_mode_grants self read" ON public.portal_mode_grants';
    EXECUTE $p$
      CREATE POLICY "portal_mode_grants self read" ON public.portal_mode_grants
      FOR SELECT TO authenticated
      USING (
        user_id = auth.uid()
        OR public.is_platform_admin(auth.uid())
        OR (
          (public.has_role(auth.uid(), 'owner'::public.app_role)
            OR public.has_role(auth.uid(), 'admin'::public.app_role))
          AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
        )
      )
    $p$;
    EXECUTE 'DROP POLICY IF EXISTS "portal_mode_grants owner write" ON public.portal_mode_grants';
    EXECUTE $p$
      CREATE POLICY "portal_mode_grants owner write" ON public.portal_mode_grants
      FOR ALL TO authenticated
      USING (
        public.is_platform_admin(auth.uid())
        OR (
          (public.has_role(auth.uid(), 'owner'::public.app_role)
            OR public.has_role(auth.uid(), 'admin'::public.app_role))
          AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
        )
      )
      WITH CHECK (
        public.is_platform_admin(auth.uid())
        OR (
          (public.has_role(auth.uid(), 'owner'::public.app_role)
            OR public.has_role(auth.uid(), 'admin'::public.app_role))
          AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
        )
      )
    $p$;
  END IF;
END
$policies$;
