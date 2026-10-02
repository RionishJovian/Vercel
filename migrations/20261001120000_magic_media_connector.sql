-- Opt-in Magic Media pilot. Apply explicitly before enabling media.connector_enabled.
BEGIN;
CREATE TABLE magic_media_connections (
 business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
 pairing_hash text UNIQUE, pairing_expires_at timestamptz,
 token_hash text UNIQUE, token_expires_at timestamptz,
 server_name text NOT NULL DEFAULT '', last_seen_at timestamptz,
 revoked_at timestamptz, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE magic_media_jobs (
 id uuid PRIMARY KEY, business_id uuid NOT NULL REFERENCES magic_media_connections(business_id) ON DELETE CASCADE,
 actor_id uuid NOT NULL REFERENCES users(user_id), operation text NOT NULL CHECK(operation IN ('status','libraries','add_library','scan')),
 payload jsonb NOT NULL DEFAULT '{}', result jsonb,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','succeeded','failed','expired','cancelled')),
 error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '2 minutes', started_at timestamptz,
 completed_at timestamptz, claimed_token_hash text
);
CREATE INDEX magic_media_jobs_tenant_recent ON magic_media_jobs(business_id,created_at DESC);
CREATE INDEX magic_media_jobs_queue ON magic_media_jobs(business_id,created_at) WHERE state='queued';
ALTER TABLE magic_media_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE magic_media_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE magic_media_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE magic_media_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY media_connections_tenant ON magic_media_connections
 USING (business_id = nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = nullif(current_setting('app.current_business_id',true),'')::uuid);
CREATE POLICY media_jobs_tenant ON magic_media_jobs
 USING (business_id = nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = nullif(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON magic_media_connections,magic_media_jobs TO authenticated;
COMMIT;
