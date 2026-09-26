-- RLS layer backfill: bring an EXISTING database up to the schema authority
-- (database/init.sql lines ~358-461, 541-542).
--
-- Why: the RLS enforcement arc added the authenticated role, its tenant-
-- isolation policies, and its base table grants to init.sql, but they reach an
-- existing DB only if a migration carried them. A long-lived dev/prod DB seeded
-- before those lines exist has RLS ENABLED (the ALTERs landed) but NO policies
-- and NO grants: `SET LOCAL ROLE authenticated` then fails with
-- "permission denied for table ...", 500ing every WithTenant-scoped read.
--
-- This file is idempotent: policies are DROP-IF-EXISTS + CREATE, and GRANT is
-- naturally idempotent.

-- 0. Ensure the role exists (CREATE ROLE has no native IF NOT EXISTS).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        CREATE ROLE authenticated;
    END IF;
END;
$$;

-- 1. Tenant-isolation policies (mirror init.sql, DROP+CREATE for idempotency).
DROP POLICY IF EXISTS businesses_tenant_isolation ON businesses;
CREATE POLICY businesses_tenant_isolation ON businesses
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS business_metered_quotas_tenant_isolation ON business_metered_quotas;
CREATE POLICY business_metered_quotas_tenant_isolation ON business_metered_quotas
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS users_tenant_isolation ON users;
CREATE POLICY users_tenant_isolation ON users
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS devices_tenant_isolation ON devices;
CREATE POLICY devices_tenant_isolation ON devices
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS telemetry_tenant_isolation ON telemetry_samples;
CREATE POLICY telemetry_tenant_isolation ON telemetry_samples
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS heartbeats_tenant_isolation ON peer_heartbeats;
CREATE POLICY heartbeats_tenant_isolation ON peer_heartbeats
    FOR SELECT TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS vouchers_tenant_isolation ON vouchers;
CREATE POLICY vouchers_tenant_isolation ON vouchers
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS portal_tenant_isolation ON portal_configs;
CREATE POLICY portal_tenant_isolation ON portal_configs
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS portal_sessions_tenant_isolation ON portal_sessions;
CREATE POLICY portal_sessions_tenant_isolation ON portal_sessions
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS incidents_tenant_isolation ON incidents;
CREATE POLICY incidents_tenant_isolation ON incidents
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS alert_policies_tenant_isolation ON alert_policies;
CREATE POLICY alert_policies_tenant_isolation ON alert_policies
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS audit_tenant_isolation ON audit_events;
CREATE POLICY audit_tenant_isolation ON audit_events
    FOR SELECT TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS webhook_endpoints_tenant_isolation ON webhook_endpoints;
CREATE POLICY webhook_endpoints_tenant_isolation ON webhook_endpoints
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS webhook_deliveries_tenant_isolation ON webhook_deliveries;
CREATE POLICY webhook_deliveries_tenant_isolation ON webhook_deliveries
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

DROP POLICY IF EXISTS purchase_requests_tenant_isolation ON purchase_requests;
CREATE POLICY purchase_requests_tenant_isolation ON purchase_requests
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- 2. Base grants so `authenticated` can read/write tenant tables (RLS then
-- filters WHICH rows). GRANT is idempotent.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO authenticated;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO authenticated;