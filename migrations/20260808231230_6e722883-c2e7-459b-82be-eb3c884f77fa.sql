CREATE TABLE public.connectors (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  name text NOT NULL,
  public_id text NOT NULL UNIQUE,
  enabled boolean NOT NULL DEFAULT true,
  status text NOT NULL DEFAULT 'unpaired',
  paired_at timestamptz,
  last_seen_at timestamptz,
  version text,
  local_ip text,
  local_subnet text,
  hostname text,
  pairing_code_hash text,
  pairing_code_expires_at timestamptz,
  token_hash text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX connectors_owner_idx ON public.connectors(owner_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.connectors TO authenticated;
GRANT ALL ON public.connectors TO service_role;

ALTER TABLE public.connectors ENABLE ROW LEVEL SECURITY;

CREATE POLICY "connectors_select_own" ON public.connectors
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "connectors_write_own" ON public.connectors
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));

CREATE TRIGGER touch_connectors
  BEFORE UPDATE ON public.connectors
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE TABLE public.connector_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  connector_id uuid NOT NULL REFERENCES public.connectors(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL,
  kind text NOT NULL DEFAULT 'http',
  request jsonb NOT NULL DEFAULT '{}'::jsonb,
  response jsonb,
  status text NOT NULL DEFAULT 'queued',
  error text,
  claimed_at timestamptz,
  completed_at timestamptz,
  expires_at timestamptz NOT NULL DEFAULT (now() + interval '2 minutes'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX connector_jobs_queue_idx ON public.connector_jobs(connector_id, status, created_at);

GRANT SELECT ON public.connector_jobs TO authenticated;
GRANT ALL ON public.connector_jobs TO service_role;

ALTER TABLE public.connector_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "connector_jobs_select_own" ON public.connector_jobs
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE TRIGGER touch_connector_jobs
  BEFORE UPDATE ON public.connector_jobs
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

ALTER TABLE public.router_connections
  ADD COLUMN connector_id uuid REFERENCES public.connectors(id) ON DELETE SET NULL;

ALTER TABLE public.unifi_controllers
  ADD COLUMN connector_id uuid REFERENCES public.connectors(id) ON DELETE SET NULL;