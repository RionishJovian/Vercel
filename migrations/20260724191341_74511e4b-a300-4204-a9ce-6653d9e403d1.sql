CREATE TABLE public.portal_deploy_audit (
  id UUID NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
  owner_id UUID NOT NULL,
  user_id UUID NOT NULL,
  router_id UUID NOT NULL REFERENCES public.router_connections(id) ON DELETE CASCADE,
  router_name TEXT NOT NULL,
  ok BOOLEAN NOT NULL DEFAULT false,
  files JSONB NOT NULL DEFAULT '[]'::jsonb,
  snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  error TEXT,
  duration_ms INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.portal_deploy_audit TO authenticated;
GRANT ALL ON public.portal_deploy_audit TO service_role;

ALTER TABLE public.portal_deploy_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "owners_read_own_deploys" ON public.portal_deploy_audit
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "owners_insert_own_deploys" ON public.portal_deploy_audit
  FOR INSERT TO authenticated
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND user_id = auth.uid());

CREATE INDEX idx_portal_deploy_audit_owner_created
  ON public.portal_deploy_audit(owner_id, created_at DESC);