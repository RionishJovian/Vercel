-- Owner-selectable captive-portal entry methods.
-- Existing businesses keep both methods enabled, but the UI presents them as
-- separate screens (voucher first) instead of one combined, scrolling page.
ALTER TABLE portal_configs
    ADD COLUMN IF NOT EXISTS voucher_login_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS mobile_payment_enabled boolean NOT NULL DEFAULT true;

-- A portal with no usable entry path would strand guests. Keep the invariant
-- in the database as well as the API validation.
ALTER TABLE portal_configs
    DROP CONSTRAINT IF EXISTS portal_configs_access_method_check;
ALTER TABLE portal_configs
    ADD CONSTRAINT portal_configs_access_method_check
    CHECK (voucher_login_enabled OR mobile_payment_enabled);
