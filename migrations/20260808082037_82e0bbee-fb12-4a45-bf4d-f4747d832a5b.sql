CREATE TABLE public.tunnel_hubs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  name text NOT NULL,
  wg_endpoint text NOT NULL,
  hub_public_key text NOT NULL,
  https_base_url text NOT NULL,
  subnet_cidr text NOT NULL DEFAULT '10.88.0.0/24',
  notes text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.tunnel_hubs TO authenticated;
GRANT ALL ON public.tunnel_hubs TO service_role;

ALTER TABLE public.tunnel_hubs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "tenant read tunnel hubs" ON public.tunnel_hubs
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "tenant write tunnel hubs" ON public.tunnel_hubs
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));

CREATE TRIGGER touch_tunnel_hubs BEFORE UPDATE ON public.tunnel_hubs
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

ALTER TABLE public.router_connections
  ADD COLUMN connection_mode text NOT NULL DEFAULT 'direct',
  ADD COLUMN tunnel_hub_id uuid REFERENCES public.tunnel_hubs(id) ON DELETE SET NULL,
  ADD COLUMN tunnel_address text,
  ADD COLUMN tunnel_public_key text,
  ADD COLUMN tunnel_private_key_ciphertext text,
  ADD COLUMN tunnel_listen_port integer,
  ADD COLUMN tunnel_last_check_at timestamptz,
  ADD COLUMN tunnel_last_ok boolean;