BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS voucher_reseller_tenant_identity ON voucher_resellers(id,business_id);
CREATE UNIQUE INDEX IF NOT EXISTS reseller_actor_tenant_identity ON users(user_id,business_id);
CREATE TABLE guest_voucher_batch_resellers (
 business_id uuid NOT NULL,
 batch_id uuid NOT NULL,
 reseller_id uuid NOT NULL,
 shop_name text NOT NULL,
 code_prefix text NOT NULL,
 assigned_by uuid NOT NULL,
 assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,batch_id),
 FOREIGN KEY(business_id,batch_id) REFERENCES guest_voucher_issue_batches(business_id,id) ON DELETE CASCADE,
 FOREIGN KEY(reseller_id,business_id) REFERENCES voucher_resellers(id,business_id) ON DELETE RESTRICT,
 FOREIGN KEY(assigned_by,business_id) REFERENCES users(user_id,business_id)
);
CREATE TABLE guest_voucher_batch_reseller_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 business_id uuid NOT NULL,
 batch_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 previous_reseller_id uuid,
 reseller_id uuid,
 details jsonb NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(business_id,batch_id) REFERENCES guest_voucher_issue_batches(business_id,id) ON DELETE CASCADE,
 FOREIGN KEY(actor_id,business_id) REFERENCES users(user_id,business_id)
);
ALTER TABLE guest_voucher_batch_resellers ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_batch_resellers FORCE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_batch_reseller_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_batch_reseller_events FORCE ROW LEVEL SECURITY;
CREATE POLICY guest_batch_reseller_tenant ON guest_voucher_batch_resellers USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid) WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
CREATE POLICY guest_batch_reseller_events_tenant ON guest_voucher_batch_reseller_events USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid) WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
REVOKE ALL ON guest_voucher_batch_resellers,guest_voucher_batch_reseller_events FROM PUBLIC,authenticated;
GRANT SELECT,INSERT,UPDATE,DELETE ON guest_voucher_batch_resellers TO authenticated;
GRANT SELECT,INSERT ON guest_voucher_batch_reseller_events TO authenticated;
GRANT USAGE,SELECT ON SEQUENCE guest_voucher_batch_reseller_events_id_seq TO authenticated;
COMMIT;
