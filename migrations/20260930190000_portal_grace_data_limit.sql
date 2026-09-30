-- Add a per-business data cap to the bank-transfer grace session.
-- Time and data are enforced together; the first exhausted limit ends access.

ALTER TABLE portal_configs
    ADD COLUMN IF NOT EXISTS guest_grace_data_mb integer NOT NULL DEFAULT 50;

ALTER TABLE portal_configs
    DROP CONSTRAINT IF EXISTS portal_configs_guest_grace_data_mb_check;
ALTER TABLE portal_configs
    ADD CONSTRAINT portal_configs_guest_grace_data_mb_check
    CHECK (guest_grace_data_mb BETWEEN 1 AND 10240);

ALTER TABLE guest_purchase_requests
    ADD COLUMN IF NOT EXISTS grace_data_mb integer NOT NULL DEFAULT 50;

ALTER TABLE guest_purchase_requests
    DROP CONSTRAINT IF EXISTS guest_purchase_requests_grace_data_mb_check;
ALTER TABLE guest_purchase_requests
    ADD CONSTRAINT guest_purchase_requests_grace_data_mb_check
    CHECK (grace_data_mb BETWEEN 1 AND 10240);

CREATE INDEX IF NOT EXISTS idx_guest_requests_grace_client
    ON guest_purchase_requests (business_id, device_id, upper(client_mac), created_at DESC);
