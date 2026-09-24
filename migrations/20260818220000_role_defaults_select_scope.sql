-- Global role-default tables must not allow unrestricted SELECT for every
-- authenticated account. Platform admins still see every row; other signed-in
-- users only see defaults for roles they hold (client / agent / site_manager).
-- Tenant grant resolution in loadGrantContext continues to work; the full
-- matrix stays off PostgREST.

DROP POLICY IF EXISTS "operator_feature_role_defaults read" ON public.operator_feature_role_defaults;
CREATE POLICY "operator_feature_role_defaults read" ON public.operator_feature_role_defaults
FOR SELECT TO authenticated
USING (
  public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), role)
);

DROP POLICY IF EXISTS "portal_mode_role_defaults read" ON public.portal_mode_role_defaults;
CREATE POLICY "portal_mode_role_defaults read" ON public.portal_mode_role_defaults
FOR SELECT TO authenticated
USING (
  public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), role)
);
