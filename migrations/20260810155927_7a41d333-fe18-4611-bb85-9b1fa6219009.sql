-- 1) Unified multi-vendor device inventory ------------------------------------
CREATE TABLE public.managed_devices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  controller_id uuid REFERENCES public.unifi_controllers(id) ON DELETE SET NULL,
  connector_id uuid REFERENCES public.connectors(id) ON DELETE SET NULL,
  vendor text NOT NULL CHECK (vendor IN ('mikrotik','ruijie','cisco','tplink','ubiquiti','generic')),
  category text NOT NULL CHECK (category IN ('router','gateway','switch','ap','controller')),
  transport text NOT NULL DEFAULT 'direct' CHECK (transport IN ('direct','connector','tunnel','controller')),
  name text NOT NULL,
  model text,
  mac text,
  host text,
  location text,
  notes text,
  capabilities jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX managed_devices_owner_idx ON public.managed_devices (owner_id, category);
CREATE INDEX managed_devices_site_idx ON public.managed_devices (site_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.managed_devices TO authenticated;
GRANT ALL ON public.managed_devices TO service_role;
ALTER TABLE public.managed_devices ENABLE ROW LEVEL SECURITY;
CREATE POLICY "managed_devices read own tenant" ON public.managed_devices
  FOR SELECT TO authenticated USING (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "managed_devices write own tenant" ON public.managed_devices
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));
CREATE TRIGGER touch_managed_devices BEFORE UPDATE ON public.managed_devices
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 2) Durable health history ----------------------------------------------------
CREATE TABLE public.device_health_samples (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  site_id uuid REFERENCES public.sites(id) ON DELETE CASCADE,
  device_id uuid REFERENCES public.managed_devices(id) ON DELETE CASCADE,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE CASCADE,
  subject_kind text NOT NULL CHECK (subject_kind IN ('site','router','device','connector')),
  subject_id text NOT NULL,
  reachable boolean,
  latency_ms integer,
  uptime_seconds bigint,
  wan_state text CHECK (wan_state IN ('up','degraded','down','unknown')),
  connector_state text CHECK (connector_state IN ('online','stale','offline','none')),
  tunnel_state text CHECK (tunnel_state IN ('up','down','none')),
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  observed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX device_health_owner_time_idx ON public.device_health_samples (owner_id, observed_at DESC);
CREATE INDEX device_health_subject_idx ON public.device_health_samples (owner_id, subject_kind, subject_id, observed_at DESC);

GRANT SELECT ON public.device_health_samples TO authenticated;
GRANT ALL ON public.device_health_samples TO service_role;
ALTER TABLE public.device_health_samples ENABLE ROW LEVEL SECURITY;
CREATE POLICY "health read own tenant" ON public.device_health_samples
  FOR SELECT TO authenticated USING (owner_id = public.effective_owner(auth.uid()));

-- 3) Alert rules ---------------------------------------------------------------
CREATE TABLE public.alert_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('site_offline','wan_degraded','connector_stale','poe_ap_offline','provisioning_failed')),
  enabled boolean NOT NULL DEFAULT true,
  severity text NOT NULL DEFAULT 'warning' CHECK (severity IN ('info','warning','critical')),
  cooldown_minutes integer NOT NULL DEFAULT 30 CHECK (cooldown_minutes BETWEEN 1 AND 1440),
  threshold integer NOT NULL DEFAULT 2 CHECK (threshold BETWEEN 1 AND 20),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_id, kind)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON public.alert_rules TO authenticated;
GRANT ALL ON public.alert_rules TO service_role;
ALTER TABLE public.alert_rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY "alert_rules read own tenant" ON public.alert_rules
  FOR SELECT TO authenticated USING (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "alert_rules write own tenant" ON public.alert_rules
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));
CREATE TRIGGER touch_alert_rules BEFORE UPDATE ON public.alert_rules
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 4) Incidents -----------------------------------------------------------------
CREATE TABLE public.incidents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  device_id uuid REFERENCES public.managed_devices(id) ON DELETE SET NULL,
  kind text NOT NULL CHECK (kind IN ('site_offline','wan_degraded','connector_stale','poe_ap_offline','provisioning_failed')),
  severity text NOT NULL DEFAULT 'warning' CHECK (severity IN ('info','warning','critical')),
  subject_id text NOT NULL,
  subject_label text NOT NULL,
  detail text,
  opened_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  last_notified_at timestamptz,
  resolved_at timestamptz,
  acknowledged_at timestamptz,
  acknowledged_by uuid
);
CREATE INDEX incidents_owner_open_idx ON public.incidents (owner_id, resolved_at, opened_at DESC);
CREATE UNIQUE INDEX incidents_open_unique_idx ON public.incidents (owner_id, kind, subject_id) WHERE resolved_at IS NULL;

