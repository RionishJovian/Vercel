-- Mega Magic Project - Database Initialization
-- This runs on first container startup

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- CORE TENANT ISOLATION
-- ============================================================================

-- Businesses (Tenants)
CREATE TABLE businesses (
    business_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_name text NOT NULL,
    owner_email text NOT NULL UNIQUE,
    access_tier text NOT NULL DEFAULT 'SMALL_WISP' 
        CHECK (access_tier IN ('SMALL_WISP', 'ENTERPRISE_ISP', 'GLOBAL_ADMIN')),
    is_active boolean NOT NULL DEFAULT true,
    device_quota_limit integer NOT NULL DEFAULT 10,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_businesses_owner_email ON businesses(owner_email);
CREATE INDEX idx_businesses_access_tier ON businesses(access_tier);

-- Metered Quotas (add-ons, WebFig passes, AI tokens)
CREATE TABLE business_metered_quotas (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    purchased_extra_slots integer NOT NULL DEFAULT 0,
    webfig_pass_expires_at timestamptz,
    ai_token_balance bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Users with RBAC
CREATE TABLE users (
    user_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    full_name text NOT NULL,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    user_role text NOT NULL DEFAULT 'VIEWER'
        CHECK (user_role IN ('OWNER', 'ADMIN', 'OPERATOR', 'TECHNICIAN', 'VIEWER', 'SUPPORT')),
    is_active boolean NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_business_id ON users(business_id);
CREATE INDEX idx_users_email ON users(email);

-- ============================================================================
-- DEVICE FLEET MANAGEMENT
-- ============================================================================

-- Devices with hierarchy support
CREATE TABLE devices (
    device_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    parent_gateway_id uuid REFERENCES devices(device_id) ON DELETE SET NULL,
    hardware_name text NOT NULL,
    device_model text,
    deployment_layer text NOT NULL DEFAULT 'MAIN_GATEWAY'
        CHECK (deployment_layer IN ('MAIN_GATEWAY', 'DOWNSTREAM_SWITCH', 'ACCESS_POINT', 'CONTROLLER')),
    vendor text NOT NULL DEFAULT 'mikrotik'
        CHECK (vendor IN ('mikrotik', 'ruijie', 'cisco', 'tplink', 'ubiquiti', 'generic')),
    category text NOT NULL DEFAULT 'router'
        CHECK (category IN ('router', 'gateway', 'switch', 'ap', 'controller')),
    transport text NOT NULL DEFAULT 'direct'
        CHECK (transport IN ('direct', 'connector', 'tunnel', 'controller')),
    mac_address macaddr,
    management_ip inet,
    is_online boolean NOT NULL DEFAULT false,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_devices_business_id ON devices(business_id);
CREATE INDEX idx_devices_parent_gateway ON devices(parent_gateway_id);
CREATE INDEX idx_devices_is_online ON devices(is_online);

-- ============================================================================
-- TELEMETRY & MONITORING
-- ============================================================================

-- Hardware telemetry samples (60s interval)
CREATE TABLE telemetry_samples (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid REFERENCES devices(device_id) ON DELETE CASCADE,
    subject_kind text NOT NULL 
        CHECK (subject_kind IN ('site', 'router', 'device', 'connector')),
    subject_id text NOT NULL,
    cpu_pct numeric(5,2),
    memory_pct numeric(5,2),
    latency_ms integer,
    uptime_seconds bigint,
    wan_state text CHECK (wan_state IN ('up', 'degraded', 'down', 'unknown')),
    connector_state text CHECK (connector_state IN ('online', 'stale', 'offline', 'none')),
    tunnel_state text CHECK (tunnel_state IN ('up', 'down', 'none')),
    observed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_telemetry_business_time ON telemetry_samples(business_id, observed_at DESC);
CREATE INDEX idx_telemetry_device_time ON telemetry_samples(device_id, observed_at DESC);
CREATE INDEX idx_telemetry_subject ON telemetry_samples(business_id, subject_kind, subject_id, observed_at DESC);

-- WireGuard peer heartbeats (25s interval)
CREATE TABLE peer_heartbeats (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    peer_id text NOT NULL,
    device_id uuid REFERENCES devices(device_id) ON DELETE SET NULL,
    handshake_at timestamptz,
    state text NOT NULL DEFAULT 'unknown'
        CHECK (state IN ('connected', 'disconnected', 'error', 'unknown')),
    rx_bytes bigint NOT NULL DEFAULT 0,
    tx_bytes bigint NOT NULL DEFAULT 0,
    observed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_heartbeats_business_time ON peer_heartbeats(business_id, observed_at DESC);
CREATE INDEX idx_heartbeats_peer ON peer_heartbeats(peer_id, observed_at DESC);

-- ============================================================================
-- VOUCHER SYSTEM
-- ============================================================================

CREATE TABLE vouchers (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid REFERENCES devices(device_id) ON DELETE SET NULL,
    plan_key text NOT NULL,
    plan_label text NOT NULL,
    code text NOT NULL,
    price_mmk integer NOT NULL DEFAULT 0,
    duration_minutes integer,
    device_mac macaddr,
    status text NOT NULL DEFAULT 'unused'
        CHECK (status IN ('unused', 'active', 'used', 'expired', 'revoked', 'banned')),
    first_seen_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id, code)
);

CREATE INDEX idx_vouchers_business_status ON vouchers(business_id, status, expires_at);
CREATE INDEX idx_vouchers_device_mac ON vouchers(device_mac);

-- ============================================================================
-- CAPTIVE PORTAL
-- ============================================================================

CREATE TABLE portal_configs (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    business_name text NOT NULL DEFAULT 'MikroTik Magic Wi-Fi',
    welcome_text text NOT NULL DEFAULT 'Enter your voucher code to connect.',
    terms_text text NOT NULL DEFAULT 'By connecting you agree to our terms.',
    primary_hex text NOT NULL DEFAULT '#ffb547',
    glass_tint_hex text NOT NULL DEFAULT '#7ad0ff',
    consent_required boolean NOT NULL DEFAULT true,
    consent_text text,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE portal_sessions (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid REFERENCES devices(device_id) ON DELETE SET NULL,
    voucher_id uuid REFERENCES vouchers(id) ON DELETE SET NULL,
    client_mac macaddr NOT NULL,
    client_ip inet,
    authenticated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    is_active boolean NOT NULL DEFAULT true,
    consent_given boolean NOT NULL DEFAULT false,
    consent_at timestamptz
);

CREATE INDEX idx_portal_sessions_business ON portal_sessions(business_id, authenticated_at DESC);
CREATE INDEX idx_portal_sessions_client_mac ON portal_sessions(client_mac);

-- ============================================================================
-- INCIDENTS & ALERTING
-- ============================================================================

CREATE TABLE alert_policies (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    kind text NOT NULL 
        CHECK (kind IN ('site_offline', 'wan_degraded', 'connector_stale', 'poe_ap_offline', 
                       'provisioning_failed', 'cpu_high', 'memory_high', 'handshake_stale')),
    enabled boolean NOT NULL DEFAULT true,
    severity text NOT NULL DEFAULT 'warning'
        CHECK (severity IN ('info', 'warning', 'critical')),
    threshold numeric(10,2),
    cooldown_minutes integer NOT NULL DEFAULT 30,
    notification_channels text[] DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id, kind)
);

CREATE TABLE incidents (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid REFERENCES devices(device_id) ON DELETE SET NULL,
    policy_id uuid REFERENCES alert_policies(id) ON DELETE SET NULL,
    kind text NOT NULL,
    severity text NOT NULL DEFAULT 'warning'
        CHECK (severity IN ('info', 'warning', 'critical')),
    subject_id text NOT NULL,
    subject_label text NOT NULL,
    detail text,
    opened_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    acknowledged_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    resolved_at timestamptz,
    resolved_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_incidents_business_open ON incidents(business_id, resolved_at) WHERE resolved_at IS NULL;
CREATE INDEX idx_incidents_device ON incidents(device_id, opened_at DESC);

-- ============================================================================
-- AUDIT LOGGING (Immutable)
-- ============================================================================

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(user_id) ON DELETE SET NULL,
    user_role text NOT NULL,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    before_state jsonb,
    after_state jsonb,
    reason text NOT NULL,
    ip_address inet,
    user_agent text,
    timestamp timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_business_time ON audit_events(business_id, timestamp DESC);
CREATE INDEX idx_audit_entity ON audit_events(entity_type, entity_id, timestamp DESC);
CREATE INDEX idx_audit_user_time ON audit_events(user_id, timestamp DESC);

-- ============================================================================
-- NATS SUBJECT PREFIXES (Per tenant isolation)
-- ============================================================================

CREATE TABLE nats_subjects (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    subject_prefix text NOT NULL UNIQUE,  -- e.g., "tenant.biz_123."
    created_at timestamptz NOT NULL DEFAULT now()
);

-- ============================================================================
-- ROW LEVEL SECURITY POLICIES
-- ============================================================================

-- Enable RLS on all tenant-scoped tables
ALTER TABLE businesses ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_metered_quotas ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE telemetry_samples ENABLE ROW LEVEL SECURITY;
ALTER TABLE peer_heartbeats ENABLE ROW LEVEL SECURITY;
ALTER TABLE vouchers ENABLE ROW LEVEL SECURITY;
ALTER TABLE portal_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE portal_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE incidents ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;

-- The `authenticated` role must exist BEFORE the policies that reference it.
-- `CREATE ROLE` has no native IF NOT EXISTS, so guard it with a DO block.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        CREATE ROLE authenticated;
    END IF;
END;
$$;

-- RLS Policies: Users can only access their business_id data
-- These assume a session variable `current_business_id` is set by middleware

-- Businesses: Users can only see their own business
CREATE POLICY businesses_tenant_isolation ON businesses
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Metered quotas: tenant-scoped. REQUIRED for the money path: the table had RLS
-- enabled but NO policy, which deny-alls writes for the `authenticated` role and
-- would break voucher redemption's entitlement grant.
CREATE POLICY business_metered_quotas_tenant_isolation ON business_metered_quotas
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Users: Users can only see users in their business
CREATE POLICY users_tenant_isolation ON users
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Devices: Users can only see devices in their business
CREATE POLICY devices_tenant_isolation ON devices
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Telemetry: Users can only see telemetry for their business
CREATE POLICY telemetry_tenant_isolation ON telemetry_samples
    FOR SELECT TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid);

-- Heartbeats: Users can only see heartbeats for their business
CREATE POLICY heartbeats_tenant_isolation ON peer_heartbeats
    FOR SELECT TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid);

-- Vouchers: Users can only see vouchers for their business
CREATE POLICY vouchers_tenant_isolation ON vouchers
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Portal: Users can only see portal data for their business
CREATE POLICY portal_tenant_isolation ON portal_configs
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY portal_sessions_tenant_isolation ON portal_sessions
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Incidents: Users can only see incidents for their business
CREATE POLICY incidents_tenant_isolation ON incidents
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Alert policies: tenant-scoped. REQUIRED by the operational alerting path: the
-- table had RLS enabled but NO policy (deny-all for `authenticated`), and
-- incidents reference alert_policies(policy_id).
CREATE POLICY alert_policies_tenant_isolation ON alert_policies
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Audit: Users can only see audit events for their business
CREATE POLICY audit_tenant_isolation ON audit_events
    FOR SELECT TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid);

-- ============================================================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER update_businesses_updated_at BEFORE UPDATE ON businesses
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_devices_updated_at BEFORE UPDATE ON devices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_vouchers_updated_at BEFORE UPDATE ON vouchers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_portal_configs_updated_at BEFORE UPDATE ON portal_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_alert_policies_updated_at BEFORE UPDATE ON alert_policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_incidents_updated_at BEFORE UPDATE ON incidents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- SEED DATA FOR DEVELOPMENT
-- ============================================================================

-- Global Admin Tenant
INSERT INTO businesses (business_id, company_name, owner_email, access_tier, device_quota_limit)
VALUES 
    ('00000000-0000-0000-0000-000000000001', 'Mega Magic Platform Admin', 'admin@megamagic.io', 'GLOBAL_ADMIN', 99999)
ON CONFLICT (business_id) DO NOTHING;

-- Small WISP Test Tenant
INSERT INTO businesses (business_id, company_name, owner_email, access_tier, device_quota_limit)
VALUES 
    ('00000000-0000-0000-0000-000000000002', 'Alpha Neighborhood WISP', 'owner@alphawisp.net', 'SMALL_WISP', 10)
ON CONFLICT (business_id) DO NOTHING;

-- Enterprise ISP Test Tenant
INSERT INTO businesses (business_id, company_name, owner_email, access_tier, device_quota_limit)
VALUES 
    ('00000000-0000-0000-0000-000000000003', 'Metro Carrier Networks', 'ceo@metrocarrier.com', 'ENTERPRISE_ISP', 100)
ON CONFLICT (business_id) DO NOTHING;

-- Metered quotas for test tenants
INSERT INTO business_metered_quotas (business_id, purchased_extra_slots, ai_token_balance)
VALUES 
    ('00000000-0000-0000-0000-000000000002', 0, 50000),
    ('00000000-0000-0000-0000-000000000003', 25, 2000000)
ON CONFLICT (business_id) DO NOTHING;

-- NATS subject prefixes
INSERT INTO nats_subjects (business_id, subject_prefix)
VALUES 
    ('00000000-0000-0000-0000-000000000001', 'tenant.global.'),
    ('00000000-0000-0000-0000-000000000002', 'tenant.biz_002.'),
    ('00000000-0000-0000-0000-000000000003', 'tenant.biz_003.')
ON CONFLICT (business_id) DO NOTHING;

-- Portal configs
INSERT INTO portal_configs (business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex)
VALUES 
    ('00000000-0000-0000-0000-000000000002', 'Alpha WISP Wi-Fi', 'Enter your voucher code from the front desk.', 'By connecting you agree to our fair-use policy.', '#ffb547', '#7ad0ff'),
    ('00000000-0000-0000-0000-000000000003', 'Metro Carrier Guest Wi-Fi', 'Welcome to Metro Carrier Networks.', 'Enterprise guest access terms apply.', '#0066cc', '#00ffff')
ON CONFLICT (business_id) DO NOTHING;

-- Grant permissions (the authenticated role is created above, before the policies)
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO authenticated;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO authenticated;