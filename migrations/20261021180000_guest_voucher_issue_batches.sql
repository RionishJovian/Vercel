-- Router installation is a durable operation, separate from guest first use.
-- Keep uncertain installations and their original voucher IDs for reconciliation.
BEGIN;
CREATE TABLE IF NOT EXISTS guest_voucher_issue_batches (
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 id uuid NOT NULL,
 device_id uuid NOT NULL,
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 plan_label text NOT NULL CHECK(length(plan_label) BETWEEN 1 AND 100),
 price_mmk integer NOT NULL CHECK(price_mmk BETWEEN 0 AND 1000000000),
 minutes integer NOT NULL CHECK(minutes BETWEEN 1 AND 525600),
 data_mb integer NOT NULL CHECK(data_mb BETWEEN 1 AND 1048576),
 total_count integer NOT NULL CHECK(total_count BETWEEN 1 AND 200),
 lease_token text,
 lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,id),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id),
 CHECK((lease_token IS NULL)=(lease_until IS NULL))
);
CREATE TABLE IF NOT EXISTS guest_voucher_issue_items (
 business_id uuid NOT NULL,
 batch_id uuid NOT NULL,
 voucher_id uuid NOT NULL,
 position integer NOT NULL CHECK(position BETWEEN 0 AND 199),
 install_state text NOT NULL DEFAULT 'pending' CHECK(install_state IN ('pending','unknown','verified')),
 verified_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,voucher_id),
 UNIQUE(business_id,batch_id,position),
 FOREIGN KEY(business_id,batch_id) REFERENCES guest_voucher_issue_batches(business_id,id),
 FOREIGN KEY(voucher_id,business_id) REFERENCES vouchers(id,business_id),
 CHECK((install_state='verified')=(verified_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS guest_voucher_issue_items_batch
 ON guest_voucher_issue_items(business_id,batch_id);

CREATE OR REPLACE FUNCTION guard_guest_voucher_issue_batch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.business_id,NEW.id,NEW.device_id,NEW.request_hash,NEW.plan_label,NEW.price_mmk,NEW.minutes,NEW.data_mb,NEW.total_count,NEW.created_at)
 IS DISTINCT FROM (OLD.business_id,OLD.id,OLD.device_id,OLD.request_hash,OLD.plan_label,OLD.price_mmk,OLD.minutes,OLD.data_mb,OLD.total_count,OLD.created_at) THEN
  RAISE EXCEPTION 'guest voucher issue batch is immutable';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS guest_voucher_issue_batch_guard ON guest_voucher_issue_batches;
CREATE TRIGGER guest_voucher_issue_batch_guard BEFORE UPDATE ON guest_voucher_issue_batches
 FOR EACH ROW EXECUTE FUNCTION guard_guest_voucher_issue_batch();

CREATE OR REPLACE FUNCTION guard_guest_voucher_issue_item() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.business_id,NEW.batch_id,NEW.voucher_id,NEW.position)
 IS DISTINCT FROM (OLD.business_id,OLD.batch_id,OLD.voucher_id,OLD.position) THEN
  RAISE EXCEPTION 'guest voucher issue membership is immutable';
 END IF;
 IF OLD.install_state='verified' AND (NEW.install_state,NEW.verified_at) IS DISTINCT FROM (OLD.install_state,OLD.verified_at) THEN
  RAISE EXCEPTION 'verified guest voucher installation cannot be reset';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS guest_voucher_issue_item_guard ON guest_voucher_issue_items;
CREATE TRIGGER guest_voucher_issue_item_guard BEFORE UPDATE ON guest_voucher_issue_items
 FOR EACH ROW EXECUTE FUNCTION guard_guest_voucher_issue_item();

ALTER TABLE guest_voucher_issue_batches ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_issue_batches FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS guest_voucher_issue_batches_tenant ON guest_voucher_issue_batches;
CREATE POLICY guest_voucher_issue_batches_tenant ON guest_voucher_issue_batches FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
ALTER TABLE guest_voucher_issue_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_issue_items FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS guest_voucher_issue_items_tenant ON guest_voucher_issue_items;
CREATE POLICY guest_voucher_issue_items_tenant ON guest_voucher_issue_items FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON guest_voucher_issue_batches,guest_voucher_issue_items TO authenticated;
COMMIT;
