-- Trigger still compared NEW.role to 'owner'::app_role after owner→primary rename.
-- Casting the dead enum label throws 22P02 on EVERY user_roles INSERT (even client).
CREATE OR REPLACE FUNCTION public.trg_seed_terminal_templates_on_owner()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NEW.role = 'primary'::public.app_role THEN
    PERFORM public.seed_terminal_templates(COALESCE(NEW.owner_id, NEW.user_id));
  END IF;
  RETURN NEW;
END;
$$;
