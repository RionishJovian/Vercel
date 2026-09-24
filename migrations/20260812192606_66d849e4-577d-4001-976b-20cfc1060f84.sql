-- 1. SECURITY DEFINER exposure -------------------------------------------------

-- Rate-limit helper is only ever called with the service role.
REVOKE ALL ON FUNCTION public.connector_rate_hit(text, integer, integer) FROM public, anon, authenticated;
GRANT EXECUTE ON FUNCTION public.connector_rate_hit(text, integer, integer) TO service_role;

-- Platform-admin check is needed inside RLS, so keep the definer body in the
-- private schema and expose only a SECURITY INVOKER wrapper in the API schema.
CREATE OR REPLACE FUNCTION private.is_platform_admin(_user_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE SECURITY DEFINER
SET search_path TO 'public'
AS $$
  SELECT EXISTS (SELECT 1 FROM public.platform_admins pa WHERE pa.user_id = _user_id);
$$;
REVOKE ALL ON FUNCTION private.is_platform_admin(uuid) FROM public, anon;
GRANT EXECUTE ON FUNCTION private.is_platform_admin(uuid) TO authenticated, service_role;

CREATE OR REPLACE FUNCTION public.is_platform_admin(_user_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE SECURITY INVOKER
SET search_path TO 'public'
AS $$
  SELECT private.is_platform_admin(_user_id);
$$;
REVOKE ALL ON FUNCTION public.is_platform_admin(uuid) FROM public, anon;
GRANT EXECUTE ON FUNCTION public.is_platform_admin(uuid) TO authenticated, service_role;

-- 2. account_entitlements -------------------------------------------------------
DROP POLICY IF EXISTS "Users read their own entitlement" ON public.account_entitlements;
CREATE POLICY "Users read their own entitlement"
  ON public.account_entitlements FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
    OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, owner_id)
    OR public.is_platform_admin(auth.uid())
  );

-- 3. account_referrals ----------------------------------------------------------
DROP POLICY IF EXISTS "Owners read all referrals" ON public.account_referrals;
CREATE POLICY "Owners read referrals in their business"
  ON public.account_referrals FOR SELECT TO authenticated
  USING (
    public.is_platform_admin(auth.uid())
    OR (
      (public.has_role(auth.uid(), 'owner'::public.app_role)
        OR public.has_role(auth.uid(), 'admin'::public.app_role))
      AND EXISTS (
        SELECT 1 FROM public.user_roles ur
         WHERE ur.user_id = account_referrals.user_id
           AND ur.owner_id = public.effective_owner(auth.uid())
      )
    )
  );

-- 4. owner_accounts -------------------------------------------------------------
DROP POLICY IF EXISTS "Owners can view owner_accounts" ON public.owner_accounts;
CREATE POLICY "Owners can view their own owner account"
  ON public.owner_accounts FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR user_id = public.effective_owner(auth.uid())
    OR public.is_platform_admin(auth.uid())
  );

-- 5. router_ops_audit -----------------------------------------------------------
DROP POLICY IF EXISTS "Read own account router operations audit" ON public.router_ops_audit;
CREATE POLICY "Read own account router operations audit"
  ON public.router_ops_audit FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
    OR public.is_platform_admin(auth.uid())
  );

-- 6. user_roles -----------------------------------------------------------------
DROP POLICY IF EXISTS "roles owner manage" ON public.user_roles;
CREATE POLICY "roles owner manage"
  ON public.user_roles FOR ALL TO authenticated
  USING (
    public.is_platform_admin(auth.uid())
    OR public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
  )
  WITH CHECK (
    public.is_platform_admin(auth.uid())
    OR public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
  );

DROP POLICY IF EXISTS "roles self read" ON public.user_roles;
CREATE POLICY "roles self read"
  ON public.user_roles FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.is_platform_admin(auth.uid())
    OR public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
  );
