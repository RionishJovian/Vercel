-- Current app passes are per login. Never import old Store purchases.
BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS users_user_business_key ON users(user_id,business_id);
CREATE TABLE app_account_passes (
 user_id uuid PRIMARY KEY,
 business_id uuid NOT NULL,
 plan text NOT NULL CHECK(plan IN ('monthly','annual')),
 activated_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL CHECK(expires_at>activated_at),
 revoked_at timestamptz,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 recorded_by uuid NOT NULL REFERENCES users(user_id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(user_id,business_id) REFERENCES users(user_id,business_id)
);
ALTER TABLE app_account_passes ENABLE ROW LEVEL SECURITY;
ALTER TABLE app_account_passes FORCE ROW LEVEL SECURITY;
CREATE POLICY app_account_passes_tenant_read ON app_account_passes FOR SELECT TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT ON app_account_passes TO authenticated;
CREATE TABLE app_account_pass_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(user_id),
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 actor_id uuid NOT NULL REFERENCES users(user_id),
 revision bigint NOT NULL,
 details jsonb NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE app_account_pass_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE app_account_pass_events FORCE ROW LEVEL SECURITY;
COMMIT;
