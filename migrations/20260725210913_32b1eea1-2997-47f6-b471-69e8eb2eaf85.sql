CREATE TABLE public.terminal_history (
  id UUID NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id UUID NOT NULL,
  router_id UUID NOT NULL REFERENCES public.router_connections(id) ON DELETE CASCADE,
  site_id UUID REFERENCES public.sites(id) ON DELETE SET NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  body TEXT,
  status INTEGER NOT NULL DEFAULT 0,
  ms INTEGER NOT NULL DEFAULT 0,
  response_snippet TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.terminal_history TO authenticated;
GRANT ALL ON public.terminal_history TO service_role;

ALTER TABLE public.terminal_history ENABLE ROW LEVEL SECURITY;

CREATE POLICY "terminal_history_owner_scope_select" ON public.terminal_history
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "terminal_history_insert_self" ON public.terminal_history
  FOR INSERT TO authenticated
  WITH CHECK (user_id = auth.uid() AND owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "terminal_history_delete_self" ON public.terminal_history
  FOR DELETE TO authenticated
  USING (user_id = auth.uid() OR owner_id = auth.uid());

CREATE INDEX idx_terminal_history_owner_created ON public.terminal_history(owner_id, created_at DESC);
CREATE INDEX idx_terminal_history_router ON public.terminal_history(router_id, created_at DESC);