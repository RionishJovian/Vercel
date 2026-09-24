DROP POLICY IF EXISTS "portal_settings tenant read" ON public.portal_settings;
DROP POLICY IF EXISTS "portal_settings tenant write" ON public.portal_settings;

CREATE POLICY "portal_settings tenant read" ON public.portal_settings
FOR SELECT TO authenticated
USING (
  owner_id = auth.uid()
  OR owner_id = public.effective_owner(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::app_role)
);

CREATE POLICY "portal_settings tenant write" ON public.portal_settings
FOR ALL TO authenticated
USING (
  (owner_id = auth.uid()
   OR owner_id = public.effective_owner(auth.uid())
   OR public.has_role(auth.uid(), 'owner'::app_role))
  AND NOT public.is_expired(auth.uid())
)
WITH CHECK (
  (owner_id = auth.uid()
   OR owner_id = public.effective_owner(auth.uid())
   OR public.has_role(auth.uid(), 'owner'::app_role))
  AND NOT public.is_expired(auth.uid())
);

DROP POLICY IF EXISTS "portal_plans tenant read" ON public.portal_plans;
DROP POLICY IF EXISTS "portal_plans tenant write" ON public.portal_plans;

CREATE POLICY "portal_plans tenant read" ON public.portal_plans
FOR SELECT TO authenticated
USING (
  owner_id = auth.uid()
  OR owner_id = public.effective_owner(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::app_role)
);

CREATE POLICY "portal_plans tenant write" ON public.portal_plans
FOR ALL TO authenticated
USING (
  (owner_id = auth.uid()
   OR owner_id = public.effective_owner(auth.uid())
   OR public.has_role(auth.uid(), 'owner'::app_role))
  AND NOT public.is_expired(auth.uid())
)
WITH CHECK (
  (owner_id = auth.uid()
   OR owner_id = public.effective_owner(auth.uid())
   OR public.has_role(auth.uid(), 'owner'::app_role))
  AND NOT public.is_expired(auth.uid())
);