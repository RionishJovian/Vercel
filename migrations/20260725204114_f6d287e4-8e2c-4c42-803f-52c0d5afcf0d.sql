CREATE TABLE public.unifi_controllers (
  id UUID NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  site_id UUID REFERENCES public.sites(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 8443,
  unifi_site TEXT NOT NULL DEFAULT 'default',
  username TEXT NOT NULL,
  password_ciphertext TEXT NOT NULL,
  is_unifi_os BOOLEAN NOT NULL DEFAULT true,
  allow_insecure_tls BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.unifi_controllers TO authenticated;
GRANT ALL ON public.unifi_controllers TO service_role;

ALTER TABLE public.unifi_controllers ENABLE ROW LEVEL SECURITY;

CREATE POLICY "owner or org can view unifi controllers"
  ON public.unifi_controllers FOR SELECT
  TO authenticated
  USING (
    owner_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
    OR public.has_role(auth.uid(), 'owner')
  );

CREATE POLICY "owner or org can insert unifi controllers"
  ON public.unifi_controllers FOR INSERT
  TO authenticated
  WITH CHECK (
    owner_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
  );

CREATE POLICY "owner or org can update unifi controllers"
  ON public.unifi_controllers FOR UPDATE
  TO authenticated
  USING (
    owner_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
    OR public.has_role(auth.uid(), 'owner')
  );

CREATE POLICY "owner or org can delete unifi controllers"
  ON public.unifi_controllers FOR DELETE
  TO authenticated
  USING (
    owner_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
    OR public.has_role(auth.uid(), 'owner')
  );

CREATE OR REPLACE FUNCTION public.tg_unifi_controllers_touch()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;

CREATE TRIGGER update_unifi_controllers_updated_at
  BEFORE UPDATE ON public.unifi_controllers
  FOR EACH ROW EXECUTE FUNCTION public.tg_unifi_controllers_touch();

CREATE INDEX idx_unifi_controllers_owner ON public.unifi_controllers(owner_id);
CREATE INDEX idx_unifi_controllers_site ON public.unifi_controllers(site_id);