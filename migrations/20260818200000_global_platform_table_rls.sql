-- Global platform tables (no owner_id): restrict writes to platform administrators.
-- Tenant owners/admins manage per-user grants on operator_feature_grants and
-- portal_mode_grants (see 20260818160000_production_security_hardening.sql).

DROP POLICY IF EXISTS "operator_feature_role_defaults owner write" ON public.operator_feature_role_defaults;
CREATE POLICY "operator_feature_role_defaults platform write" ON public.operator_feature_role_defaults
FOR ALL TO authenticated
USING (public.is_platform_admin(auth.uid()))
WITH CHECK (public.is_platform_admin(auth.uid()));

DROP POLICY IF EXISTS "portal_mode_role_defaults owner write" ON public.portal_mode_role_defaults;
CREATE POLICY "portal_mode_role_defaults platform write" ON public.portal_mode_role_defaults
FOR ALL TO authenticated
USING (public.is_platform_admin(auth.uid()))
WITH CHECK (public.is_platform_admin(auth.uid()));

DROP POLICY IF EXISTS "Owners can update the pricing promo" ON public.pricing_promo;
CREATE POLICY "Platform admins can update the pricing promo" ON public.pricing_promo
FOR UPDATE TO authenticated
USING (public.is_platform_admin(auth.uid()))
WITH CHECK (public.is_platform_admin(auth.uid()));
