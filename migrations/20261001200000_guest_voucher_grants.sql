-- Guest internet is a distinct product, not an implicit WebFig/business pass.
-- Do not backfill legacy vouchers: their guest allowance cannot be inferred.
BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS vouchers_id_business_key ON vouchers(id,business_id);
CREATE UNIQUE INDEX IF NOT EXISTS devices_id_business_key ON devices(device_id,business_id);
CREATE TABLE IF NOT EXISTS guest_voucher_grants (
 voucher_id uuid PRIMARY KEY,
 business_id uuid NOT NULL,
 device_id uuid NOT NULL,
 minutes integer NOT NULL CHECK(minutes BETWEEN 1 AND 525600),
 data_mb integer NOT NULL CHECK(data_mb BETWEEN 1 AND 1048576),
 state text NOT NULL DEFAULT 'unused' CHECK(state IN ('unused','provisioning','active','expired','revoked')),
 client_mac macaddr,
 first_used_at timestamptz,
 expires_at timestamptz,
 lease_token text,
 lease_until timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(voucher_id,business_id) REFERENCES vouchers(id,business_id),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id),
 CHECK((state='unused' AND client_mac IS NULL AND first_used_at IS NULL AND expires_at IS NULL)
    OR (state<>'unused' AND client_mac IS NOT NULL AND first_used_at IS NOT NULL
        AND expires_at=first_used_at+minutes*interval '1 minute')),
 CHECK((state='provisioning' AND lease_token IS NOT NULL AND lease_until IS NOT NULL)
    OR (state<>'provisioning' AND lease_token IS NULL AND lease_until IS NULL))
);
CREATE INDEX IF NOT EXISTS guest_voucher_grants_expiry ON guest_voucher_grants(expires_at)
 WHERE state IN ('provisioning','active');
ALTER TABLE guest_voucher_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_grants FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS guest_voucher_grants_tenant ON guest_voucher_grants;
CREATE POLICY guest_voucher_grants_tenant ON guest_voucher_grants FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON guest_voucher_grants TO authenticated;

CREATE OR REPLACE FUNCTION guard_guest_voucher_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.voucher_id,NEW.business_id,NEW.device_id,NEW.minutes,NEW.data_mb)
    IS DISTINCT FROM (OLD.voucher_id,OLD.business_id,OLD.device_id,OLD.minutes,OLD.data_mb) THEN
  RAISE EXCEPTION 'guest voucher allowance snapshot is immutable';
 END IF;
 IF OLD.first_used_at IS NOT NULL AND
    (NEW.client_mac,NEW.first_used_at,NEW.expires_at)
    IS DISTINCT FROM (OLD.client_mac,OLD.first_used_at,OLD.expires_at) THEN
  RAISE EXCEPTION 'guest voucher binding and deadline are immutable';
 END IF;
 IF OLD.state IN ('expired','revoked') AND NEW.state<>OLD.state THEN
  RAISE EXCEPTION 'terminal guest voucher cannot be reactivated';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS guest_voucher_snapshot_guard ON guest_voucher_grants;
CREATE TRIGGER guest_voucher_snapshot_guard BEFORE UPDATE ON guest_voucher_grants
 FOR EACH ROW EXECUTE FUNCTION guard_guest_voucher_snapshot();
COMMIT;
