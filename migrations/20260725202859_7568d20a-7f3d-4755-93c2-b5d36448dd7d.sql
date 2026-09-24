CREATE TABLE public.sites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  name text NOT NULL,
  location text,
  timezone text,
  notes text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sites_owner_idx ON public.sites(owner_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.sites TO authenticated;
GRANT ALL ON public.sites TO service_role;

ALTER TABLE public.sites ENABLE ROW LEVEL SECURITY;

CREATE POLICY "sites_select_own_or_owner" ON public.sites
  FOR SELECT TO authenticated
  USING (
    owner_id = auth.uid()
    OR public.has_role(auth.uid(), 'owner')
  );

CREATE POLICY "sites_insert_own" ON public.sites
  FOR INSERT TO authenticated
  WITH CHECK (
    owner_id = auth.uid()
    AND NOT public.has_role(auth.uid(), 'read_only')
    AND NOT public.has_role(auth.uid(), 'expired')
  );

CREATE POLICY "sites_update_own_or_owner" ON public.sites
  FOR UPDATE TO authenticated
  USING (
    owner_id = auth.uid()
    OR public.has_role(auth.uid(), 'owner')
  )
  WITH CHECK (
    NOT public.has_role(auth.uid(), 'read_only')
    AND NOT public.has_role(auth.uid(), 'expired')
  );

CREATE POLICY "sites_delete_own_or_owner" ON public.sites
  FOR DELETE TO authenticated
  USING (
    owner_id = auth.uid()
    OR public.has_role(auth.uid(), 'owner')
  );

CREATE TRIGGER sites_touch_updated_at
  BEFORE UPDATE ON public.sites
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

ALTER TABLE public.router_connections
  ADD COLUMN IF NOT EXISTS site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS router_connections_site_idx ON public.router_connections(site_id);
