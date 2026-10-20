-- Prune OPERATOR, TECHNICIAN, SUPPORT from RBAC role CHECK constraints.
-- These roles were consolidated into ADMIN after the RBAC streamlining;
-- grantable tenant roles are now OWNER/ADMIN/VIEWER and DEVELOPER is
-- platform-only. Viewer is retained; Owner/Admin/Developer stay unchanged.
--
-- Existing rows holding a removed role are reassigned to ADMIN (the
-- documented consolidation target) BEFORE the stricter constraints are
-- enforced, so an upgraded database cannot fail on surviving rows. Only
-- the role column is touched — user, business and invitation associations
-- keep their tenant boundaries and no privilege is granted beyond the
-- consolidation itself.
--
-- The whole migration runs in ONE transaction. Apply with the documented
-- `psql -v ON_ERROR_STOP=1` like the other migrations; the BEGIN/COMMIT
-- below additionally keep psql's default autocommit mode from committing
-- partial work when any statement fails (a failed ADD CONSTRAINT must not
-- leave a preceding DROP CONSTRAINT applied).
BEGIN;

-- 0) Reassign removed roles to their consolidation target first.
UPDATE public.users SET user_role = 'ADMIN'
 WHERE user_role IN ('OPERATOR', 'TECHNICIAN', 'SUPPORT');
UPDATE public.business_memberships SET role = 'ADMIN'
 WHERE role IN ('OPERATOR', 'TECHNICIAN', 'SUPPORT');
UPDATE public.business_invitations SET role = 'ADMIN'
 WHERE role IN ('OPERATOR', 'TECHNICIAN', 'SUPPORT');
UPDATE public.operator_feature_role_defaults SET role = 'ADMIN'
 WHERE role IN ('OPERATOR', 'TECHNICIAN', 'SUPPORT');
UPDATE public.portal_mode_role_defaults SET role = 'ADMIN'
 WHERE role IN ('OPERATOR', 'TECHNICIAN', 'SUPPORT');

-- 1) users.user_role
ALTER TABLE public.users DROP CONSTRAINT IF EXISTS users_user_role_check;
ALTER TABLE public.users ADD CONSTRAINT users_user_role_check
    CHECK (user_role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER'));

-- 2) business_memberships.role
ALTER TABLE public.business_memberships DROP CONSTRAINT IF EXISTS business_memberships_role_check;
ALTER TABLE public.business_memberships ADD CONSTRAINT business_memberships_role_check
    CHECK (role IN ('OWNER', 'ADMIN', 'VIEWER'));

-- 3) business_invitations.role (OWNER excluded — invitations create staff, not owners)
ALTER TABLE public.business_invitations DROP CONSTRAINT IF EXISTS business_invitations_role_check;
ALTER TABLE public.business_invitations ADD CONSTRAINT business_invitations_role_check
    CHECK (role IN ('ADMIN', 'VIEWER'));

-- 4) operator_feature_role_defaults.role
ALTER TABLE public.operator_feature_role_defaults DROP CONSTRAINT IF EXISTS operator_feature_role_defaults_role_check;
ALTER TABLE public.operator_feature_role_defaults ADD CONSTRAINT operator_feature_role_defaults_role_check
    CHECK (role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER'));

-- 5) portal_mode_role_defaults.role
ALTER TABLE public.portal_mode_role_defaults DROP CONSTRAINT IF EXISTS portal_mode_role_defaults_role_check;
ALTER TABLE public.portal_mode_role_defaults ADD CONSTRAINT portal_mode_role_defaults_role_check
    CHECK (role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER'));

COMMIT;
