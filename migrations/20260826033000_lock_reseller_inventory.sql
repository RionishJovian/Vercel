-- Lock reseller-contact inventory mutations for User, Trial, and Expired
-- accounts. The Reseller Operation dashboard remains readable after trial.

CREATE OR REPLACE FUNCTION public.owner_operations_can_manage_reseller_inventory(_owner uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY INVOKER
SET search_path = public
AS $$
  SELECT public.owner_operations_can_read(_owner)
    AND (
      public.has_role(auth.uid(), 'primary'::public.app_role)
      OR public.has_role(auth.uid(), 'agent'::public.app_role)
      OR public.is_platform_admin(auth.uid())
    );
$$;

REVOKE ALL ON FUNCTION public.owner_operations_can_manage_reseller_inventory(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.owner_operations_can_manage_reseller_inventory(uuid)
  TO authenticated, service_role;

DROP POLICY IF EXISTS owner_operations_write ON public.voucher_resellers;
DROP POLICY IF EXISTS owner_operations_reseller_inventory_write ON public.voucher_resellers;
CREATE POLICY owner_operations_reseller_inventory_write
  ON public.voucher_resellers
  FOR ALL TO authenticated
  USING (public.owner_operations_can_manage_reseller_inventory(owner_id))
  WITH CHECK (public.owner_operations_can_manage_reseller_inventory(owner_id));
