ALTER TABLE public.router_connections
  ADD COLUMN IF NOT EXISTS environment text NOT NULL DEFAULT 'production',
  ADD COLUMN IF NOT EXISTS insecure_tls_reason text,
  ADD COLUMN IF NOT EXISTS insecure_tls_approved_by uuid,
  ADD COLUMN IF NOT EXISTS insecure_tls_approved_at timestamptz;

ALTER TABLE public.router_connections
  DROP CONSTRAINT IF EXISTS router_connections_environment_check;
ALTER TABLE public.router_connections
  ADD CONSTRAINT router_connections_environment_check
  CHECK (environment IN ('test', 'production'));

CREATE TABLE IF NOT EXISTS public.router_ops_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid,
  user_id uuid NOT NULL,
  router_id uuid,
  router_name text,
  action text NOT NULL,
  environment text NOT NULL DEFAULT 'production',
  outcome text NOT NULL DEFAULT 'ok',
  detail text,
  error_message text,
  duration_ms integer,
  created_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON public.router_ops_audit TO authenticated;
GRANT ALL ON public.router_ops_audit TO service_role;

ALTER TABLE public.router_ops_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Read own account router operations audit"
  ON public.router_ops_audit
  FOR SELECT
  TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    OR user_id = auth.uid()
    OR public.has_role(auth.uid(), 'owner'::public.app_role)
    OR public.has_role(auth.uid(), 'admin'::public.app_role)
  );

CREATE INDEX IF NOT EXISTS router_ops_audit_owner_created_idx
  ON public.router_ops_audit (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS router_ops_audit_router_created_idx
  ON public.router_ops_audit (router_id, created_at DESC);