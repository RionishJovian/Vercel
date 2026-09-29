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
        CHECK (user_role IN ('OWNER', 'ADMIN', 'OPERATOR', 'TECHNICIAN', 'VIEWER', 'SUPPORT', 'DEVELOPER')),
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
    -- Security-posture audit (C1): RouterOS version captured from the device by
    -- the prober; NULL until first observed. security_posture is
    -- safe|affected|unknown (fail-safe unknown).
    routeros_version text,
    routeros_channel text,
    version_checked_at timestamptz,
    security_posture text,
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
    bank_name text,
    bank_account text,
    bank_holder text,
    mmqr_url text,                -- owner's MMQR payment QR image (Myanmar QR)
    -- Guest wifi (Option C): the free online-payment window (minutes) and the
    -- data plans the owner sets on the portal (JSON: [{key,label,minutes,amount}]).
    guest_grace_minutes integer NOT NULL DEFAULT 10,
    guest_plans jsonb NOT NULL DEFAULT '[]'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ============================================================================
-- BILLING: device-anchored purchase requests (operator-approved)
-- ============================================================================

CREATE TABLE IF NOT EXISTS purchase_requests (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid REFERENCES devices(device_id) ON DELETE SET NULL,
    plan_key text NOT NULL,
    amount integer NOT NULL DEFAULT 0,
    bank_reference text NOT NULL CHECK (length(btrim(bank_reference)) > 0),
    duration_minutes integer NOT NULL CHECK (duration_minutes > 0),
    receipt_path text,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_purchase_requests_business ON purchase_requests(business_id, status, created_at DESC);

-- ============================================================================
-- C2: device config backups (per-device, tenant-scoped snapshots)
-- ============================================================================

CREATE TABLE IF NOT EXISTS device_config_backups (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    label text NOT NULL DEFAULT '',
    kind text NOT NULL DEFAULT 'snapshot'
        CHECK (kind IN ('export', 'snapshot')),
    content text NOT NULL DEFAULT '',
    size_bytes integer NOT NULL DEFAULT 0,
    created_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_device_backups_business_device ON device_config_backups(business_id, device_id, created_at DESC);

-- C2: per-device change-history timeline (tenant-scoped; the audit_events table
-- is a system-append trail with its own policy and is not the tenant timeline).
CREATE TABLE IF NOT EXISTS device_change_events (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    user_role text NOT NULL DEFAULT '',
    action text NOT NULL,
    reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_device_change_events_business_device ON device_change_events(business_id, device_id, created_at DESC);

-- The `authenticated` role must exist BEFORE any GRANT references it. `CREATE ROLE`
-- has no native IF NOT EXISTS, so guard it with a DO block. This block MUST stay
-- ABOVE the first GRANT below: a fresh (empty) database runs this file in order,
-- so a grant to a not-yet-created role aborts the whole init script.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        CREATE ROLE authenticated;
    END IF;
EXCEPTION
    -- Roles are cluster-wide. Parallel throwaway databases can both observe
    -- the role as absent, then race to create it. Either duplicate condition
    -- means another initializer won and the required role now exists.
    WHEN duplicate_object OR unique_violation THEN NULL;
END;
$$;

-- C2: explicit grants to the authenticated role. These are idempotent and cover
-- the C2 tables independently of the generic ALL-TABLES grant below, so an
-- incremental migration never leaves the backups/change APIs without access.
GRANT SELECT, INSERT, UPDATE, DELETE ON device_config_backups TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON device_change_events TO authenticated;
-- NOTE: the incidents grant intentionally lives in the incidents schema section
-- (after CREATE TABLE incidents) so a fresh init.sql load does not fail on a
-- grant-to-nonexistent-table.

-- One bank transfer (per business) may fund at most ONE live pass: a duplicate
-- bank reference on a non-rejected request is rejected at the DB level.
CREATE UNIQUE INDEX idx_purchase_requests_bank_ref_unique
    ON purchase_requests (business_id, lower(bank_reference))
    WHERE status <> 'rejected';

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
    suggested_action jsonb,
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
GRANT SELECT, INSERT, UPDATE, DELETE ON incidents TO authenticated;

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
-- WEBHOOK NOTIFICATIONS
-- Tenant-configured endpoints + delivery audit log. Mirrors
-- migrations/20260924000000_webhook_notifications.sql (schema-authority drift
-- fix: these tables were missing from init.sql).
-- ============================================================================

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    url text NOT NULL CHECK (url ~ '^https?://'),
    secret text,                    -- HMAC signing secret (optional)
    event_types text[] NOT NULL DEFAULT '{incident.opened, incident.resolved}',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id, url)
);

CREATE INDEX idx_webhook_endpoints_business ON webhook_endpoints(business_id);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    webhook_id uuid REFERENCES webhook_endpoints(id) ON DELETE SET NULL,
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    last_status_code integer,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    delivered_at timestamptz
);

