ALTER TYPE public.app_role RENAME VALUE 'staff' TO 'client';

CREATE OR REPLACE FUNCTION public.handle_new_user()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
 SET search_path TO 'public'
AS $function$
DECLARE is_first BOOLEAN;
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1)));
  SELECT NOT EXISTS (SELECT 1 FROM public.user_roles WHERE role = 'owner') INTO is_first;
  IF is_first THEN
    INSERT INTO public.user_roles (user_id, role, owner_id) VALUES (NEW.id, 'owner', NEW.id);
  ELSE
    INSERT INTO public.user_roles (user_id, role, owner_id)
    VALUES (NEW.id, 'client', (SELECT user_id FROM public.user_roles WHERE role = 'owner' ORDER BY created_at LIMIT 1));
  END IF;
  RETURN NEW;
END; $function$;

CREATE OR REPLACE FUNCTION public.effective_owner(_user_id uuid)
 RETURNS uuid
 LANGUAGE sql
 STABLE SECURITY DEFINER
 SET search_path TO 'public'
AS $function$
  SELECT CASE
    WHEN public.has_role(_user_id, 'owner') THEN _user_id
    ELSE (SELECT owner_id FROM public.user_roles WHERE user_id = _user_id AND role = 'client' LIMIT 1)
  END;
$function$;