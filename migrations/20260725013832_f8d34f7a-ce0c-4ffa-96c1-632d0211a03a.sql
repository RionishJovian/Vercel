
CREATE TABLE public.admin_notifications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recipient_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  kind text NOT NULL,
  title text NOT NULL,
  body text,
  data jsonb NOT NULL DEFAULT '{}'::jsonb,
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, UPDATE ON public.admin_notifications TO authenticated;
GRANT ALL ON public.admin_notifications TO service_role;

ALTER TABLE public.admin_notifications ENABLE ROW LEVEL SECURITY;

CREATE POLICY "recipient can read own notifications"
  ON public.admin_notifications
  FOR SELECT TO authenticated
  USING (recipient_id = auth.uid());

CREATE POLICY "recipient can mark own notifications read"
  ON public.admin_notifications
  FOR UPDATE TO authenticated
  USING (recipient_id = auth.uid())
  WITH CHECK (recipient_id = auth.uid());

CREATE INDEX admin_notifications_recipient_created_idx
  ON public.admin_notifications (recipient_id, created_at DESC);

CREATE INDEX admin_notifications_recipient_unread_idx
  ON public.admin_notifications (recipient_id)
  WHERE read_at IS NULL;

CREATE OR REPLACE FUNCTION public.notify_owners_client_expired()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  client_name text;
  client_email text;
  owner_row record;
BEGIN
  IF NEW.role <> 'expired' THEN
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

  FOR owner_row IN
    SELECT ur.user_id FROM public.user_roles ur WHERE ur.role = 'owner'
  LOOP
    INSERT INTO public.admin_notifications (recipient_id, kind, title, body, data)
    VALUES (
      owner_row.user_id,
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
  END LOOP;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_notify_owners_client_expired ON public.user_roles;
CREATE TRIGGER trg_notify_owners_client_expired
AFTER INSERT ON public.user_roles
FOR EACH ROW EXECUTE FUNCTION public.notify_owners_client_expired();
