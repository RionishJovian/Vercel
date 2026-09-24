CREATE OR REPLACE FUNCTION public.handle_new_user()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
 SET search_path TO 'public'
AS $function$
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1)));

  INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
  VALUES (NEW.id, 'client', NEW.id, now() + INTERVAL '7 days');

  PERFORM public.seed_owner_defaults(NEW.id);

  RETURN NEW;
END;
$function$;

-- Convert any accounts still awaiting approval into active 7-day clients.
INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
SELECT ur.user_id, 'client'::public.app_role, COALESCE(ur.owner_id, ur.user_id), now() + INTERVAL '7 days'
FROM public.user_roles ur
WHERE ur.role = 'pending'
ON CONFLICT (user_id, role) DO NOTHING;

DELETE FROM public.user_roles WHERE role = 'pending';