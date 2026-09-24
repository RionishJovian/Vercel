-- Account hierarchy: owner_id is the direct parent/creator for new child accounts.
-- Operational tenant ownership remains separate: effective_owner() returns the
-- client/agent account itself, preventing parent accounts from gaining automatic
-- access to child operational data.

CREATE OR REPLACE FUNCTION private.effective_owner(_user_id uuid)
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT CASE
    WHEN private.has_role(_user_id, 'primary'::public.app_role) THEN _user_id
    WHEN EXISTS (
      SELECT 1
      FROM public.user_roles
      WHERE user_id = _user_id
        AND role::text IN ('client', 'agent', 'expired', 'pending')
    ) THEN _user_id
    ELSE (
      SELECT owner_id
      FROM public.user_roles
      WHERE user_id = _user_id
        AND owner_id IS NOT NULL
      ORDER BY created_at
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
