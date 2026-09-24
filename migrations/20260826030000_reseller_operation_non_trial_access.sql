-- Reseller Operation is available to every non-trial app account. Trial
-- accounts remain denied at navigation, route, server, and RLS layers.

CREATE OR REPLACE FUNCTION public.owner_operations_is_trial_account(_user_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1
    FROM public.user_roles role_row
    LEFT JOIN public.account_entitlements entitlement ON entitlement.user_id = role_row.user_id
    LEFT JOIN public.profiles profile ON profile.id = role_row.user_id
    WHERE role_row.user_id = _user_id
      AND role_row.role = 'client'::public.app_role
      AND COALESCE(entitlement.tier, 'trial') = 'trial'
      AND COALESCE(entitlement.tier_expires_at, role_row.expires_at) > now()
      AND COALESCE(entitlement.tier_expires_at, role_row.expires_at)
        <= COALESCE(profile.created_at, now()) + interval '7 days'
  );
$$;

REVOKE ALL ON FUNCTION public.owner_operations_is_trial_account(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.owner_operations_is_trial_account(uuid) TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.owner_operations_can_read(_owner uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public
AS $$
  SELECT (
    _owner = public.effective_owner(auth.uid())
    OR public.is_platform_admin(auth.uid())
  ) AND NOT public.owner_operations_is_trial_account(auth.uid());
$$;

CREATE OR REPLACE FUNCTION public.owner_operations_can_write(_owner uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public
AS $$
  SELECT public.owner_operations_can_read(_owner);
$$;

REVOKE ALL ON FUNCTION public.owner_operations_can_read(uuid), public.owner_operations_can_write(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.owner_operations_can_read(uuid), public.owner_operations_can_write(uuid)
  TO authenticated, service_role;
