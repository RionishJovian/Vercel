-- Add payment evidence to current account-pass requests. Existing requests and
-- legacy Store history remain intact; no payment or pass is approved here.
BEGIN;
ALTER TABLE app_account_pass_renewals
 ADD COLUMN amount_mmk integer,
 ADD COLUMN bank_account_id uuid REFERENCES platform_service_bank_accounts(id),
 ADD COLUMN bank_reference text,
 ADD COLUMN bank_snapshot jsonb,
 ADD COLUMN idempotency_key uuid;
ALTER TABLE app_account_pass_renewals ADD CONSTRAINT account_pass_payment_evidence CHECK (
 (amount_mmk IS NULL AND bank_account_id IS NULL AND bank_reference IS NULL AND bank_snapshot IS NULL AND idempotency_key IS NULL)
 OR (amount_mmk IS NOT NULL AND amount_mmk BETWEEN 1 AND 1000000000 AND bank_reference IS NOT NULL AND length(btrim(bank_reference)) BETWEEN 1 AND 120
     AND bank_snapshot IS NOT NULL AND jsonb_typeof(bank_snapshot)='object' AND idempotency_key IS NOT NULL
     AND bank_account_id IS NOT NULL)
);
CREATE UNIQUE INDEX account_pass_payment_key ON app_account_pass_renewals(user_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
COMMIT;