CREATE INDEX idx_webhook_deliveries_business ON webhook_deliveries(business_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status);
CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id);

-- ============================================================================
-- GUEST WIFI (Option C)
-- ============================================================================
-- Each business owns the Telegram bot that carries their GUEST wifi loop
-- (notify + approve/reject), bound by the owner under their platform account.
CREATE TABLE IF NOT EXISTS business_telegram_bots (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    bot_token text,                 -- stored masked/redacted on read-back
    target_chat_id bigint,          -- operator chat for this bot
    enabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Guest online-banking purchase submit: a guest on the captive portal picks a
-- data plan, gets a free grace window to pay, attaches a receipt, and the
-- owner's bot carries the approve/reject. Approved => wifi time extension per
-- the portal guest plan.
CREATE TABLE IF NOT EXISTS guest_purchase_requests (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid,                 -- device the guest is attached to
    client_mac text,
    client_ip inet,
    plan_key text NOT NULL,
    amount integer NOT NULL DEFAULT 0,
    data_mb integer NOT NULL DEFAULT 0,   -- plan data allowance (MB); 0 = time-only
    paid_phone text,                     -- MMQR: payer phone submitted after payment
    tx_last6 text,                       -- MMQR: last 6 digits of bank/tx number
    bank_reference text,
    receipt_path text,              -- uploaded successful-payment receipt
    grace_started_at timestamptz,
    grace_expires_at timestamptz,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_guest_requests_business ON guest_purchase_requests(business_id);
CREATE INDEX idx_guest_requests_status ON guest_purchase_requests(status);

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
ALTER TABLE webhook_endpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_config_backups ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_change_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_telegram_bots ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_purchase_requests ENABLE ROW LEVEL SECURITY;

-- NOTE: this role is CREATED EARLIER in this file, immediately before the first
-- GRANT that references it. It is guaranteed to exist by this point.

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

-- Telemetry: tenant-scoped. Covers WRITES too (ingest): the original FOR SELECT
-- policy deny-all'd inserts for `authenticated`, which would break telemetry
-- ingest the moment enforcement applies.
CREATE POLICY telemetry_tenant_isolation ON telemetry_samples
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

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

-- Webhooks: tenant-scoped endpoints + delivery log.
CREATE POLICY webhook_endpoints_tenant_isolation ON webhook_endpoints
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY webhook_deliveries_tenant_isolation ON webhook_deliveries
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Billing: tenant-scoped purchase requests.
CREATE POLICY purchase_requests_tenant_isolation ON purchase_requests
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- C2: tenant-scoped device config backups.
CREATE POLICY device_config_backups_tenant_isolation ON device_config_backups
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- C2: tenant-scoped device change history.
CREATE POLICY device_change_events_tenant_isolation ON device_change_events
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- Guest wifi (Option C): owner bot binding + guest purchase requests, both
-- tenant-scoped so each owner sees/decides only their own guest loop.
CREATE POLICY business_telegram_bots_tenant_isolation ON business_telegram_bots
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY guest_requests_tenant_isolation ON guest_purchase_requests
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

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
GRANT SELECT, INSERT, UPDATE, DELETE ON business_telegram_bots TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON guest_purchase_requests TO authenticated;

-- ============================================================================
-- PHASE 2 — ported Mikromagic features
-- ============================================================================
-- These tables have no equivalent in the original 20-table schema. They are
-- ported from the old Supabase DDL, re-expressed in this file's conventions:
--   * `owner_id` (old auth user id) -> `business_id`  (the tenant key)
--   * `auth.users(id)` FKs           -> users(user_id) / businesses(business_id)
--   * `auth.uid()` RLS + helpers      -> the business-scoped _tenant_isolation policy
--   * Supabase `SECURITY DEFINER` RPCs -> dropped; the Go backend owns that logic
-- Appended at the end so the `authenticated` role (created earlier) exists; the
-- blanket GRANT above ran before these tables existed, so they grant explicitly.

-- ---- Magic Coins -----------------------------------------------------------
-- Per-account, append-only wallet. 1 Magic Coin = 1 MMK. `balance` is a cached
-- total guarded by a row lock; `magic_coin_transactions` is the source of truth.
CREATE TABLE IF NOT EXISTS magic_coin_wallets (
    user_id uuid PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    balance numeric(12,2) NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS magic_coin_transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    delta numeric(12,2) NOT NULL CHECK (delta <> 0),
    balance_after numeric(12,2) NOT NULL,
    kind text NOT NULL CHECK (kind IN (
        'agent_earn', 'agent_reversal', 'service_payment', 'service_refund',
        'feature_key_purchase', 'purchase_credit',
        'manual_credit', 'manual_debit'
    )),
    service_purchase_id uuid REFERENCES purchase_requests(id) ON DELETE SET NULL,
    note text NOT NULL,
    actor_user_id uuid REFERENCES users(user_id) ON DELETE SET NULL,
    -- Unlock-key purchases paid with coins. These point at Bucket B tables that
    -- are not ported yet, so they stay plain uuids; their FKs land with those tables.
    webfig_key_activation_id        uuid,
    reseller_add_key_activation_id  uuid,
    router_unlock_key_activation_id uuid,
    magic_dude_unlock_activation_id uuid,
    switch_unlock_key_activation_id uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_tx_service_payment_once
    ON magic_coin_transactions(service_purchase_id) WHERE kind = 'service_payment';
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_tx_service_refund_once
    ON magic_coin_transactions(service_purchase_id) WHERE kind = 'service_refund';
CREATE INDEX IF NOT EXISTS magic_coin_tx_user_created_idx
    ON magic_coin_transactions(user_id, created_at DESC);

-- The ledger is immutable: a spent/earned coin can never be edited or erased.
CREATE OR REPLACE FUNCTION magic_coin_transaction_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = public AS $$
BEGIN
    RAISE EXCEPTION 'MAGIC_COIN_TRANSACTION_IMMUTABLE';
END;
$$;
DROP TRIGGER IF EXISTS trg_magic_coin_transaction_immutable ON magic_coin_transactions;
CREATE TRIGGER trg_magic_coin_transaction_immutable
    BEFORE UPDATE OR DELETE ON magic_coin_transactions
    FOR EACH ROW EXECUTE FUNCTION magic_coin_transaction_immutable();

-- Every account gets a wallet on creation (the old app did this via a profiles trigger).
CREATE OR REPLACE FUNCTION create_magic_coin_wallet_for_user()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
BEGIN
    INSERT INTO magic_coin_wallets (user_id, business_id)
    VALUES (NEW.user_id, NEW.business_id)
    ON CONFLICT (user_id) DO NOTHING;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_user_magic_coin_wallet ON users;
CREATE TRIGGER trg_user_magic_coin_wallet
    AFTER INSERT ON users
    FOR EACH ROW EXECUTE FUNCTION create_magic_coin_wallet_for_user();

GRANT SELECT, INSERT, UPDATE, DELETE ON magic_coin_wallets TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON magic_coin_transactions TO authenticated;

ALTER TABLE magic_coin_wallets ENABLE ROW LEVEL SECURITY;
ALTER TABLE magic_coin_transactions ENABLE ROW LEVEL SECURITY;

CREATE POLICY magic_coin_wallets_tenant_isolation ON magic_coin_wallets
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY magic_coin_transactions_tenant_isolation ON magic_coin_transactions
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- ---- Unlock-key activations ------------------------------------------------
-- The old product had FIVE near-identical tables (webfig / magic_dude / router /
-- switch / reseller), differing only in hardcoded constants (key_id, key_name,
-- price_coins) enforced by rigid CHECKs. Consolidated into ONE table with a
-- `key_kind` discriminator — the same consolidation this schema already applies
-- to audit_events. The per-key constants live in the data, not the schema.
CREATE TABLE IF NOT EXISTS unlock_key_activations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    key_kind text NOT NULL CHECK (key_kind IN ('webfig','magic_dude','router','switch','reseller')),
    key_id integer,                           -- absent on magic_dude activations
    key_name text,                            -- absent on magic_dude activations
    price_coins numeric(12,2) NOT NULL,
    attributes text NOT NULL,
    account_bound boolean NOT NULL DEFAULT true,
    non_transferable boolean NOT NULL DEFAULT true,
    -- router keys are single-use against a specific router; others use consumed_ref_id
    unique_router_key boolean NOT NULL DEFAULT false,
    grace_days integer,                       -- magic_dude: expires_at = created_at + N days
    consumed_at timestamptz,
    consumed_ref_id uuid,                     -- the router/switch/reseller it was spent on
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at),
    CHECK ((consumed_at IS NULL) = (consumed_ref_id IS NULL))
);
CREATE INDEX IF NOT EXISTS unlock_key_activations_user_idx ON unlock_key_activations(user_id, created_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON unlock_key_activations TO authenticated;
ALTER TABLE unlock_key_activations ENABLE ROW LEVEL SECURITY;
CREATE POLICY unlock_key_activations_tenant_isolation ON unlock_key_activations
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- ---- Account entitlements --------------------------------------------------
-- Which service tier an account holds. Old `app_role`-free; carried verbatim.
CREATE TABLE IF NOT EXISTS account_entitlements (
    user_id uuid PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    tier text NOT NULL CHECK (tier IN ('spark','trial','monthly','annual')),
    tier_expires_at timestamptz,
    plus boolean NOT NULL DEFAULT false,
    plus_since timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON account_entitlements TO authenticated;
ALTER TABLE account_entitlements ENABLE ROW LEVEL SECURITY;
CREATE POLICY account_entitlements_tenant_isolation ON account_entitlements
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- ---- Operator feature grants / role defaults -------------------------------
-- The feature-flag system the new backend does not have yet. Ported so the data
-- survives; `legacy_role` keeps the original old `app_role` value because the
-- old vocabulary (client/agent/site_manager) collapses onto the new one.
CREATE TABLE IF NOT EXISTS operator_feature_grants (
    user_id uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    feature text NOT NULL CHECK (feature IN (
        'vouchers','portal_deploy','cash_sales','bank_edit','router_config',
        'reboot','alerts','syslog_ai','poe','telegram')),
    granted_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feature)
);

CREATE TABLE IF NOT EXISTS operator_feature_role_defaults (
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER')),
    feature text NOT NULL CHECK (feature IN (
        'vouchers','portal_deploy','cash_sales','bank_edit','router_config',
        'reboot','alerts','syslog_ai','poe','telegram')),
    -- The old vocabulary was finer-grained than the new one: `agent` AND
    -- `site_manager` both map to OPERATOR, so keying on (role, feature) would
    -- silently merge them and drop rows. Key on the ORIGINAL pair to preserve
    -- every default; `role` carries the mapping for the app to group by.
    legacy_role text NOT NULL,
    updated_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (legacy_role, feature)
);

CREATE TABLE IF NOT EXISTS portal_mode_role_defaults (
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER')),
    mode text NOT NULL CHECK (mode IN ('hybrid_light','commerce')),
    legacy_role text NOT NULL,               -- see note above
    updated_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (legacy_role, mode)
);

-- Role defaults are platform-wide (not tenant data), so they are readable by any
-- authenticated account and carry no business_id / RLS policy.
GRANT SELECT, INSERT, UPDATE, DELETE ON operator_feature_grants TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON operator_feature_role_defaults TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON portal_mode_role_defaults TO authenticated;

ALTER TABLE operator_feature_grants ENABLE ROW LEVEL SECURITY;
CREATE POLICY operator_feature_grants_tenant_isolation ON operator_feature_grants
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- ---- Sites -----------------------------------------------------------------
-- The old product grouped devices by "site"; in the old single-tenant model one
-- site == one owner, so today each imported site corresponds to a business. The
-- table is still worth having: a WISP legitimately runs several sites per
-- business, the SPA ships a Sites view, and the geo/timezone data has no other home.
CREATE TABLE IF NOT EXISTS sites (
    site_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    name text NOT NULL,
    location text,
    timezone text,
    latitude numeric(9,6),
    longitude numeric(9,6),
    notes text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sites_business_idx ON sites(business_id);

-- ---- Router connections ----------------------------------------------------
-- How to reach a router (host/port/TLS) plus its tunnel/cloud state.
-- SECURITY: `legacy_password_ciphertext` is the OLD Supabase-encrypted secret.
-- It is carried so nothing is destroyed, but it was encrypted with the previous
-- stack's key and CANNOT be decrypted here — the credential must be re-entered.
-- The old tunnel private keys were never populated (0 rows), so there is nothing
-- to carry for them.
CREATE TABLE IF NOT EXISTS router_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    name text NOT NULL,
    host text,
    port integer,
    use_tls boolean NOT NULL DEFAULT true,
    is_default boolean NOT NULL DEFAULT false,
    site_id uuid REFERENCES sites(site_id) ON DELETE SET NULL,
    connection_mode text,
    cloud_status text,
    cloud_last_seen_at timestamptz,
    environment text,
    legacy_password_ciphertext text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS router_connections_business_idx ON router_connections(business_id);

-- ---- Site topology config --------------------------------------------------
-- Per (site, router): the port labels and downstream devices shown on the
-- topology canvas.
CREATE TABLE IF NOT EXISTS site_topology_config (
    site_id uuid NOT NULL REFERENCES sites(site_id) ON DELETE CASCADE,
    router_id uuid NOT NULL REFERENCES router_connections(id) ON DELETE CASCADE,
    port_labels jsonb NOT NULL DEFAULT '[]'::jsonb,
    downstream_devices jsonb NOT NULL DEFAULT '[]'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, router_id)
);

-- devices gains the links it used to have in the old schema. The new device
-- hierarchy (parent_gateway_id / deployment_layer) is untouched; these are added
-- alongside it so migrated devices keep their site and connection.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS site_id uuid REFERENCES sites(site_id) ON DELETE SET NULL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS router_connection_id uuid REFERENCES router_connections(id) ON DELETE SET NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON sites TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON router_connections TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON site_topology_config TO authenticated;

ALTER TABLE sites ENABLE ROW LEVEL SECURITY;
ALTER TABLE router_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE site_topology_config ENABLE ROW LEVEL SECURITY;

CREATE POLICY sites_tenant_isolation ON sites
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY router_connections_tenant_isolation ON router_connections
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- topology config has no business_id of its own: scope it through its site.
CREATE POLICY site_topology_config_tenant_isolation ON site_topology_config
    FOR ALL TO authenticated
    USING (site_id IN (SELECT s.site_id FROM sites s
                        WHERE s.business_id = current_setting('app.current_business_id')::uuid))
    WITH CHECK (site_id IN (SELECT s.site_id FROM sites s
                        WHERE s.business_id = current_setting('app.current_business_id')::uuid));

-- ---- Terminal templates / history -----------------------------------------
-- Saved RouterOS REST calls, and an audit of what was run. The new backend has
-- no terminal yet; ported so the 102 templates and their history survive.
CREATE TABLE IF NOT EXISTS terminal_templates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    created_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    name text NOT NULL,
    description text,
    category text NOT NULL,
    method text NOT NULL CHECK (method IN ('GET','POST','PATCH','PUT','DELETE')),
    path text NOT NULL CHECK (path LIKE '/%'),
    body text,
    is_builtin boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS terminal_templates_business_idx ON terminal_templates(business_id, category);

CREATE TABLE IF NOT EXISTS terminal_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(user_id) ON DELETE SET NULL,
    router_connection_id uuid REFERENCES router_connections(id) ON DELETE SET NULL,
    site_id uuid REFERENCES sites(site_id) ON DELETE SET NULL,
    method text NOT NULL,
    path text NOT NULL,
    body text,
    status integer,
    ms integer,
    response_snippet text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS terminal_history_business_idx ON terminal_history(business_id, created_at DESC);

-- ---- Voucher resellers -----------------------------------------------------
CREATE TABLE IF NOT EXISTS voucher_resellers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    shop_name text NOT NULL CHECK (char_length(shop_name) BETWEEN 1 AND 120),
    contact_name text NOT NULL CHECK (char_length(contact_name) BETWEEN 1 AND 120),
    location text,
    phone text,
    active boolean NOT NULL DEFAULT true,
    code_prefix text NOT NULL CHECK (
        char_length(code_prefix) BETWEEN 2 AND 4
        AND code_prefix = upper(code_prefix)
        AND code_prefix ~ '^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]+$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- A code prefix must be unique within a business, compared case-insensitively.
-- Previously upheld only in the repository (which returns 409); the ported data
-- satisfies it, so the database can now hold the invariant itself.
CREATE UNIQUE INDEX IF NOT EXISTS voucher_resellers_business_prefix_unique
    ON voucher_resellers (business_id, upper(code_prefix));

-- ---- Voucher print layouts -------------------------------------------------
-- One row per business. The old CHECKs are real print/UI constraints, so they
-- are carried over verbatim rather than loosened.
CREATE TABLE IF NOT EXISTS voucher_print_layouts (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    business_name text NOT NULL CHECK (char_length(business_name) BETWEEN 1 AND 80),
    wifi_name text NOT NULL CHECK (char_length(wifi_name) BETWEEN 1 AND 80),
    support_contact text NOT NULL CHECK (char_length(support_contact) BETWEEN 1 AND 120),
    terms text NOT NULL CHECK (char_length(terms) BETWEEN 1 AND 600),
    paper_width_mm integer NOT NULL CHECK (paper_width_mm IN (58, 80)),
    show_qr boolean NOT NULL DEFAULT true,
    show_price boolean NOT NULL DEFAULT true,
    show_expiry boolean NOT NULL DEFAULT true,
    a4_columns integer NOT NULL CHECK (a4_columns IN (2, 3, 4, 5)),
    a4_rows_per_page integer NOT NULL CHECK (a4_rows_per_page BETWEEN 4 AND 50),
    thermal_columns integer NOT NULL CHECK (thermal_columns IN (1, 2)),
    thermal_rows_per_strip integer NOT NULL CHECK (thermal_rows_per_strip BETWEEN 4 AND 40),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Pricing promo (platform singleton) ------------------------------------
-- `id boolean PRIMARY KEY CHECK (id)` makes this structurally a one-row table.
-- NOTE ON THE PORTED DATA (read before building a screen or activating a promo):
-- the single migrated row's own label reads "30% off Monthly & Annual", but its
-- monthly_promo_mmk (66,500) is ~1.9x monthly_standard_mmk (35,000) and the annual
-- pair is ~2.1x. Either the promo/standard columns were transposed in the old app,
-- or "promo" meant something other than a discount there. It cannot be resolved from
-- the data, and inventing corrected prices in a billing table would be worse than
-- carrying a suspicious value, so the row is migrated exactly as the old system stored
-- it. Harmless today: active = false and ends_on is in the past. Fix the values (or
-- the semantics) before ever setting active = true — a promo priced above the standard
-- rate would overcharge.
CREATE TABLE IF NOT EXISTS pricing_promo (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    active boolean NOT NULL DEFAULT false,
    starts_on date,
    ends_on date,
    label text,
    monthly_promo_mmk integer,
    monthly_standard_mmk integer,
    annual_promo_mmk integer,
    annual_standard_mmk integer,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Ruijie cloud connections ----------------------------------------------
-- Same secret handling as router_connections: the stored app id/secret are OLD
-- stack ciphertext and cannot be decrypted here, so they are labelled legacy and
-- the values must be re-entered.
CREATE TABLE IF NOT EXISTS ruijie_cloud_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    region text,
    base_url text,
    legacy_app_id_ciphertext text,
    legacy_app_secret_ciphertext text,
    site_ref text,
    device_ref text,
    enabled boolean NOT NULL DEFAULT true,
    last_test_at timestamptz,
    last_test_status text,
    last_sync_at timestamptz,
    created_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    token_path text,
    device_path text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Magic Dude knowledge base ---------------------------------------------
-- knowledge_notes is platform-wide editorial content (no tenant scope), so it is
-- granted but not RLS-scoped — like the role defaults.
CREATE TABLE IF NOT EXISTS magic_dude_knowledge_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title text NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
    body_md text NOT NULL CHECK (char_length(btrim(body_md)) BETWEEN 1 AND 20000),
    sort integer NOT NULL DEFAULT 0,
    active boolean NOT NULL DEFAULT true,
    updated_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS magic_dude_solved_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(user_id) ON DELETE SET NULL,
    question text NOT NULL CHECK (char_length(btrim(question)) BETWEEN 1 AND 4000),
    answer text NOT NULL CHECK (char_length(btrim(answer)) BETWEEN 1 AND 8000),
    language text CHECK (language IS NULL OR language IN ('en','my','zh')),
    helpful_count integer NOT NULL DEFAULT 1 CHECK (helpful_count >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON terminal_templates TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON terminal_history TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON voucher_resellers TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON voucher_print_layouts TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON pricing_promo TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON ruijie_cloud_connections TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON magic_dude_knowledge_notes TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON magic_dude_solved_cases TO authenticated;

ALTER TABLE terminal_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE terminal_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE voucher_resellers ENABLE ROW LEVEL SECURITY;
ALTER TABLE voucher_print_layouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE ruijie_cloud_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE magic_dude_solved_cases ENABLE ROW LEVEL SECURITY;

CREATE POLICY terminal_templates_tenant_isolation ON terminal_templates
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY terminal_history_tenant_isolation ON terminal_history
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY voucher_resellers_tenant_isolation ON voucher_resellers
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY voucher_print_layouts_tenant_isolation ON voucher_print_layouts
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY ruijie_cloud_connections_tenant_isolation ON ruijie_cloud_connections
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY magic_dude_solved_cases_tenant_isolation ON magic_dude_solved_cases
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- ---- i18n overrides (platform-wide) ----------------------------------------
-- UI/copy translation overrides. No owner_id in the old schema => platform-wide
-- content, so it is granted but not RLS-scoped.
CREATE TABLE IF NOT EXISTS i18n_overrides (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    locale text NOT NULL CHECK (locale IN ('en','zh','my')),
    ns text NOT NULL CHECK (ns IN ('ui','copy')),
    key text NOT NULL,
    value text NOT NULL,
    updated_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (locale, ns, key)
);

-- ---- Owner daily closes ----------------------------------------------------
CREATE TABLE IF NOT EXISTS owner_daily_closes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    site_id uuid REFERENCES sites(site_id) ON DELETE SET NULL,
    router_connection_id uuid REFERENCES router_connections(id) ON DELETE SET NULL,
    business_day date NOT NULL,
    counted_cash_mmk integer NOT NULL DEFAULT 0,
    note text,
    closed_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    closed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS owner_daily_closes_business_idx ON owner_daily_closes(business_id, business_day DESC);
-- One close per business per DAY. This is a cash ledger: without the constraint a
-- second close for the same day is accepted and that day can be counted twice.
-- The ported data satisfies it (30 rows, 30 distinct pairs) — verified before the
-- index was added, not assumed.
CREATE UNIQUE INDEX IF NOT EXISTS owner_daily_closes_business_day_unique
    ON owner_daily_closes (business_id, business_day);

-- ---- Device allowances -----------------------------------------------------
-- NOTE: this is finer-grained than business_metered_quotas (extra slots) and
-- businesses.device_quota_limit (a single total): it counts routers, controllers
-- and sites separately. Kept as its own table rather than forced into either.
CREATE TABLE IF NOT EXISTS device_allowances (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    routers integer NOT NULL DEFAULT 0,
    controllers integer NOT NULL DEFAULT 0,
    sites integer NOT NULL DEFAULT 0,
    included_router_id uuid REFERENCES router_connections(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Device requests -------------------------------------------------------
CREATE TABLE IF NOT EXISTS device_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(user_id) ON DELETE SET NULL,
    kind text NOT NULL,
    reason text,
    status text NOT NULL DEFAULT 'pending',
    decided_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Notification preferences ----------------------------------------------
CREATE TABLE IF NOT EXISTS notification_prefs (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    in_app boolean NOT NULL DEFAULT true,
    email boolean NOT NULL DEFAULT false,
    quiet_hours_start smallint CHECK (quiet_hours_start BETWEEN 0 AND 23),
    quiet_hours_end smallint CHECK (quiet_hours_end BETWEEN 0 AND 23),
    min_severity text NOT NULL DEFAULT 'warning' CHECK (min_severity IN ('info','warning','critical')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Platform announcement (singleton, platform-wide) ----------------------
CREATE TABLE IF NOT EXISTS platform_announcement (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    active boolean NOT NULL DEFAULT false,
    message text,
    speed_seconds integer CHECK (speed_seconds BETWEEN 8 AND 120),
    updated_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Platform admins -------------------------------------------------------
-- The old model kept a separate platform-admin grant list; the new model uses
-- the DEVELOPER role. Ported so the authorisation RECORD survives, but this
-- table grants nothing by itself — no role is changed by the migration.
CREATE TABLE IF NOT EXISTS platform_admins (
    user_id uuid PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    note text,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Platform service bank accounts (platform-wide) ------------------------
CREATE TABLE IF NOT EXISTS platform_service_bank_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slot integer NOT NULL CHECK (slot IN (1, 2)),
    holder_name text NOT NULL,
    bank_name text NOT NULL,
    account_number text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    sort integer NOT NULL DEFAULT 0,
    configured_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (slot)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON i18n_overrides TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON owner_daily_closes TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON device_allowances TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON device_requests TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON notification_prefs TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_announcement TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_admins TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_service_bank_accounts TO authenticated;

ALTER TABLE owner_daily_closes ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_allowances ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_prefs ENABLE ROW LEVEL SECURITY;

CREATE POLICY owner_daily_closes_tenant_isolation ON owner_daily_closes
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY device_allowances_tenant_isolation ON device_allowances
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY device_requests_tenant_isolation ON device_requests
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY notification_prefs_tenant_isolation ON notification_prefs
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

-- ---- Syslog events ---------------------------------------------------------
-- Operational log history. The old table carried a `token_id`, but it is NULL in
-- every one of the 1,177 rows, so the column is dropped rather than carried.
CREATE TABLE IF NOT EXISTS syslog_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    router_connection_id uuid REFERENCES router_connections(id) ON DELETE SET NULL,
    site_id uuid REFERENCES sites(site_id) ON DELETE SET NULL,
    source_ip text,
    facility text,
    severity text NOT NULL CHECK (severity IN ('info','warning','critical')),
    program text,
    message text NOT NULL,
    ai_summary text,
    ai_severity text CHECK (ai_severity IS NULL OR ai_severity IN ('info','warning','critical')),
    received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS syslog_events_business_idx ON syslog_events(business_id, received_at DESC);

-- ---- Fleet scan runs -------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_scan_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('ai','security')),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    max_severity text NOT NULL CHECK (max_severity IN ('critical','warning','info','ok')),
    router_count integer NOT NULL DEFAULT 0,
    generated_at timestamptz NOT NULL DEFAULT now(),
    triggered_by uuid REFERENCES users(user_id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS fleet_scan_runs_business_idx ON fleet_scan_runs(business_id, generated_at DESC);

-- ---- AI usage events -------------------------------------------------------
CREATE TABLE IF NOT EXISTS ai_usage_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(user_id) ON DELETE SET NULL,
    feature text NOT NULL,
    model text,
    prompt_tokens integer NOT NULL DEFAULT 0,
    completion_tokens integer NOT NULL DEFAULT 0,
    total_tokens integer NOT NULL DEFAULT 0,
    cached boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_usage_events_business_idx ON ai_usage_events(business_id, created_at DESC);

-- ---- Admin notifications ---------------------------------------------------
-- Scoped to a RECIPIENT USER, not a business, so it is granted but not
-- RLS-scoped: the app's tenant context only carries a business id.
CREATE TABLE IF NOT EXISTS admin_notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_user_id uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    kind text NOT NULL,
    title text NOT NULL,
    body text,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_notifications_recipient_idx ON admin_notifications(recipient_user_id, created_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON syslog_events TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON fleet_scan_runs TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON ai_usage_events TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON admin_notifications TO authenticated;

ALTER TABLE syslog_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_scan_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_usage_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY syslog_events_tenant_isolation ON syslog_events
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY fleet_scan_runs_tenant_isolation ON fleet_scan_runs
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

CREATE POLICY ai_usage_events_tenant_isolation ON ai_usage_events
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);
-- ===========================================================================
-- PHASE 3 — per-device SSH transport (security-posture capture, C1)
-- ===========================================================================
-- WHY: the posture prober must reach EACH device at that device's own address
-- with that device's own credentials. Previously one transport was built from
-- config and reused for every peer, so it could only ever report one router's
-- version (see Prober.ObserveVersion). These columns give every device the
-- facts needed to resolve ITS OWN transport.
--
-- The credential is stored ENCRYPTED (AES-256-GCM via internal/secretbox). The
-- key lives in config/env, never here — a DB dump alone must not yield a router
-- password. `legacy_password_ciphertext` on router_connections is NOT reused:
-- it is from the old REST/WebFig stack and is undecryptable by design.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_port integer NOT NULL DEFAULT 22;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_username text;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_secret_ciphertext text;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_secret_kind text;
-- Required for every configured credential. The multi-tenant transport refuses
-- to connect without a SHA256 pin, preventing router impersonation.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_host_key_fingerprint text;

-- A credential is either COMPLETE or ABSENT. A half-configured device must not
-- look reachable: a username with no secret would otherwise resolve to a
-- transport that fails at dial time and gets silently skipped.
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_ssh_creds_complete;
ALTER TABLE devices ADD CONSTRAINT devices_ssh_creds_complete CHECK (
    (ssh_username IS NULL AND ssh_secret_ciphertext IS NULL AND ssh_secret_kind IS NULL
     AND ssh_host_key_fingerprint IS NULL)
 OR (ssh_username IS NOT NULL AND ssh_secret_ciphertext IS NOT NULL
     AND ssh_secret_kind IN ('password', 'key')
     AND ssh_host_key_fingerprint IS NOT NULL
     AND ssh_host_key_fingerprint ~ '^SHA256:[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$')
);

ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_ssh_port_valid;
ALTER TABLE devices ADD CONSTRAINT devices_ssh_port_valid CHECK (ssh_port BETWEEN 1 AND 65535);

-- Tell the "no address" case apart from "address not set yet": without this
-- index the fleet sweep's per-device credential lookup scans devices.
CREATE INDEX IF NOT EXISTS devices_ssh_target_idx
    ON devices(device_id) WHERE management_ip IS NOT NULL AND ssh_username IS NOT NULL;
