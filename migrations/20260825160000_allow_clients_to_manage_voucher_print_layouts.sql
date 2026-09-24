-- Active café Users may customize their own counter receipt layout.
-- Agents remain excluded: voucher reseller/agent workflows do not confer
-- authority over a client's branding or receipt configuration.
DROP POLICY IF EXISTS voucher_print_layouts_write ON public.voucher_print_layouts;
CREATE POLICY voucher_print_layouts_write ON public.voucher_print_layouts FOR ALL TO authenticated
  USING (
    (owner_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid()))
    AND (
      public.has_role(auth.uid(), 'primary'::public.app_role)
      OR (
        public.has_role(auth.uid(), 'client'::public.app_role)
        AND NOT public.has_role(auth.uid(), 'expired'::public.app_role)
        AND NOT EXISTS (
          SELECT 1
          FROM public.user_roles AS active_client
          WHERE active_client.user_id = auth.uid()
            AND active_client.role = 'client'::public.app_role
            AND active_client.expires_at IS NOT NULL
            AND active_client.expires_at <= now()
        )
      )
      OR public.is_platform_admin(auth.uid())
    )
  )
  WITH CHECK (
    (owner_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid()))
    AND (
      public.has_role(auth.uid(), 'primary'::public.app_role)
      OR (
        public.has_role(auth.uid(), 'client'::public.app_role)
        AND NOT public.has_role(auth.uid(), 'expired'::public.app_role)
        AND NOT EXISTS (
          SELECT 1
          FROM public.user_roles AS active_client
          WHERE active_client.user_id = auth.uid()
            AND active_client.role = 'client'::public.app_role
            AND active_client.expires_at IS NOT NULL
            AND active_client.expires_at <= now()
        )
      )
      OR public.is_platform_admin(auth.uid())
    )
  );
