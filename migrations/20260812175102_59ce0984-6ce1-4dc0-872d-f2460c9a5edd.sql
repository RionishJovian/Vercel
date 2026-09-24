-- ============ A. Immutable voucher -> hotspot profile binding ============
ALTER TABLE public.voucher_codes
  ADD COLUMN IF NOT EXISTS plan_id uuid REFERENCES public.portal_plans(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS hotspot_profile text,
  ADD COLUMN IF NOT EXISTS profile_bound_at timestamptz,
  ADD COLUMN IF NOT EXISTS deploy_version integer;

CREATE INDEX IF NOT EXISTS voucher_codes_plan_id_idx ON public.voucher_codes (plan_id);

-- One-time best-effort backfill of the historical derivation for existing rows.
UPDATE public.voucher_codes
   SET hotspot_profile = 'mm-' || plan_key,
       profile_bound_at = created_at
 WHERE hotspot_profile IS NULL;

CREATE OR REPLACE FUNCTION public.voucher_binding_immutable()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  IF OLD.hotspot_profile IS NOT NULL AND NEW.hotspot_profile IS DISTINCT FROM OLD.hotspot_profile THEN
    RAISE EXCEPTION 'VOUCHER_BINDING_IMMUTABLE: hotspot_profile cannot change once a voucher is issued';
  END IF;
  IF OLD.plan_id IS NOT NULL AND NEW.plan_id IS DISTINCT FROM OLD.plan_id THEN
    RAISE EXCEPTION 'VOUCHER_BINDING_IMMUTABLE: plan_id cannot change once a voucher is issued';
  END IF;
  IF OLD.profile_bound_at IS NOT NULL AND NEW.profile_bound_at IS DISTINCT FROM OLD.profile_bound_at THEN
    NEW.profile_bound_at := OLD.profile_bound_at;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_voucher_binding_immutable ON public.voucher_codes;
CREATE TRIGGER trg_voucher_binding_immutable
  BEFORE UPDATE ON public.voucher_codes
  FOR EACH ROW EXECUTE FUNCTION public.voucher_binding_immutable();

-- ============ B. Authoritative Tier Pass Magic Points ledger ============
ALTER TABLE public.service_purchases
  ADD COLUMN IF NOT EXISTS refunded_at timestamptz,
  ADD COLUMN IF NOT EXISTS refunded_by uuid,
  ADD COLUMN IF NOT EXISTS refund_reason text;

ALTER TABLE public.service_purchases DROP CONSTRAINT IF EXISTS service_purchases_status_check;
ALTER TABLE public.service_purchases ADD CONSTRAINT service_purchases_status_check
  CHECK (status = ANY (ARRAY['pending','approved','rejected','cancelled','refunded']));

ALTER TABLE public.agent_points
  ALTER COLUMN points TYPE numeric(12,2);

ALTER TABLE public.agent_points
  ADD COLUMN IF NOT EXISTS service_purchase_id uuid REFERENCES public.service_purchases(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS service_key text,
  ADD COLUMN IF NOT EXISTS billing_period text,
  ADD COLUMN IF NOT EXISTS source_status text,
  ADD COLUMN IF NOT EXISTS reverses_point_id uuid REFERENCES public.agent_points(id) ON DELETE SET NULL;

ALTER TABLE public.agent_points DROP CONSTRAINT IF EXISTS agent_points_kind_check;
ALTER TABLE public.agent_points ADD CONSTRAINT agent_points_kind_check
  CHECK (kind = ANY (ARRAY['signup','renewal','tier_pass_award','tier_pass_reversal']));

-- Exactly one positive award per eligible approved purchase and billing period.
CREATE UNIQUE INDEX IF NOT EXISTS agent_points_award_once
  ON public.agent_points (service_purchase_id, billing_period)
  WHERE kind = 'tier_pass_award';

-- Exactly one compensating reversal per award.
CREATE UNIQUE INDEX IF NOT EXISTS agent_points_reversal_once
  ON public.agent_points (reverses_point_id)
  WHERE kind = 'tier_pass_reversal';

CREATE INDEX IF NOT EXISTS agent_points_agent_created_idx
  ON public.agent_points (agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS agent_points_purchase_idx
  ON public.agent_points (service_purchase_id);