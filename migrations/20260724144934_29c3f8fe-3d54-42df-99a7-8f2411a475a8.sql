
CREATE TABLE public.router_save_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL,
  owner_id uuid,
  action text NOT NULL,
  router_id uuid,
  attempted_name text,
  attempted_host text,
  success boolean NOT NULL,
  error_message text,
  error_code text,
  created_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON public.router_save_audit TO authenticated;
GRANT ALL ON public.router_save_audit TO service_role;

ALTER TABLE public.router_save_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "audit_select_own"
  ON public.router_save_audit
  FOR SELECT
  TO authenticated
  USING (
    user_id = auth.uid()
    OR owner_id = auth.uid()
    OR owner_id = public.effective_owner(auth.uid())
  );

CREATE INDEX router_save_audit_owner_created_idx
  ON public.router_save_audit (owner_id, created_at DESC);
CREATE INDEX router_save_audit_user_created_idx
  ON public.router_save_audit (user_id, created_at DESC);
