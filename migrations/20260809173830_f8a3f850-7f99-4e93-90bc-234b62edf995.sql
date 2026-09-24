CREATE OR REPLACE FUNCTION private.effective_owner(_user_id uuid)
RETURNS uuid
LANGUAGE sql
STABLE SECURITY DEFINER
SET search_path TO 'public'
AS $function$
  SELECT COALESCE(
    -- Owners, admins, agents, clients and expired clients own their own data.
    CASE WHEN EXISTS (
      SELECT 1 FROM public.user_roles
       WHERE user_id = _user_id
         AND role IN ('owner','admin','agent','client','expired','pending')
    ) THEN _user_id END,
    -- Genuine sub-accounts (site managers / read-only members) inherit the owner.
    (SELECT owner_id FROM public.user_roles
       WHERE user_id = _user_id
         AND role IN ('site_manager','read_only')
         AND owner_id IS NOT NULL
       ORDER BY created_at LIMIT 1),
    _user_id
  );
$function$;