-- Prune OPERATOR, TECHNICIAN, SUPPORT from RBAC role CHECK constraints.
-- These roles were consolidated to ADMIN after the RBAC streamlining.
-- Viewer is retained; Owner/Admin/Developer remain unchanged.
-- No existing rows contain the removed roles, so no data reassignment needed.

-- 1) users.user_role
ALTER TABLE public.users DROP CONSTRAINT IF EXISTS users_user_role_check;
ALTER TABLE public.users ADD CONSTRAINT users_user_role_check
    CHECK (user_role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER'));

-- 2) business_memberships.role
ALTER TABLE public.business_memberships DROP CONSTRAINT IF EXISTS business_memberships_role_check;
ALTER TABLE public.business_memberships ADD CONSTRAINT business_memberships_role_check
    CHECK (role IN ('OWNER', 'ADMIN', 'VIEWER'));

-- 3) business_invitations.role (OWNER excluded — invitations create staff, not owners)
ALTER TABLE business_invitations DROP CONSTRAINT IF EXISTS business_invitations_role_check;
ALTER TABLE business_invitations ADD CONSTRAINT business_invitations_role_check
    CHECK (role IN ('ADMIN', 'VIEWER'));

-- 4) operator_feature_role_defaults.role
ALTER TABLE public.operator_feature_role_defaults DROP CONSTRAINT IF EXISTS operator_feature_role_defaults_role_check;
ALTER TABLE public.operator_feature_role_defaults ADD CONSTRAINT operator_feature_role_defaults_role_check
    CHECK (role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER'));

-- 5) portal_mode_role_defaults.role
ALTER TABLE public.portal_mode_role_defaults DROP CONSTRAINT IF EXISTS portal_mode_role_defaults_role_check;
ALTER TABLE public.portal_mode_role_defaults ADD CONSTRAINT portal_mode_role_defaults_role_check
    CHECK (role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER'));
