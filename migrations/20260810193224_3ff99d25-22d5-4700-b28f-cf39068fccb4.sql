ALTER TABLE public.portal_plans
  ADD COLUMN IF NOT EXISTS data_quota_mb integer,
  ADD COLUMN IF NOT EXISTS validity_days integer,
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active';

ALTER TABLE public.portal_plans DROP CONSTRAINT IF EXISTS portal_plans_status_check;
ALTER TABLE public.portal_plans
  ADD CONSTRAINT portal_plans_status_check CHECK (status IN ('active','inactive'));