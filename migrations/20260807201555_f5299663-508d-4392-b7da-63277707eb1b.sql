ALTER TABLE public.fleet_scan_runs
  ADD COLUMN IF NOT EXISTS triggered_by uuid REFERENCES auth.users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS fleet_scan_runs_triggered_by_idx
  ON public.fleet_scan_runs (triggered_by, generated_at DESC);