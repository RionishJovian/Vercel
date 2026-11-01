-- Latest router-observed usage per guest Internet voucher (time and data used).
-- Operator-run, additive and idempotent. The app tolerates this table being
-- absent (usage is then read live only), so it may be applied before or after
-- the release that writes it. After applying, rerun deploy/runtime-db-role.sql
-- so the runtime role receives its service policy on the new table.
-- Rollback: DROP TABLE guest_voucher_usage;  (nothing else references it)
BEGIN;
CREATE TABLE IF NOT EXISTS guest_voucher_usage (
 business_id uuid NOT NULL,
 voucher_id uuid NOT NULL,
 device_id uuid NOT NULL,
 bytes_used bigint NOT NULL CHECK (bytes_used >= 0),
 uptime_seconds bigint NOT NULL CHECK (uptime_seconds >= 0),
 observed_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (business_id, voucher_id),
 FOREIGN KEY (voucher_id, business_id) REFERENCES vouchers(id, business_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS guest_voucher_usage_device ON guest_voucher_usage(business_id, device_id);
ALTER TABLE guest_voucher_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_voucher_usage FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS guest_voucher_usage_tenant ON guest_voucher_usage;
CREATE POLICY guest_voucher_usage_tenant ON guest_voucher_usage
 USING (business_id = NULLIF(current_setting('app.current_business_id', true), '')::uuid)
 WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id', true), '')::uuid);
REVOKE ALL ON guest_voucher_usage FROM PUBLIC, authenticated;
GRANT SELECT, INSERT, UPDATE ON guest_voucher_usage TO authenticated;
COMMIT;
