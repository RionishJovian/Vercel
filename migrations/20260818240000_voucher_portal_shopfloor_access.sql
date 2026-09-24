-- Voucher profile push + portal HTML deploy: every active role (not expired).
-- Also lets any non-expired tenant member record a portal_deploy_audit row.

DROP POLICY IF EXISTS "portal_deploy_audit tenant insert" ON public.portal_deploy_audit;
CREATE POLICY "portal_deploy_audit tenant insert" ON public.portal_deploy_audit
  FOR INSERT TO authenticated
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND user_id = auth.uid()
    AND NOT public.is_expired(auth.uid())
  );

DROP POLICY IF EXISTS "portal_deploy_audit tenant read" ON public.portal_deploy_audit;
CREATE POLICY "portal_deploy_audit tenant read" ON public.portal_deploy_audit
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

-- Shop-floor voucher plans + issued codes: any non-expired, non-pending tenant member.
DROP POLICY IF EXISTS "portal_plans operator write" ON public.portal_plans;
DROP POLICY IF EXISTS "portal_plans tenant write" ON public.portal_plans;
CREATE POLICY "portal_plans tenant write" ON public.portal_plans
  FOR ALL TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND NOT public.has_role(auth.uid(), 'pending'::public.app_role)
  )
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND NOT public.has_role(auth.uid(), 'pending'::public.app_role)
  );

DROP POLICY IF EXISTS "voucher_codes operator write" ON public.voucher_codes;
DROP POLICY IF EXISTS "voucher_codes write own tenant" ON public.voucher_codes;
DROP POLICY IF EXISTS "voucher_codes tenant write" ON public.voucher_codes;
DROP POLICY IF EXISTS "voucher_codes tenant write" ON public.voucher_codes;
CREATE POLICY "voucher_codes tenant write" ON public.voucher_codes
  FOR ALL TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND NOT public.has_role(auth.uid(), 'pending'::public.app_role)
  )
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND NOT public.has_role(auth.uid(), 'pending'::public.app_role)
  );

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
