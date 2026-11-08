-- Distinct tenant-scoped TP-Link Omada controller connection metadata.
-- Client secrets are AES-GCM encrypted before database insertion and never
-- returned by any owner API. No client is enrolled or used by default.
CREATE TABLE IF NOT EXISTS ap_omada_connections (
 id uuid NOT NULL DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 base_url text NOT NULL CHECK(length(base_url) BETWEEN 12 AND 256),
 omada_id text NOT NULL CHECK(omada_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 site_id text NOT NULL CHECK(site_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 client_id text NOT NULL CHECK(length(client_id) BETWEEN 1 AND 256),
 client_secret_ciphertext text NOT NULL CHECK(length(client_secret_ciphertext) BETWEEN 40 AND 8192),
 created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,id),
 UNIQUE(business_id,base_url,omada_id,site_id)
);
ALTER TABLE ap_omada_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE ap_omada_connections FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ap_omada_connections_tenant ON ap_omada_connections;
CREATE POLICY ap_omada_connections_tenant ON ap_omada_connections FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
REVOKE ALL ON ap_omada_connections FROM PUBLIC;
GRANT SELECT,INSERT,DELETE ON ap_omada_connections TO authenticated;
