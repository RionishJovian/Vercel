-- Billing schema: bring existing DBs in line with init.sql.
--
-- The billing slice added the purchase_requests table and the portal_config
-- bank_* columns to init.sql but shipped NO migration, so persistent databases
-- (created before the slice) still lack them — every live portal/config call
-- 500s with "column bank_name does not exist". This idempotently applies the
-- same schema to existing DBs that init.sql provides to fresh ones.

-- 1. portal_configs: bank receiving details (shown on the guest store page).
ALTER TABLE portal_configs ADD COLUMN IF NOT EXISTS bank_name text;
ALTER TABLE portal_configs ADD COLUMN IF NOT EXISTS bank_account text;
ALTER TABLE portal_configs ADD COLUMN IF NOT EXISTS bank_holder text;

-- 2. purchase_requests: device-anchored, operator-approved billing requests.
CREATE TABLE IF NOT EXISTS purchase_requests (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid,
    plan_key text NOT NULL,
    amount int NOT NULL,
    bank_reference text NOT NULL CHECK (length(btrim(bank_reference)) > 0),
    duration_minutes int NOT NULL,
    receipt_path text,
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_purchase_requests_business
    ON purchase_requests(business_id, status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_purchase_requests_bank_ref_unique
    ON purchase_requests (business_id, lower(bank_reference));

-- 3. RLS: same policy init.sql applies. Postgres has no CREATE POLICY IF
-- NOT EXISTS, so drop-then-create for idempotency.
ALTER TABLE purchase_requests ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS purchase_requests_tenant_isolation ON purchase_requests;
CREATE POLICY purchase_requests_tenant_isolation ON purchase_requests
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid);