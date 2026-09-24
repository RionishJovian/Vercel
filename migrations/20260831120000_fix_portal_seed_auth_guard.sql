-- Account creation runs handle_new_user from the Auth database trigger. That
-- trigger seeds portal_settings without a request JWT, so the default,
-- voucher-only row must not be rejected as an unauthenticated gated-mode write.
-- Gated modes remain protected, while trusted service-role requests retain
-- their existing ability to maintain portal settings.

CREATE OR REPLACE FUNCTION public.enforce_portal_guest_mode_grant()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_actor uuid := auth.uid();
  v_jwt_role text := current_setting('request.jwt.claim.role', true);
BEGIN
  -- This is the safe default seeded for every new account. It carries no
  -- temporary-access or commerce capability and is valid without a JWT.
  IF NEW.guest_mode = 'voucher_only' THEN
    RETURN NEW;
  END IF;

  -- Trusted backend requests may maintain already-authorized portal settings.
  IF v_jwt_role = 'service_role' THEN
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
