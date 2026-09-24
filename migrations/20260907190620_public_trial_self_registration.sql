-- Public trial registration is performed by a rate-limited server function.
-- Never grant the platform Primary role implicitly, even on an empty restore.
-- A Primary must be provisioned explicitly by an administrator.

CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_primary uuid;
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (
    NEW.id,
    COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1))
  );

  SELECT user_id
  INTO v_primary
  FROM public.user_roles
  WHERE role = 'primary'::public.app_role
  ORDER BY created_at
  LIMIT 1;

  INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
  VALUES (
    NEW.id,
    'client'::public.app_role,
    COALESCE(v_primary, NEW.id),
    now() + INTERVAL '7 days'
  );

  PERFORM public.seed_owner_defaults(NEW.id);
  RETURN NEW;
END;
$$;

REVOKE ALL ON FUNCTION public.handle_new_user() FROM PUBLIC, anon, authenticated;
