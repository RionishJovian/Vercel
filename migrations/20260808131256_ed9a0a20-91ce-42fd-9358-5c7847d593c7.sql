ALTER TABLE public.unifi_controllers
  ADD COLUMN IF NOT EXISTS brand text NOT NULL DEFAULT 'unifi',
  ADD COLUMN IF NOT EXISTS api_base_path text,
  ADD COLUMN IF NOT EXISTS capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL;

ALTER TABLE public.unifi_controllers
  DROP CONSTRAINT IF EXISTS unifi_controllers_brand_check;
ALTER TABLE public.unifi_controllers
  ADD CONSTRAINT unifi_controllers_brand_check
  CHECK (brand IN ('unifi','mikrotik','ruijie','generic'));

CREATE TABLE IF NOT EXISTS public.ap_devices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  controller_id uuid NOT NULL REFERENCES public.unifi_controllers(id) ON DELETE CASCADE,
  mac text NOT NULL,
  name text,
  model text,
  location text,
  notes text,
  last_state text,
  last_seen_at timestamptz,
  cpu_pct numeric,
  mem_pct numeric,
  client_count integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (controller_id, mac)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.ap_devices TO authenticated;
GRANT ALL ON public.ap_devices TO service_role;
ALTER TABLE public.ap_devices ENABLE ROW LEVEL SECURITY;

CREATE POLICY "ap_devices_select_own" ON public.ap_devices
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "ap_devices_write_own" ON public.ap_devices
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));

CREATE TRIGGER touch_ap_devices BEFORE UPDATE ON public.ap_devices
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE TABLE IF NOT EXISTS public.ap_actions_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  user_id uuid NOT NULL,
  controller_id uuid REFERENCES public.unifi_controllers(id) ON DELETE SET NULL,
  brand text NOT NULL,
  target text,
  action text NOT NULL,
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  success boolean NOT NULL,
  error_message text,
  created_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT ON public.ap_actions_audit TO authenticated;
GRANT ALL ON public.ap_actions_audit TO service_role;
ALTER TABLE public.ap_actions_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "ap_audit_select_own" ON public.ap_actions_audit
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "ap_audit_insert_own" ON public.ap_actions_audit
  FOR INSERT TO authenticated
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND user_id = auth.uid());
