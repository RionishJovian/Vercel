BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS devices_migration_tenant_identity ON devices(device_id,business_id);
CREATE TABLE hub_pending_enrollments (
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 device_id uuid NOT NULL,
 purpose text NOT NULL CHECK(purpose IN ('router_key','management')),
 ciphertext text NOT NULL,
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(business_id,device_id,purpose),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id) ON DELETE CASCADE
);
ALTER TABLE hub_pending_enrollments ENABLE ROW LEVEL SECURITY;
ALTER TABLE hub_pending_enrollments FORCE ROW LEVEL SECURITY;
-- Secrets are available only to the trusted coordinator, never the tenant SQL role.
REVOKE ALL ON hub_pending_enrollments FROM PUBLIC, authenticated;
COMMIT;