GRANT SELECT, UPDATE ON public.incidents TO authenticated;
GRANT ALL ON public.incidents TO service_role;
ALTER TABLE public.incidents ENABLE ROW LEVEL SECURITY;
CREATE POLICY "incidents read own tenant" ON public.incidents
  FOR SELECT TO authenticated USING (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "incidents ack own tenant" ON public.incidents
  FOR UPDATE TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));

-- 5) Notification preferences --------------------------------------------------
CREATE TABLE public.notification_prefs (
  owner_id uuid PRIMARY KEY,
  in_app boolean NOT NULL DEFAULT true,
  email boolean NOT NULL DEFAULT false,
  quiet_hours_start smallint CHECK (quiet_hours_start BETWEEN 0 AND 23),
  quiet_hours_end smallint CHECK (quiet_hours_end BETWEEN 0 AND 23),
  min_severity text NOT NULL DEFAULT 'warning' CHECK (min_severity IN ('info','warning','critical')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE ON public.notification_prefs TO authenticated;
GRANT ALL ON public.notification_prefs TO service_role;
ALTER TABLE public.notification_prefs ENABLE ROW LEVEL SECURITY;
CREATE POLICY "notification_prefs read own tenant" ON public.notification_prefs
  FOR SELECT TO authenticated USING (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "notification_prefs write own tenant" ON public.notification_prefs
  FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));
CREATE TRIGGER touch_notification_prefs BEFORE UPDATE ON public.notification_prefs
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 6) Deployment history --------------------------------------------------------
CREATE TABLE public.deployment_history (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  actor_user_id uuid,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  device_id uuid REFERENCES public.managed_devices(id) ON DELETE SET NULL,
  intent text NOT NULL,
  mode text NOT NULL DEFAULT 'sandbox' CHECK (mode IN ('sandbox','dry_run','apply')),
  plan_hash text NOT NULL,
  idempotency_key text NOT NULL,
  diff_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  backup_ref text,
  verification jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','applied','verified','failed','rolled_back')),
  failure_reason text,
  rolled_back_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_id, idempotency_key)
);
CREATE INDEX deployment_history_owner_idx ON public.deployment_history (owner_id, created_at DESC);

GRANT SELECT ON public.deployment_history TO authenticated;
GRANT ALL ON public.deployment_history TO service_role;
ALTER TABLE public.deployment_history ENABLE ROW LEVEL SECURITY;
CREATE POLICY "deployment_history read own tenant" ON public.deployment_history
  FOR SELECT TO authenticated USING (owner_id = public.effective_owner(auth.uid()));

-- 7) Seed default alert rules for existing owners -------------------------------
INSERT INTO public.alert_rules (owner_id, kind, enabled, severity, cooldown_minutes, threshold)
SELECT DISTINCT ur.user_id, k.kind, true,
       CASE WHEN k.kind IN ('site_offline','provisioning_failed') THEN 'critical' ELSE 'warning' END,
       CASE k.kind WHEN 'site_offline' THEN 15 WHEN 'connector_stale' THEN 60 WHEN 'provisioning_failed' THEN 5 ELSE 30 END,
       CASE WHEN k.kind = 'provisioning_failed' THEN 1 ELSE 2 END
  FROM public.user_roles ur
 CROSS JOIN (VALUES ('site_offline'),('wan_degraded'),('connector_stale'),('poe_ap_offline'),('provisioning_failed')) AS k(kind)
 WHERE ur.role IN ('owner','admin','client')
ON CONFLICT (owner_id, kind) DO NOTHING;