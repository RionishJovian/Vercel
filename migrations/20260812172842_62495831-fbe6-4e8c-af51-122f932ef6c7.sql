-- 1) Tenant column for the Magic Points ledger
ALTER TABLE public.agent_points ADD COLUMN IF NOT EXISTS owner_id uuid;

UPDATE public.agent_points
   SET owner_id = COALESCE(public.effective_owner(referred_user_id), referred_user_id)
 WHERE owner_id IS NULL AND referred_user_id IS NOT NULL;

UPDATE public.agent_points
   SET owner_id = COALESCE(public.effective_owner(agent_id), agent_id)
 WHERE owner_id IS NULL;

CREATE INDEX IF NOT EXISTS agent_points_owner_idx ON public.agent_points(owner_id);
CREATE INDEX IF NOT EXISTS agent_points_agent_created_idx ON public.agent_points(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS agent_points_referred_idx ON public.agent_points(referred_user_id);

DROP POLICY IF EXISTS "Owners read all points" ON public.agent_points;
CREATE POLICY "Tenant staff read tenant points"
  ON public.agent_points FOR SELECT TO authenticated
  USING (
    public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
    OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, owner_id)
    OR public.is_platform_admin(auth.uid())
  );

-- 2) Tier Pass purchases + audit trail scoped to the buyer's tenant
DROP POLICY IF EXISTS "Users read their own purchases" ON public.service_purchases;
CREATE POLICY "Tenant read service purchases"
  ON public.service_purchases FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
    OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, owner_id)
    OR public.is_platform_admin(auth.uid())
  );

DROP POLICY IF EXISTS "Owners and admins read the purchase audit trail" ON public.service_purchase_audit;
CREATE POLICY "Tenant read service purchase audit"
  ON public.service_purchase_audit FOR SELECT TO authenticated
  USING (
    EXISTS (
      SELECT 1 FROM public.service_purchases sp
       WHERE sp.id = service_purchase_audit.purchase_id
         AND (
           sp.user_id = auth.uid()
           OR public.has_tenant_role(auth.uid(), 'owner'::public.app_role, sp.owner_id)
           OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, sp.owner_id)
           OR public.is_platform_admin(auth.uid())
         )
    )
  );

-- 3) Voucher operator boundary: only owner/admin of the tenant may write
DROP POLICY IF EXISTS "portal_plans tenant write" ON public.portal_plans;
CREATE POLICY "portal_plans operator write"
  ON public.portal_plans FOR ALL TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
  )
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
  );

DROP POLICY IF EXISTS "voucher_codes write own tenant" ON public.voucher_codes;
CREATE POLICY "voucher_codes operator write"
  ON public.voucher_codes FOR ALL TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
  )
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.is_expired(auth.uid())
    AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
  );