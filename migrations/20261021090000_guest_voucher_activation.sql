-- Record successful portal/router activation once. Existing active rows are
-- deliberately not backfilled: first-use reservations do not prove when the
-- router was successfully provisioned. This is voucher value, not cash settlement.
BEGIN;
ALTER TABLE guest_voucher_grants ADD COLUMN IF NOT EXISTS activated_at timestamptz;
CREATE INDEX IF NOT EXISTS guest_voucher_activation_business_time
 ON guest_voucher_grants(business_id,activated_at) WHERE activated_at IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_guest_voucher_activation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.activated_at IS NOT NULL AND NEW.activated_at IS DISTINCT FROM OLD.activated_at THEN
  RAISE EXCEPTION 'guest voucher activation time is immutable';
 END IF;
 IF OLD.activated_at IS NULL AND NEW.activated_at IS NOT NULL AND
    (OLD.state<>'provisioning' OR NEW.state<>'active' OR NEW.first_used_at IS NULL
     OR NEW.activated_at<NEW.first_used_at OR NEW.activated_at>=NEW.expires_at) THEN
  RAISE EXCEPTION 'guest voucher activation requires a live provisioning claim';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS guest_voucher_activation_guard ON guest_voucher_grants;
CREATE TRIGGER guest_voucher_activation_guard BEFORE UPDATE ON guest_voucher_grants
 FOR EACH ROW EXECUTE FUNCTION guard_guest_voucher_activation();
COMMIT;
