-- Requires guest_voucher_grants. First-use MAC remains immutable audit evidence;
-- this separate row tracks the current connection and cumulative consumption.
BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS guest_voucher_grants_tenant_key ON guest_voucher_grants(voucher_id,business_id);
CREATE TABLE IF NOT EXISTS guest_access_recovery (
 voucher_id uuid PRIMARY KEY,
 business_id uuid NOT NULL,
 device_id uuid NOT NULL,
 secret_hash bytea NOT NULL CHECK(octet_length(secret_hash)=32),
 current_mac macaddr NOT NULL,
 pending_mac macaddr,
 state text NOT NULL DEFAULT 'active' CHECK(state IN ('active','transferring','frozen','blocked')),
 transfer_id uuid,
 used_bytes bigint NOT NULL DEFAULT 0 CHECK(used_bytes>=0),
 transfer_count integer NOT NULL DEFAULT 0 CHECK(transfer_count BETWEEN 0 AND 3),
 window_started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(voucher_id,business_id) REFERENCES guest_voucher_grants(voucher_id,business_id),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id),
 CHECK((state='active' AND pending_mac IS NULL AND transfer_id IS NULL)
    OR (state<>'active' AND pending_mac IS NOT NULL AND transfer_id IS NOT NULL AND pending_mac<>current_mac))
);
ALTER TABLE guest_access_recovery ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_access_recovery FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS guest_access_recovery_tenant ON guest_access_recovery;
CREATE POLICY guest_access_recovery_tenant ON guest_access_recovery FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON guest_access_recovery TO authenticated;

CREATE OR REPLACE FUNCTION guard_guest_access_recovery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.voucher_id,NEW.business_id,NEW.device_id,NEW.secret_hash)
    IS DISTINCT FROM (OLD.voucher_id,OLD.business_id,OLD.device_id,OLD.secret_hash) THEN
  RAISE EXCEPTION 'recovery ownership and credential are immutable';
 END IF;
 IF NEW.used_bytes<OLD.used_bytes THEN
  RAISE EXCEPTION 'recovery cannot replenish consumed data';
 END IF;
 IF NEW.current_mac IS DISTINCT FROM OLD.current_mac AND NOT
    (OLD.state='frozen' AND NEW.state='active' AND NEW.current_mac=OLD.pending_mac) THEN
  RAISE EXCEPTION 'connection binding needs a verified frozen transfer';
 END IF;
 IF OLD.state='blocked' AND NEW.state<>'blocked' THEN
  RAISE EXCEPTION 'blocked recovery needs a separate audited repair workflow';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS guest_access_recovery_guard ON guest_access_recovery;
CREATE TRIGGER guest_access_recovery_guard BEFORE UPDATE ON guest_access_recovery
 FOR EACH ROW EXECUTE FUNCTION guard_guest_access_recovery();
COMMIT;
