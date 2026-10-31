-- Store add-on catalog (platform managed) and per-business purchase requests.
-- Operator-run, not an automatic migration: back up first, apply once with
-- psql -v ON_ERROR_STOP=1, then rerun deploy/runtime-db-role.sql so the serving
-- login receives its grants and service policy for the new tables.
BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS reseller_actor_tenant_identity ON users(user_id,business_id);
CREATE TABLE store_addons (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 addon_key text NOT NULL UNIQUE CHECK (addon_key ~ '^[a-z0-9_]{3,40}$'),
 kind text NOT NULL CHECK (kind IN ('router_slot','server','feature')),
 name_en text NOT NULL CHECK (length(name_en) BETWEEN 1 AND 120),
 name_my text NOT NULL CHECK (length(name_my) BETWEEN 1 AND 120),
 description_en text NOT NULL DEFAULT '' CHECK (length(description_en) <= 600),
 description_my text NOT NULL DEFAULT '' CHECK (length(description_my) <= 600),
 price_mmk integer NOT NULL CHECK (price_mmk >= 0),
 period text NOT NULL CHECK (period IN ('one_time','monthly','annual')),
 active boolean NOT NULL DEFAULT true,
 sort_order integer NOT NULL DEFAULT 100,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE store_addon_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
 user_id uuid NOT NULL,
 addon_id uuid NOT NULL REFERENCES store_addons(id) ON DELETE RESTRICT,
 price_mmk integer NOT NULL CHECK (price_mmk >= 0),
 period text NOT NULL CHECK (period IN ('one_time','monthly','annual')),
 bank_reference text NOT NULL CHECK (length(bank_reference) BETWEEN 4 AND 80),
 idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 80),
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
 decision_note text NOT NULL DEFAULT '' CHECK (length(decision_note) <= 500),
 decided_by uuid,
 decided_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY (user_id,business_id) REFERENCES users(user_id,business_id),
 UNIQUE (business_id,idempotency_key)
);
CREATE UNIQUE INDEX store_addon_one_pending ON store_addon_requests(business_id,addon_id) WHERE status='pending';
CREATE INDEX store_addon_requests_status ON store_addon_requests(status,created_at DESC);
ALTER TABLE store_addon_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE store_addon_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY store_addon_requests_tenant ON store_addon_requests
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
REVOKE ALL ON store_addons,store_addon_requests FROM PUBLIC,authenticated;
GRANT SELECT ON store_addons TO authenticated;
GRANT SELECT,INSERT ON store_addon_requests TO authenticated;
COMMIT;
