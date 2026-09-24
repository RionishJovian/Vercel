-- Authoritative guard for gated captive-portal modes.
-- The app already checks grants, but portal_settings is client-writable under
-- tenant RLS. This trigger prevents a direct PostgREST write from bypassing
-- Hybrid Light / Guest Commerce permission checks.

CREATE OR REPLACE FUNCTION public.can_operate_portal_guest_mode(
  _user_id uuid,
  _mode text
)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT
    _mode = 'voucher_only'
    OR public.is_platform_admin(_user_id)
    OR public.has_role(_user_id, 'primary'::public.app_role)
    OR EXISTS (
      SELECT 1
      FROM public.portal_mode_grants g
      WHERE g.user_id = _user_id
        AND g.mode = _mode
    )
    OR EXISTS (
      SELECT 1
      FROM public.user_roles ur
      JOIN public.portal_mode_role_defaults d ON d.role = ur.role
      WHERE ur.user_id = _user_id
        AND d.mode = _mode
    );
$$;

REVOKE ALL ON FUNCTION public.can_operate_portal_guest_mode(uuid, text) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.can_operate_portal_guest_mode(uuid, text) TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.enforce_portal_guest_mode_grant()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_actor uuid := auth.uid();
BEGIN
  -- Trusted backend and migrations may maintain historical settings. Browser
  -- and PostgREST writes always carry the authenticated actor and are checked.
  IF auth.role() = 'service_role' THEN
    RETURN NEW;
  END IF;

  IF v_actor IS NULL THEN
    RAISE EXCEPTION 'PORTAL_MODE_AUTH_REQUIRED';
  END IF;

  IF NOT public.can_operate_portal_guest_mode(v_actor, NEW.guest_mode) THEN
    RAISE EXCEPTION 'PORTAL_MODE_PERMISSION_REQUIRED:%', NEW.guest_mode
      USING ERRCODE = '42501';
  END IF;

  RETURN NEW;
END;
$$;

REVOKE ALL ON FUNCTION public.enforce_portal_guest_mode_grant() FROM PUBLIC, anon, authenticated;

DROP TRIGGER IF EXISTS enforce_portal_guest_mode_grant ON public.portal_settings;
CREATE TRIGGER enforce_portal_guest_mode_grant
  BEFORE INSERT OR UPDATE OF guest_mode ON public.portal_settings
  FOR EACH ROW EXECUTE FUNCTION public.enforce_portal_guest_mode_grant();

COMMENT ON FUNCTION public.can_operate_portal_guest_mode(uuid, text) IS
  'Checks whether a user may activate a captive portal guest mode.';
COMMENT ON FUNCTION public.enforce_portal_guest_mode_grant() IS
  'Rejects direct portal_settings writes that attempt Hybrid Light or Guest Commerce without a grant.';
