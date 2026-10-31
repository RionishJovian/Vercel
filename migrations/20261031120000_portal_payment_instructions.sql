-- Owner-written bank-transfer instructions shown on the guest payment screen.
-- The platform never takes payment; each owner describes their own account steps.
-- Operator-run: back up first, then apply once with psql -v ON_ERROR_STOP=1.
-- Existing table grants and RLS policies already cover the new column.
BEGIN;
ALTER TABLE portal_configs ADD COLUMN IF NOT EXISTS payment_instructions text;
COMMIT;
