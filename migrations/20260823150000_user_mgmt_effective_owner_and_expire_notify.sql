-- User Management audit fixes (2026-08-23):
-- 1) effective_owner: resolve tenant Primary from any role row with owner_id
--    (client, agent, expired, pending) — not only client.
-- 2) notify_owners_client_expired: use primary role + notify only the
--    expired account's owner_id (café Primary), not every legacy owner.

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
      SELECT owner_id
      FROM public.user_roles
      WHERE user_id = _user_id
        AND owner_id IS NOT NULL
      ORDER BY CASE role
        WHEN 'client'::public.app_role THEN 1
        WHEN 'agent'::public.app_role THEN 2
        WHEN 'expired'::public.app_role THEN 3
        WHEN 'pending'::public.app_role THEN 4
        ELSE 5
      END
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

CREATE OR REPLACE FUNCTION public.notify_owners_client_expired()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  client_name text;
  client_email text;
  primary_id uuid;
BEGIN
  IF NEW.role IS DISTINCT FROM 'expired'::public.app_role THEN
    RETURN NEW;
  END IF;

  SELECT COALESCE(p.display_name, p.username)
    INTO client_name
    FROM public.profiles p
   WHERE p.id = NEW.user_id;

  SELECT au.email
    INTO client_email
    FROM auth.users au
   WHERE au.id = NEW.user_id;

  primary_id := NEW.owner_id;
  IF primary_id IS NULL THEN
    SELECT ur.user_id INTO primary_id
      FROM public.user_roles ur
     WHERE ur.role = 'primary'::public.app_role
     ORDER BY ur.created_at
     LIMIT 1;
  END IF;

  IF primary_id IS NULL THEN
    RETURN NEW;
  END IF;

  INSERT INTO public.admin_notifications (recipient_id, kind, title, body, data)
  VALUES (
    primary_id,
    'client_expired',
    'Client account expired',
    COALESCE(client_name, client_email, NEW.user_id::text) || ' was switched to read-only.',
    jsonb_build_object(
      'user_id', NEW.user_id,
      'client_name', client_name,
      'client_email', client_email,
      'expired_at', now()
    )
  );

  RETURN NEW;
END;
$$;
