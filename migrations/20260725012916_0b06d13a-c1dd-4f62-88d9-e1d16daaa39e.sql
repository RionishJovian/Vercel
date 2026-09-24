-- 1) Add new enum value (must be in its own effective step; do not use 'expired'::app_role
--    literal in this migration). Function bodies referencing it are fine.
ALTER TYPE public.app_role ADD VALUE IF NOT EXISTS 'expired';

-- 2) Add expiration column to user_roles
ALTER TABLE public.user_roles
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- 3) Backfill expires_at for existing client rows (1 month from created_at)
UPDATE public.user_roles
   SET expires_at = created_at + INTERVAL '1 month'
 WHERE role = 'client' AND expires_at IS NULL;

-- 4) Update handle_new_user to set expires_at for new client signups
CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1)));

  INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
  VALUES (NEW.id, 'client', NEW.id, now() + INTERVAL '1 month');

  RETURN NEW;
END;
$$;

-- 5) Function to expire clients whose expires_at has passed.
--    Deletes the client role row and inserts an expired role row.
--    Function body is deferred; the 'expired' enum literal is only evaluated when called.
CREATE OR REPLACE FUNCTION public.expire_stale_clients()
RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  affected INTEGER := 0;
  r RECORD;
BEGIN
  FOR r IN
    SELECT user_id, owner_id
      FROM public.user_roles
     WHERE role = 'client'
       AND expires_at IS NOT NULL
       AND expires_at <= now()
  LOOP
    DELETE FROM public.user_roles
     WHERE user_id = r.user_id AND role = 'client';
    INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
    VALUES (r.user_id, 'expired'::public.app_role, r.owner_id, NULL)
    ON CONFLICT (user_id, role) DO NOTHING;
    affected := affected + 1;
  END LOOP;
  RETURN affected;
END;
$$;

REVOKE ALL ON FUNCTION public.expire_stale_clients() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.expire_stale_clients() TO service_role;

-- 6) Schedule hourly cron
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM cron.job WHERE jobname = 'expire-stale-clients') THEN
    PERFORM cron.unschedule('expire-stale-clients');
  END IF;
  PERFORM cron.schedule(
    'expire-stale-clients',
    '5 * * * *',
    $CRON$ SELECT public.expire_stale_clients(); $CRON$
  );
END $$;