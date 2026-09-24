-- Retire the Plus Tier Pass without deleting historical purchases or reducing
-- already-approved device allowances. Pending Plus requests are cancelled and
-- no new Plus order can enter any checkout path.

UPDATE public.service_purchases
SET status = 'rejected',
    reject_reason = 'Plus Tier Pass retired',
    decided_at = COALESCE(decided_at, now()),
    updated_at = now()
WHERE service_key = 'plus' AND status = 'pending';

UPDATE public.account_entitlements
SET plus = false,
    plus_since = NULL,
    updated_at = now()
WHERE plus = true;

CREATE OR REPLACE FUNCTION public.reject_retired_plus_service_purchase()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NEW.service_key = 'plus' THEN
    RAISE EXCEPTION 'PLUS_TIER_PASS_RETIRED' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_reject_retired_plus_service_purchase ON public.service_purchases;
CREATE TRIGGER trg_reject_retired_plus_service_purchase
  BEFORE INSERT OR UPDATE OF service_key ON public.service_purchases
  FOR EACH ROW EXECUTE FUNCTION public.reject_retired_plus_service_purchase();

REVOKE ALL ON FUNCTION public.reject_retired_plus_service_purchase() FROM PUBLIC, anon;
