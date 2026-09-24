ALTER TABLE public.router_connections
  ADD COLUMN IF NOT EXISTS connection_preference text NOT NULL DEFAULT 'auto',
  ADD COLUMN IF NOT EXISTS pending_tunnel_public_key text,
  ADD COLUMN IF NOT EXISTS pending_tunnel_private_key_ciphertext text,
  ADD COLUMN IF NOT EXISTS rotation_started_at timestamptz,
  ADD COLUMN IF NOT EXISTS last_rotated_at timestamptz,
  ADD COLUMN IF NOT EXISTS last_active_path text;

CREATE TABLE IF NOT EXISTS public.ai_usage_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  user_id uuid NOT NULL,
  feature text NOT NULL,
  model text,
  prompt_tokens integer NOT NULL DEFAULT 0,
  completion_tokens integer NOT NULL DEFAULT 0,
  total_tokens integer NOT NULL DEFAULT 0,
  cached boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT ON public.ai_usage_events TO authenticated;
GRANT ALL ON public.ai_usage_events TO service_role;
ALTER TABLE public.ai_usage_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY "ai usage readable by owner scope" ON public.ai_usage_events
  FOR SELECT TO authenticated
  USING (user_id = auth.uid() OR owner_id = public.effective_owner(auth.uid()) OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "ai usage insert own" ON public.ai_usage_events
  FOR INSERT TO authenticated
  WITH CHECK (user_id = auth.uid());
CREATE INDEX IF NOT EXISTS ai_usage_events_owner_created_idx ON public.ai_usage_events (owner_id, created_at DESC);

CREATE TABLE IF NOT EXISTS public.device_allowances (
  owner_id uuid PRIMARY KEY,
  routers integer NOT NULL DEFAULT 1,
  controllers integer NOT NULL DEFAULT 1,
  sites integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT ON public.device_allowances TO authenticated;
GRANT ALL ON public.device_allowances TO service_role;
ALTER TABLE public.device_allowances ENABLE ROW LEVEL SECURITY;
CREATE POLICY "allowances readable in scope" ON public.device_allowances
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "allowances managed by owners" ON public.device_allowances
  FOR ALL TO authenticated
  USING (public.has_role(auth.uid(), 'owner'))
  WITH CHECK (public.has_role(auth.uid(), 'owner'));
CREATE TRIGGER touch_device_allowances BEFORE UPDATE ON public.device_allowances
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE TABLE IF NOT EXISTS public.device_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  user_id uuid NOT NULL,
  kind text NOT NULL,
  reason text,
  status text NOT NULL DEFAULT 'pending',
  decided_by uuid,
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT ON public.device_requests TO authenticated;
GRANT UPDATE ON public.device_requests TO authenticated;
GRANT ALL ON public.device_requests TO service_role;
ALTER TABLE public.device_requests ENABLE ROW LEVEL SECURITY;
CREATE POLICY "requests readable in scope" ON public.device_requests
  FOR SELECT TO authenticated
  USING (user_id = auth.uid() OR owner_id = public.effective_owner(auth.uid()) OR public.has_role(auth.uid(), 'owner'));
CREATE POLICY "requests created by self" ON public.device_requests
  FOR INSERT TO authenticated
  WITH CHECK (user_id = auth.uid());
CREATE POLICY "requests decided by owners" ON public.device_requests
  FOR UPDATE TO authenticated
  USING (public.has_role(auth.uid(), 'owner'))
  WITH CHECK (public.has_role(auth.uid(), 'owner'));
CREATE INDEX IF NOT EXISTS device_requests_status_idx ON public.device_requests (status, created_at DESC);
CREATE TRIGGER touch_device_requests BEFORE UPDATE ON public.device_requests
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();