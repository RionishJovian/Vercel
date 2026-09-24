
-- syslog_tokens: per-owner ingest tokens
CREATE TABLE public.syslog_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  token text NOT NULL UNIQUE,
  label text,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz
);
GRANT SELECT, INSERT, UPDATE, DELETE ON public.syslog_tokens TO authenticated;
GRANT ALL ON public.syslog_tokens TO service_role;
ALTER TABLE public.syslog_tokens ENABLE ROW LEVEL SECURITY;

CREATE POLICY "owners manage their tokens" ON public.syslog_tokens
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) AND public.has_role(auth.uid(), 'owner'))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND public.has_role(auth.uid(), 'owner'));

CREATE INDEX syslog_tokens_owner_idx ON public.syslog_tokens (owner_id);

-- syslog_events
CREATE TABLE public.syslog_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  source_ip text,
  facility text,
  severity text NOT NULL DEFAULT 'info',
  program text,
  message text NOT NULL,
  ai_summary text,
  ai_severity text,
  received_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE, DELETE ON public.syslog_events TO authenticated;
GRANT ALL ON public.syslog_events TO service_role;
ALTER TABLE public.syslog_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY "tenant read syslog" ON public.syslog_events
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "privileged write syslog" ON public.syslog_events
  FOR UPDATE TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) AND (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin')))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin')));

CREATE POLICY "privileged delete syslog" ON public.syslog_events
  FOR DELETE TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) AND (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin')));

CREATE INDEX syslog_events_owner_time_idx ON public.syslog_events (owner_id, received_at DESC);
CREATE INDEX syslog_events_router_idx ON public.syslog_events (router_id);
CREATE INDEX syslog_events_severity_idx ON public.syslog_events (severity);
