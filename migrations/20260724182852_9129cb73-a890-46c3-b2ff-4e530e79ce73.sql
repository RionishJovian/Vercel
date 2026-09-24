
CREATE INDEX IF NOT EXISTS idx_router_save_audit_created_at ON public.router_save_audit (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_router_save_audit_owner_created ON public.router_save_audit (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_scan_runs_owner_kind_gen ON public.fleet_scan_runs (owner_id, kind, generated_at DESC);

CREATE OR REPLACE FUNCTION private.prune_history()
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  DELETE FROM public.router_save_audit WHERE created_at < now() - interval '30 days';

  WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY owner_id, kind ORDER BY generated_at DESC) AS rn
    FROM public.fleet_scan_runs
  )
  DELETE FROM public.fleet_scan_runs f USING ranked r
  WHERE f.id = r.id AND r.rn > 30;
END; $$;

REVOKE ALL ON FUNCTION private.prune_history() FROM public, anon, authenticated;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_cron') THEN
    PERFORM cron.unschedule('prune-history-hourly')
      WHERE EXISTS (SELECT 1 FROM cron.job WHERE jobname = 'prune-history-hourly');
    PERFORM cron.schedule(
      'prune-history-hourly',
      '17 * * * *',
      $cron$ SELECT private.prune_history(); $cron$
    );
  END IF;
END $$;

SELECT private.prune_history();
