-- Durable completion evidence allows exact cleanup after an ambiguous commit.
-- Never auto-apply this migration in production.
BEGIN;
ALTER TABLE guest_access_recovery ADD COLUMN IF NOT EXISTS completed_transfer_id uuid;
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
 IF NEW.completed_transfer_id IS DISTINCT FROM OLD.completed_transfer_id AND
    (OLD.state='frozen' AND NEW.state='active' AND NEW.completed_transfer_id=OLD.transfer_id
     AND NEW.current_mac=OLD.pending_mac) IS NOT TRUE THEN
  RAISE EXCEPTION 'completion receipt needs a verified frozen transfer';
 END IF;
 IF OLD.state='blocked' AND NEW.state<>'blocked' THEN
  RAISE EXCEPTION 'blocked recovery needs a separate audited repair workflow';
 END IF;
 RETURN NEW;
END $$;
COMMIT;
