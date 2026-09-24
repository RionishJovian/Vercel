-- tunnel_hubs is shared WireGuard hub infrastructure (endpoint + hub public key).
-- The previous "platform" policies used has_role(..., 'owner'|'admin'), but every
-- shop owner holds the tenant owner role, so any tenant could read/write all hubs.
-- Restrict to explicitly listed platform administrators only.

DROP POLICY IF EXISTS "tunnel hubs platform read" ON public.tunnel_hubs;
DROP POLICY IF EXISTS "tunnel hubs platform write" ON public.tunnel_hubs;
DROP POLICY IF EXISTS "tenant read tunnel hubs" ON public.tunnel_hubs;
DROP POLICY IF EXISTS "tenant write tunnel hubs" ON public.tunnel_hubs;

CREATE POLICY "tunnel hubs platform read" ON public.tunnel_hubs
  FOR SELECT TO authenticated
  USING (public.is_platform_admin(auth.uid()));

CREATE POLICY "tunnel hubs platform write" ON public.tunnel_hubs
  FOR ALL TO authenticated
  USING (public.is_platform_admin(auth.uid()))
  WITH CHECK (public.is_platform_admin(auth.uid()));
