-- Persist real RouterOS /system/resource observations and support actionable,
-- deduplicated CPU/memory incidents. No remediation command is executed.

ALTER TABLE public.device_health_samples
  ADD COLUMN IF NOT EXISTS cpu_usage_pct double precision,
  ADD COLUMN IF NOT EXISTS memory_usage_pct double precision,
  ADD COLUMN IF NOT EXISTS free_memory_bytes bigint,
  ADD COLUMN IF NOT EXISTS total_memory_bytes bigint;

ALTER TABLE public.device_health_samples
  DROP CONSTRAINT IF EXISTS device_health_samples_cpu_usage_pct_check,
  DROP CONSTRAINT IF EXISTS device_health_samples_memory_usage_pct_check,
  DROP CONSTRAINT IF EXISTS device_health_samples_free_memory_bytes_check,
  DROP CONSTRAINT IF EXISTS device_health_samples_total_memory_bytes_check;

ALTER TABLE public.device_health_samples
  ADD CONSTRAINT device_health_samples_cpu_usage_pct_check
    CHECK (cpu_usage_pct IS NULL OR cpu_usage_pct BETWEEN 0 AND 100),
  ADD CONSTRAINT device_health_samples_memory_usage_pct_check
    CHECK (memory_usage_pct IS NULL OR memory_usage_pct BETWEEN 0 AND 100),
  ADD CONSTRAINT device_health_samples_free_memory_bytes_check
    CHECK (free_memory_bytes IS NULL OR free_memory_bytes >= 0),
  ADD CONSTRAINT device_health_samples_total_memory_bytes_check
    CHECK (total_memory_bytes IS NULL OR total_memory_bytes > 0);

ALTER TABLE public.alert_rules DROP CONSTRAINT IF EXISTS alert_rules_kind_check;
ALTER TABLE public.incidents DROP CONSTRAINT IF EXISTS incidents_kind_check;

ALTER TABLE public.alert_rules
  ADD CONSTRAINT alert_rules_kind_check CHECK (kind IN (
    'site_offline', 'wan_degraded', 'connector_stale', 'poe_ap_offline',
    'provisioning_failed', 'interface_down', 'dhcp_pool_high',
    'router_anomaly', 'wan_saturated', 'vpn_peer_down',
    'router_cpu_high', 'router_memory_high'
  ));

ALTER TABLE public.incidents
  ADD CONSTRAINT incidents_kind_check CHECK (kind IN (
    'site_offline', 'wan_degraded', 'connector_stale', 'poe_ap_offline',
    'provisioning_failed', 'interface_down', 'dhcp_pool_high',
    'router_anomaly', 'wan_saturated', 'vpn_peer_down',
    'router_cpu_high', 'router_memory_high'
  ));

INSERT INTO public.alert_rules
  (owner_id, kind, enabled, severity, cooldown_minutes, threshold)
SELECT DISTINCT COALESCE(ur.owner_id, ur.user_id), rule.kind, true, rule.severity, rule.cooldown, 2
FROM public.user_roles ur
CROSS JOIN (VALUES
  ('router_cpu_high', 'critical', 30),
  ('router_memory_high', 'warning', 60)
) AS rule(kind, severity, cooldown)
WHERE ur.role IN ('owner', 'admin', 'client')
ON CONFLICT (owner_id, kind) DO NOTHING;

-- Supabase owns the durable schedule; the web endpoint owns the monitoring
-- cycle. The private secret is never exposed to browser clients.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM cron.job WHERE jobname = 'router-health-every-five-minutes') THEN
    PERFORM cron.unschedule('router-health-every-five-minutes');
  END IF;
END $$;

SELECT cron.schedule(
  'router-health-every-five-minutes',
  '*/5 * * * *',
  format($cmd$
  SELECT net.http_post(
    url := 'https://mikromagic.com/api/public/hooks/router-health',
    headers := jsonb_build_object(
      'Content-Type', 'application/json',
      'x-cron-secret', %L
    ),
    body := '{}'::jsonb
  ) AS request_id;
  $cmd$, (SELECT secret FROM private.cron_secrets WHERE name = 'default'))
);
