
-- Clear expiry on any non-client role rows
UPDATE public.user_roles SET expires_at = NULL WHERE role <> 'client' AND expires_at IS NOT NULL;

-- Safeguard trigger: force expires_at to NULL unless role = 'client'
CREATE OR REPLACE FUNCTION public.enforce_client_only_expiry()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  IF NEW.role <> 'client' THEN
    NEW.expires_at := NULL;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_user_roles_client_only_expiry ON public.user_roles;
CREATE TRIGGER trg_user_roles_client_only_expiry
BEFORE INSERT OR UPDATE ON public.user_roles
FOR EACH ROW EXECUTE FUNCTION public.enforce_client_only_expiry();
