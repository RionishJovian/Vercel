-- Voucher codes at rest, step 1 of 2 (expand). Additive: nothing is moved or dropped, and with the app flag
-- security.protect_voucher_codes off the application behaves exactly as before.
--
-- Why not a one-way hash: the router's HotSpot user name IS the voucher code. Sync, Kick, Ban, batch retry and
-- printing all need the code back, so the app keeps it recoverable, but only as ciphertext:
--   code_enc  AES-256-GCM (internal/vouchercode); the key never lives in this database.
--   code_hmac HMAC-SHA256 blind index (lowercase hex) so a typed code can be looked up without the plaintext.
-- With protection on, new rows have code NULL. A separate, explicit backfill (cmd/voucher-code-protect) converts
-- existing rows and only then clears their plaintext. Roll back by leaving the flag off; protected rows stay readable
-- as long as the app still holds the key.

ALTER TABLE public.vouchers ADD COLUMN IF NOT EXISTS code_hmac text;
ALTER TABLE public.vouchers ADD COLUMN IF NOT EXISTS code_enc text;

ALTER TABLE public.vouchers ALTER COLUMN code DROP NOT NULL;

ALTER TABLE public.vouchers
    ADD CONSTRAINT vouchers_code_present
    CHECK (code IS NOT NULL OR (code_hmac IS NOT NULL AND code_enc IS NOT NULL));

ALTER TABLE public.vouchers
    ADD CONSTRAINT vouchers_code_hmac_format
    CHECK (code_hmac IS NULL OR code_hmac ~ '^[0-9a-f]{64}$');

-- A protected code stays unique per business, exactly like the plaintext constraint it replaces.
CREATE UNIQUE INDEX IF NOT EXISTS vouchers_business_code_hmac_key
    ON public.vouchers (business_id, code_hmac)
    WHERE code_hmac IS NOT NULL;

COMMENT ON COLUMN public.vouchers.code IS 'Plaintext voucher code. NULL once the row is protected (code_enc + code_hmac).';
COMMENT ON COLUMN public.vouchers.code_enc IS 'AES-256-GCM sealed voucher code (see internal/vouchercode).';
COMMENT ON COLUMN public.vouchers.code_hmac IS 'HMAC-SHA256 blind index of the voucher code, lowercase hex.';
