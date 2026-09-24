-- Fleet scan runs (AI + security posture)
CREATE TABLE public.fleet_scan_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('ai','security')),
  payload jsonb NOT NULL,
  max_severity text NOT NULL DEFAULT 'info' CHECK (max_severity IN ('critical','warning','info','ok')),
  router_count int NOT NULL DEFAULT 0,
  generated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT ON public.fleet_scan_runs TO authenticated;
GRANT ALL ON public.fleet_scan_runs TO service_role;
ALTER TABLE public.fleet_scan_runs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Owners can view their fleet scans"
ON public.fleet_scan_runs FOR SELECT
TO authenticated
USING (public.has_role(auth.uid(), 'owner') AND owner_id = public.effective_owner(auth.uid()));

CREATE INDEX fleet_scan_runs_owner_kind_time_idx
  ON public.fleet_scan_runs (owner_id, kind, generated_at DESC);

-- Schedule hourly AI fleet scan via pg_cron -> public endpoint
CREATE EXTENSION IF NOT EXISTS pg_cron;
CREATE EXTENSION IF NOT EXISTS pg_net;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM cron.job WHERE jobname = 'fleet-ai-scan-hourly') THEN
    PERFORM cron.unschedule('fleet-ai-scan-hourly');
  END IF;
END $$;

SELECT cron.schedule(
  'fleet-ai-scan-hourly',
  '0 * * * *',
  $$
  SELECT net.http_post(
    url := 'https://mikrotikmagic.lovable.app/api/public/hooks/fleet-ai-scan',
    headers := jsonb_build_object(
      'Content-Type','application/json',
      'apikey','eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6Imt0eXR3d2xoeHhjZ2l3a2toeW1pIiwicm9sZSI6ImFub24iLCJpYXQiOjE3ODQ2MjEwODMsImV4cCI6MjEwMDE5NzA4M30.-3pFO8RA7FRhWklSyY1EO-4SITA8o6x-Xp47ln8txH0'
    ),
    body := '{}'::jsonb
  );
  $$
);