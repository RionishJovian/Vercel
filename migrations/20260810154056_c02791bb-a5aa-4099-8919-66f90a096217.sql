CREATE TABLE IF NOT EXISTS public.notifier_audit (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null,
  actor_user_id uuid,
  channel text not null default 'telegram',
  action text not null,
  outcome text not null default 'ok',
  detail text,
  created_at timestamptz not null default now()
);

GRANT SELECT ON public.notifier_audit TO authenticated;
GRANT ALL ON public.notifier_audit TO service_role;

ALTER TABLE public.notifier_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Tenant can read own notifier audit"
ON public.notifier_audit FOR SELECT TO authenticated
USING (owner_id = public.effective_owner(auth.uid()));

CREATE INDEX IF NOT EXISTS idx_notifier_audit_owner_created
ON public.notifier_audit (owner_id, created_at DESC);