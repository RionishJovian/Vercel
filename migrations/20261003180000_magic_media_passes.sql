-- Additive purchase ledger. An uploaded receipt or browser success URL is NOT
-- proof of payment. The authorized business owner reviews bank/MMQR receipts.
BEGIN;
CREATE TABLE magic_media_passes (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{64}$'),
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 guest_hash text NOT NULL CHECK(guest_hash ~ '^[0-9a-f]{64}$'),
 checkout_key text NOT NULL CHECK(checkout_key ~ '^[0-9a-f]{64}$'),
 publication_id text NOT NULL CHECK(publication_id ~ '^[0-9a-f]{64}$'),
 payment_details jsonb NOT NULL DEFAULT 'null'::jsonb,
 amount integer NOT NULL CHECK(amount BETWEEN 1 AND 1000000),
 currency text NOT NULL CHECK(currency='MMK'),
 duration_seconds integer NOT NULL CHECK(duration_seconds BETWEEN 3600 AND 2592000),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','paid','rejected','refunded')),
 provider text,
 payment_reference text,
 review_id uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 receipt_path text,
 receipt_hash text CHECK(receipt_hash IS NULL OR receipt_hash ~ '^[0-9a-f]{64}$'),
 receipt_submitted_at timestamptz,
 decided_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 payment_deadline timestamptz NOT NULL DEFAULT now()+interval '30 minutes',
 paid_at timestamptz,
 activated_at timestamptz,
 expires_at timestamptz,
 UNIQUE(business_id,guest_hash,checkout_key),
 UNIQUE(provider,payment_reference),
 CHECK((provider IS NULL)=(payment_reference IS NULL)),
 CHECK((receipt_path IS NULL)=(receipt_hash IS NULL)),
 CHECK((receipt_path IS NULL)=(receipt_submitted_at IS NULL)),
 CHECK(provider IS NULL OR (length(provider) BETWEEN 1 AND 40 AND length(payment_reference) BETWEEN 1 AND 200)),
 CHECK(state NOT IN ('paid','refunded') OR paid_at IS NOT NULL),
 CHECK((activated_at IS NULL)=(expires_at IS NULL)),
 CHECK(expires_at IS NULL OR expires_at=activated_at+duration_seconds*interval '1 second')
);
CREATE INDEX magic_media_passes_tenant_created ON magic_media_passes(business_id,created_at DESC);
ALTER TABLE magic_media_passes ENABLE ROW LEVEL SECURITY;
ALTER TABLE magic_media_passes FORCE ROW LEVEL SECURITY;
CREATE POLICY media_passes_tenant ON magic_media_passes
 USING (business_id=(SELECT nullif(current_setting('app.current_business_id',true),'')::uuid))
 WITH CHECK (business_id=(SELECT nullif(current_setting('app.current_business_id',true),'')::uuid));
GRANT SELECT,INSERT,UPDATE ON magic_media_passes TO authenticated;
COMMIT;
