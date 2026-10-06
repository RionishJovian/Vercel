BEGIN;

CREATE TABLE alembic_version (
    version_num VARCHAR(32) NOT NULL, 
    CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num)
);

-- Running upgrade  -> 001

CREATE TABLE login_attempts (
    key VARCHAR(64) NOT NULL, 
    failures INTEGER NOT NULL, 
    window_start BIGINT NOT NULL, 
    PRIMARY KEY (key)
);

CREATE TABLE tenants (
    id VARCHAR(36) NOT NULL, 
    name VARCHAR(160) NOT NULL, 
    PRIMARY KEY (id)
);

CREATE TABLE packages (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    policy JSON NOT NULL, 
    active BOOLEAN NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id)
);

CREATE INDEX ix_packages_tenant_id ON packages (tenant_id);

CREATE TABLE users (
    id VARCHAR(36) NOT NULL, 
    username VARCHAR(160) NOT NULL, 
    password_hash VARCHAR(255) NOT NULL, 
    role VARCHAR(20) NOT NULL, 
    tenant_id VARCHAR(36), 
    active BOOLEAN NOT NULL, 
    PRIMARY KEY (id), 
    CHECK (role = 'platform' OR tenant_id IS NOT NULL), 
    CHECK (role in ('platform','sme','guest')), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    UNIQUE (username)
);

CREATE TABLE audit_events (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36), 
    actor_id VARCHAR(36) NOT NULL, 
    action VARCHAR(100) NOT NULL, 
    target VARCHAR(160) NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(actor_id) REFERENCES users (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id)
);

CREATE INDEX ix_audit_events_tenant_id ON audit_events (tenant_id);

CREATE TABLE orders (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    guest_id VARCHAR(36) NOT NULL, 
    package_id VARCHAR(36) NOT NULL, 
    snapshot JSON NOT NULL, 
    amount BIGINT NOT NULL, 
    currency VARCHAR(3) NOT NULL, 
    method VARCHAR(80) NOT NULL, 
    reference VARCHAR(160) NOT NULL, 
    evidence VARCHAR(4000) NOT NULL, 
    status VARCHAR(24) NOT NULL, 
    fulfillment VARCHAR(24) NOT NULL, 
    reviewer_id VARCHAR(36), 
    reason VARCHAR(1000), 
    created BIGINT NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    PRIMARY KEY (id), 
    CHECK (amount > 0), 
    FOREIGN KEY(guest_id) REFERENCES users (id), 
    FOREIGN KEY(package_id) REFERENCES packages (id), 
    FOREIGN KEY(reviewer_id) REFERENCES users (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    UNIQUE (guest_id, request_key)
);

CREATE INDEX ix_orders_tenant_id ON orders (tenant_id);

CREATE TABLE sessions (
    token_hash VARCHAR(64) NOT NULL, 
    user_id VARCHAR(36) NOT NULL, 
    expires BIGINT NOT NULL, 
    PRIMARY KEY (token_hash), 
    FOREIGN KEY(user_id) REFERENCES users (id)
);

CREATE TABLE entitlements (
    id VARCHAR(36) NOT NULL, 
    order_id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    guest_id VARCHAR(36) NOT NULL, 
    policy JSON NOT NULL, 
    activated BIGINT, 
    used_bytes BIGINT NOT NULL, 
    connected_seconds BIGINT NOT NULL, 
    revoked BOOLEAN NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(guest_id) REFERENCES users (id), 
    FOREIGN KEY(order_id) REFERENCES orders (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    UNIQUE (order_id)
);

CREATE INDEX ix_entitlements_tenant_id ON entitlements (tenant_id);

CREATE TABLE outbox_events (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    order_id VARCHAR(36) NOT NULL, 
    event VARCHAR(50) NOT NULL, 
    delivered BOOLEAN NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(order_id) REFERENCES orders (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    UNIQUE (order_id, event)
);

CREATE INDEX ix_outbox_events_tenant_id ON outbox_events (tenant_id);

INSERT INTO alembic_version (version_num) VALUES ('001') RETURNING alembic_version.version_num;

-- Running upgrade 001 -> 002

CREATE TABLE portal_settings (
    tenant_id VARCHAR(36) NOT NULL, 
    revision INTEGER NOT NULL, 
    draft JSON NOT NULL, 
    published JSON, 
    published_revision INTEGER, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (tenant_id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id)
);

ALTER TABLE orders ADD COLUMN payment_context JSON;

UPDATE alembic_version SET version_num='002' WHERE alembic_version.version_num = '001';

-- Running upgrade 002 -> 003

CREATE TABLE vouchers (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    package_id VARCHAR(36) NOT NULL, 
    policy JSON NOT NULL, 
    code_hash VARCHAR(64) NOT NULL, 
    code_hint VARCHAR(4) NOT NULL, 
    issued_by VARCHAR(36) NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    created BIGINT NOT NULL, 
    expires BIGINT NOT NULL, 
    redeemed_by VARCHAR(36), 
    redeemed_at BIGINT, 
    revoked BOOLEAN NOT NULL, 
    reason VARCHAR(1000), 
    PRIMARY KEY (id), 
    UNIQUE (issued_by, request_key), 
    CHECK (expires > created), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(package_id) REFERENCES packages (id), 
    UNIQUE (code_hash), 
    FOREIGN KEY(issued_by) REFERENCES users (id), 
    FOREIGN KEY(redeemed_by) REFERENCES users (id)
);

CREATE INDEX ix_vouchers_tenant_id ON vouchers (tenant_id);

ALTER TABLE entitlements ALTER COLUMN order_id DROP NOT NULL;

ALTER TABLE entitlements ADD COLUMN voucher_id VARCHAR(36);

ALTER TABLE entitlements ADD CONSTRAINT fk_entitlement_voucher FOREIGN KEY(voucher_id) REFERENCES vouchers (id);

ALTER TABLE entitlements ADD CONSTRAINT uq_entitlement_voucher UNIQUE (voucher_id);

ALTER TABLE entitlements ADD CONSTRAINT ck_entitlement_source CHECK ((order_id IS NOT NULL AND voucher_id IS NULL) OR (order_id IS NULL AND voucher_id IS NOT NULL));

UPDATE alembic_version SET version_num='003' WHERE alembic_version.version_num = '002';

-- Running upgrade 003 -> 004

CREATE TABLE voucher_sales (
    id VARCHAR(36) NOT NULL, 
    voucher_id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    snapshot JSON NOT NULL, 
    amount BIGINT NOT NULL, 
    currency VARCHAR(3) NOT NULL, 
    method VARCHAR(80) NOT NULL, 
    reference VARCHAR(160) NOT NULL, 
    evidence VARCHAR(4000) NOT NULL, 
    recorded_by VARCHAR(36) NOT NULL, 
    created BIGINT NOT NULL, 
    refunded_at BIGINT, 
    refunded_by VARCHAR(36), 
    refund_reason VARCHAR(1000), 
    refund_reference VARCHAR(160), 
    PRIMARY KEY (id), 
    CHECK (amount > 0), 
    UNIQUE (voucher_id), 
    FOREIGN KEY(voucher_id) REFERENCES vouchers (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(recorded_by) REFERENCES users (id), 
    FOREIGN KEY(refunded_by) REFERENCES users (id)
);

CREATE INDEX ix_voucher_sales_tenant_id ON voucher_sales (tenant_id);

UPDATE alembic_version SET version_num='004' WHERE alembic_version.version_num = '003';

-- Running upgrade 004 -> 005

CREATE TABLE sites (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    name VARCHAR(120) NOT NULL, 
    timezone VARCHAR(80) NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (tenant_id, name), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id)
);

CREATE INDEX ix_sites_tenant_id ON sites (tenant_id);

CREATE TABLE router_devices (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    site_id VARCHAR(36) NOT NULL, 
    name VARCHAR(120) NOT NULL, 
    model VARCHAR(120) NOT NULL, 
    reported_routeros VARCHAR(40) NOT NULL, 
    registered_by VARCHAR(36) NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (site_id, name), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(site_id) REFERENCES sites (id), 
    FOREIGN KEY(registered_by) REFERENCES users (id)
);

CREATE INDEX ix_router_devices_tenant_id ON router_devices (tenant_id);

CREATE TABLE site_portals (
    site_id VARCHAR(36) NOT NULL, 
    revision INTEGER NOT NULL, 
    draft JSON NOT NULL, 
    published JSON, 
    published_revision INTEGER, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (site_id), 
    FOREIGN KEY(site_id) REFERENCES sites (id)
);

ALTER TABLE orders ADD COLUMN site_id VARCHAR(36);

ALTER TABLE orders ADD CONSTRAINT fk_orders_site FOREIGN KEY(site_id) REFERENCES sites (id);

ALTER TABLE vouchers ADD COLUMN site_id VARCHAR(36);

ALTER TABLE vouchers ADD CONSTRAINT fk_vouchers_site FOREIGN KEY(site_id) REFERENCES sites (id);

ALTER TABLE entitlements ADD COLUMN site_id VARCHAR(36);

ALTER TABLE entitlements ADD CONSTRAINT fk_entitlements_site FOREIGN KEY(site_id) REFERENCES sites (id);

UPDATE alembic_version SET version_num='005' WHERE alembic_version.version_num = '004';

-- Running upgrade 005 -> 006

CREATE TABLE magic_hubs (
    id VARCHAR(36) NOT NULL, 
    name VARCHAR(120) NOT NULL, 
    endpoint VARCHAR(253) NOT NULL, 
    port INTEGER NOT NULL, 
    pool VARCHAR(18) NOT NULL, 
    public_key VARCHAR(44) NOT NULL, 
    next_host INTEGER NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (endpoint, port), 
    CHECK (port >= 1 AND port <= 65535), 
    CHECK (next_host >= 2 AND next_host <= 255), 
    UNIQUE (name), 
    UNIQUE (pool), 
    UNIQUE (public_key)
);

CREATE TABLE hub_assignments (
    hub_id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (hub_id, tenant_id), 
    FOREIGN KEY(hub_id) REFERENCES magic_hubs (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id)
);

CREATE TABLE router_plans (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    router_id VARCHAR(36) NOT NULL, 
    hub_id VARCHAR(36) NOT NULL, 
    router_public_key VARCHAR(44) NOT NULL, 
    address VARCHAR(15) NOT NULL, 
    snapshot JSON NOT NULL, 
    created BIGINT NOT NULL, 
    reviewed_at BIGINT, 
    PRIMARY KEY (id), 
    UNIQUE (hub_id, address), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    UNIQUE (router_id), 
    FOREIGN KEY(router_id) REFERENCES router_devices (id), 
    FOREIGN KEY(hub_id) REFERENCES magic_hubs (id), 
    UNIQUE (router_public_key)
);

CREATE INDEX ix_router_plans_tenant_id ON router_plans (tenant_id);

UPDATE alembic_version SET version_num='006' WHERE alembic_version.version_num = '005';

-- Running upgrade 006 -> 007

CREATE TABLE financial_events (
    id VARCHAR(80) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    site_id VARCHAR(36), 
    source VARCHAR(20) NOT NULL, 
    source_id VARCHAR(36) NOT NULL, 
    kind VARCHAR(16) NOT NULL, 
    amount BIGINT NOT NULL, 
    currency VARCHAR(3) NOT NULL, 
    service VARCHAR(20) NOT NULL, 
    package_name VARCHAR(120) NOT NULL, 
    method VARCHAR(80) NOT NULL, 
    reference VARCHAR(160) NOT NULL, 
    actor_id VARCHAR(36), 
    occurred_at BIGINT NOT NULL, 
    timestamp_basis VARCHAR(32) NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (source, source_id, kind), 
    CHECK (amount > 0), 
    CHECK (kind IN ('collection','refund')), 
    CHECK (source IN ('guest_order','voucher_sale')), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(site_id) REFERENCES sites (id), 
    FOREIGN KEY(actor_id) REFERENCES users (id)
);

CREATE INDEX ix_financial_events_tenant_id ON financial_events (tenant_id);

CREATE INDEX ix_financial_events_occurred_at ON financial_events (occurred_at);

INSERT INTO financial_events (id, tenant_id, site_id, source, source_id, kind, amount, currency, service, package_name, method, reference, actor_id, occurred_at, timestamp_basis) SELECT 'guest_order:collection:' || orders.id AS anon_1, orders.tenant_id, orders.site_id, 'guest_order' AS anon_2, orders.id, 'collection' AS anon_3, orders.amount, orders.currency, coalesce(CAST(orders.snapshot ->> 'service' AS VARCHAR), 'unknown') AS coalesce_1, coalesce(CAST(orders.snapshot ->> 'name' AS VARCHAR), 'Legacy package') AS coalesce_2, orders.method, orders.reference, coalesce((SELECT audit_events.actor_id 
FROM audit_events 
WHERE audit_events.tenant_id = orders.tenant_id AND audit_events.target = orders.id AND audit_events.action = 'payment_approved' ORDER BY audit_events.created, audit_events.id 
 LIMIT 1), orders.reviewer_id) AS coalesce_3, coalesce((SELECT min(audit_events.created) AS min_1 
FROM audit_events 
WHERE audit_events.tenant_id = orders.tenant_id AND audit_events.target = orders.id AND audit_events.action = 'payment_approved'), orders.created) AS coalesce_4, CASE WHEN ((SELECT min(audit_events.created) AS min_1 
FROM audit_events 
WHERE audit_events.tenant_id = orders.tenant_id AND audit_events.target = orders.id AND audit_events.action = 'payment_approved') IS NOT NULL) THEN 'migrated_audit' ELSE 'legacy_submission_estimate' END AS anon_4 
FROM orders 
WHERE orders.status IN ('approved', 'refunded');

INSERT INTO financial_events (id, tenant_id, site_id, source, source_id, kind, amount, currency, service, package_name, method, reference, actor_id, occurred_at, timestamp_basis) SELECT 'guest_order:refund:' || orders.id AS anon_1, orders.tenant_id, orders.site_id, 'guest_order' AS anon_2, orders.id, 'refund' AS anon_3, orders.amount, orders.currency, coalesce(CAST(orders.snapshot ->> 'service' AS VARCHAR), 'unknown') AS coalesce_1, coalesce(CAST(orders.snapshot ->> 'name' AS VARCHAR), 'Legacy package') AS coalesce_2, orders.method, orders.reference, (SELECT audit_events.actor_id 
FROM audit_events 
WHERE audit_events.tenant_id = orders.tenant_id AND audit_events.target = orders.id AND audit_events.action = 'refund_recorded' ORDER BY audit_events.created, audit_events.id 
 LIMIT 1) AS anon_4, coalesce((SELECT min(audit_events.created) AS min_1 
FROM audit_events 
WHERE audit_events.tenant_id = orders.tenant_id AND audit_events.target = orders.id AND audit_events.action = 'refund_recorded'), orders.created) AS coalesce_3, CASE WHEN ((SELECT min(audit_events.created) AS min_1 
FROM audit_events 
WHERE audit_events.tenant_id = orders.tenant_id AND audit_events.target = orders.id AND audit_events.action = 'refund_recorded') IS NOT NULL) THEN 'migrated_audit' ELSE 'legacy_submission_estimate' END AS anon_5 
FROM orders 
WHERE orders.status IN ('refunded');

INSERT INTO financial_events (id, tenant_id, site_id, source, source_id, kind, amount, currency, service, package_name, method, reference, actor_id, occurred_at, timestamp_basis) SELECT 'voucher_sale:collection:' || voucher_sales.id AS anon_1, voucher_sales.tenant_id, vouchers.site_id, 'voucher_sale' AS anon_2, voucher_sales.id, 'collection' AS anon_3, voucher_sales.amount, voucher_sales.currency, coalesce(CAST(voucher_sales.snapshot ->> 'service' AS VARCHAR), 'unknown') AS coalesce_1, coalesce(CAST(voucher_sales.snapshot ->> 'name' AS VARCHAR), 'Legacy package') AS coalesce_2, voucher_sales.method, voucher_sales.reference, voucher_sales.recorded_by, voucher_sales.created, 'migrated_record' AS anon_4 
FROM voucher_sales JOIN vouchers ON vouchers.id = voucher_sales.voucher_id AND vouchers.tenant_id = voucher_sales.tenant_id;

INSERT INTO financial_events (id, tenant_id, site_id, source, source_id, kind, amount, currency, service, package_name, method, reference, actor_id, occurred_at, timestamp_basis) SELECT 'voucher_sale:refund:' || voucher_sales.id AS anon_1, voucher_sales.tenant_id, vouchers.site_id, 'voucher_sale' AS anon_2, voucher_sales.id, 'refund' AS anon_3, voucher_sales.amount, voucher_sales.currency, coalesce(CAST(voucher_sales.snapshot ->> 'service' AS VARCHAR), 'unknown') AS coalesce_1, coalesce(CAST(voucher_sales.snapshot ->> 'name' AS VARCHAR), 'Legacy package') AS coalesce_2, voucher_sales.method, coalesce(voucher_sales.refund_reference, voucher_sales.reference) AS coalesce_3, voucher_sales.refunded_by, voucher_sales.refunded_at, 'migrated_record' AS anon_4 
FROM voucher_sales JOIN vouchers ON vouchers.id = voucher_sales.voucher_id AND vouchers.tenant_id = voucher_sales.tenant_id 
WHERE voucher_sales.refunded_at IS NOT NULL;

UPDATE alembic_version SET version_num='007' WHERE alembic_version.version_num = '006';

-- Running upgrade 007 -> 008

CREATE TABLE theme_settings (
    id VARCHAR(32) NOT NULL, 
    revision INTEGER NOT NULL, 
    draft JSON NOT NULL, 
    published JSON, 
    previous JSON, 
    published_revision INTEGER, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (id)
);

UPDATE alembic_version SET version_num='008' WHERE alembic_version.version_num = '007';

-- Running upgrade 008 -> 009

CREATE TABLE media_titles (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    name VARCHAR(120) NOT NULL, 
    description VARCHAR(4000) NOT NULL, 
    genre VARCHAR(80) NOT NULL, 
    runtime_minutes INTEGER NOT NULL, 
    release_year INTEGER, 
    published BOOLEAN NOT NULL, 
    archived BOOLEAN NOT NULL, 
    revision INTEGER NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    CHECK (runtime_minutes > 0), 
    CHECK (revision > 0), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id)
);

CREATE INDEX ix_media_titles_tenant_id ON media_titles (tenant_id);

UPDATE alembic_version SET version_num='009' WHERE alembic_version.version_num = '008';

-- Running upgrade 009 -> 010

CREATE TABLE knowledge_cards (
    id VARCHAR(36) NOT NULL, 
    content JSON NOT NULL, 
    published BOOLEAN NOT NULL, 
    archived BOOLEAN NOT NULL, 
    revision INTEGER NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    CHECK (revision > 0)
);

INSERT INTO knowledge_cards (id, content, published, archived, revision, updated) VALUES ('guide-hotspot-radius', '{"title": "Portal, HotSpot and RADIUS: three jobs", "category": "Internet access", "fact": "The captive portal is the guest webpage. MikroTik HotSpot controls network access. RADIUS checks credentials, supplies access rules and receives usage reports.", "why": "Payment approval creates a stored access entitlement; the network adapter still needs to provision credentials and enforcement before a guest can use it.", "requirements": "HotSpot can use local accounts without RADIUS. Cloud RADIUS or local User Manager are authentication choices; their adapters are not connected in this build.", "action": "Captive portal", "source_url": "https://manual.mikrotik.com/docs/authentication-authorization-accounting/radius/", "verified_on": "2026-10-05"}', true, false, 1, 1791158400);

INSERT INTO knowledge_cards (id, content, published, archived, revision, updated) VALUES ('guide-local-radius', '{"title": "Your router may host a RADIUS server", "category": "MikroTik", "fact": "MikroTik User Manager is a separate RADIUS server package. The documented active-session limits are Level 4: 20, Level 5: 50 and Level 6: unlimited by license.", "why": "At approximately 50 guests, a Level 4 router cannot host all those User Manager sessions. Multiple connections from one guest count as separate sessions.", "requirements": "Check exact model, RouterOS version, architecture, storage and license. User Manager excludes SMIPS. Unlimited licensing does not guarantee hardware capacity. Local integration is proposed, not implemented.", "action": "Routers", "source_url": "https://manual.mikrotik.com/docs/getting-started/routeros-licensing/mikrotik-hardware/", "verified_on": "2026-10-05"}', true, false, 1, 1791158400);

INSERT INTO knowledge_cards (id, content, published, archived, revision, updated) VALUES ('guide-wireguard', '{"title": "WireGuard connects your router to Magic Hub", "category": "MikroTik", "fact": "WireGuard creates an encrypted tunnel and authenticates peers using keys. Guest voucher authentication is a separate HotSpot/RADIUS operation.", "why": "Our proposed configuration uses the tunnel for router management and cloud RADIUS traffic, while guest internet stays on the site WAN or Starlink.", "requirements": "Requires supported RouterOS, configured keys, routes and firewall isolation. A registered router or exported onboarding plan does not prove a live tunnel or connectivity.", "action": "Routers", "source_url": "https://manual.mikrotik.com/docs/virtual-private-networks/wireguard/", "verified_on": "2026-10-05"}', true, false, 1, 1791158400);

UPDATE alembic_version SET version_num='010' WHERE alembic_version.version_num = '009';

-- Running upgrade 010 -> 011

ALTER TABLE orders ADD COLUMN receipt_revision INTEGER DEFAULT '0' NOT NULL;

CREATE TABLE receipt_images (
    id VARCHAR(36) NOT NULL, 
    order_id VARCHAR(36) NOT NULL, 
    uploaded_by VARCHAR(36) NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    original_hash VARCHAR(64) NOT NULL, 
    image BYTEA NOT NULL, 
    width INTEGER NOT NULL, 
    height INTEGER NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (order_id, request_key), 
    CHECK (width > 0 AND height > 0), 
    FOREIGN KEY(order_id) REFERENCES orders (id), 
    FOREIGN KEY(uploaded_by) REFERENCES users (id)
);

CREATE INDEX ix_receipt_images_order_id ON receipt_images (order_id);

UPDATE alembic_version SET version_num='011' WHERE alembic_version.version_num = '010';

-- Running upgrade 011 -> 012

CREATE TABLE review_credentials (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    user_id VARCHAR(36) NOT NULL, 
    label VARCHAR(120) NOT NULL, 
    provider VARCHAR(16) NOT NULL, 
    reviewer_identity VARCHAR(160) NOT NULL, 
    can_decide BOOLEAN NOT NULL, 
    token_hash VARCHAR(64) NOT NULL, 
    created BIGINT NOT NULL, 
    expires BIGINT NOT NULL, 
    revoked_at BIGINT, 
    revoke_reason VARCHAR(1000), 
    PRIMARY KEY (id), 
    CHECK (expires > created), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(user_id) REFERENCES users (id), 
    UNIQUE (token_hash)
);

CREATE INDEX ix_review_credentials_tenant_id ON review_credentials (tenant_id);

CREATE TABLE external_review_actions (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    credential_id VARCHAR(36) NOT NULL, 
    order_id VARCHAR(36) NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    request_hash VARCHAR(64) NOT NULL, 
    reviewer_identity VARCHAR(160) NOT NULL, 
    provider VARCHAR(16) NOT NULL, 
    service_label VARCHAR(120) NOT NULL, 
    decision VARCHAR(24) NOT NULL, 
    reason VARCHAR(1000) NOT NULL, 
    result JSON NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (credential_id, request_key), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(credential_id) REFERENCES review_credentials (id), 
    FOREIGN KEY(order_id) REFERENCES orders (id)
);

CREATE INDEX ix_external_review_actions_tenant_id ON external_review_actions (tenant_id);

UPDATE alembic_version SET version_num='012' WHERE alembic_version.version_num = '011';

-- Running upgrade 012 -> 013

ALTER TABLE orders ADD COLUMN event_revision INTEGER DEFAULT '0' NOT NULL;

ALTER TABLE review_credentials ADD COLUMN receive_events BOOLEAN DEFAULT false NOT NULL;

ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_order_id_event_key;

ALTER TABLE outbox_events ADD COLUMN revision INTEGER;

ALTER TABLE outbox_events ADD COLUMN created BIGINT;

ALTER TABLE outbox_events ADD COLUMN payload JSON;

ALTER TABLE outbox_events ADD CONSTRAINT uq_outbox_order_revision UNIQUE (order_id, revision);

CREATE TABLE event_deliveries (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    credential_id VARCHAR(36) NOT NULL, 
    event_id VARCHAR(36) NOT NULL, 
    state VARCHAR(16) NOT NULL, 
    attempts INTEGER NOT NULL, 
    cycle_attempts INTEGER NOT NULL, 
    available_at BIGINT NOT NULL, 
    lease_expires BIGINT, 
    lease_hash VARCHAR(64), 
    completion JSON, 
    acknowledged_at BIGINT, 
    last_error VARCHAR(1000), 
    PRIMARY KEY (id), 
    UNIQUE (credential_id, event_id), 
    CHECK (state IN ('pending','leased','acknowledged','failed')), 
    CHECK (attempts >= 0 AND cycle_attempts >= 0), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(credential_id) REFERENCES review_credentials (id), 
    FOREIGN KEY(event_id) REFERENCES outbox_events (id)
);

CREATE INDEX ix_event_deliveries_tenant_id ON event_deliveries (tenant_id);

CREATE INDEX ix_event_deliveries_credential_id ON event_deliveries (credential_id);

UPDATE alembic_version SET version_num='013' WHERE alembic_version.version_num = '012';

-- Running upgrade 013 -> 014

ALTER TABLE users ADD COLUMN must_change_password BOOLEAN DEFAULT false NOT NULL;

ALTER TABLE users ADD COLUMN temporary_password_expires BIGINT;

CREATE TABLE business_provisioning (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    owner_id VARCHAR(36) NOT NULL, 
    created_by VARCHAR(36) NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    request_hash VARCHAR(64) NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (created_by, request_key), 
    UNIQUE (tenant_id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    UNIQUE (owner_id), 
    FOREIGN KEY(owner_id) REFERENCES users (id), 
    FOREIGN KEY(created_by) REFERENCES users (id)
);

CREATE TABLE password_reset_records (
    id VARCHAR(36) NOT NULL, 
    user_id VARCHAR(36) NOT NULL, 
    actor_id VARCHAR(36) NOT NULL, 
    reason VARCHAR(1000) NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(user_id) REFERENCES users (id), 
    FOREIGN KEY(actor_id) REFERENCES users (id)
);

UPDATE alembic_version SET version_num='014' WHERE alembic_version.version_num = '013';

-- Running upgrade 014 -> 015

CREATE TABLE site_planning (
    site_id VARCHAR(36) NOT NULL, 
    revision INTEGER NOT NULL, 
    settings JSON NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (site_id), 
    FOREIGN KEY(site_id) REFERENCES sites (id)
);

UPDATE alembic_version SET version_num='015' WHERE alembic_version.version_num = '014';

-- Running upgrade 015 -> 016

CREATE TABLE subscription_plans (
    id VARCHAR(36) NOT NULL, 
    settings JSON NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id)
);

CREATE TABLE tenant_subscriptions (
    tenant_id VARCHAR(36) NOT NULL, 
    revision INTEGER NOT NULL, 
    plan_id VARCHAR(36) NOT NULL, 
    plan_snapshot JSON NOT NULL, 
    state VARCHAR(20) NOT NULL, 
    period_start BIGINT NOT NULL, 
    period_end BIGINT NOT NULL, 
    reason VARCHAR(1000) NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (tenant_id), 
    CHECK (state in ('trial','active','past_due','suspended','cancelled')), 
    CHECK (period_end > period_start), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(plan_id) REFERENCES subscription_plans (id)
);

UPDATE alembic_version SET version_num='016' WHERE alembic_version.version_num = '015';

-- Running upgrade 016 -> 017

CREATE TABLE staff_grants (
    user_id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    profile VARCHAR(20) NOT NULL, 
    revision INTEGER NOT NULL, 
    created_by VARCHAR(36) NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    request_hash VARCHAR(64) NOT NULL, 
    created BIGINT NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (user_id), 
    UNIQUE (created_by, request_key), 
    CHECK (profile in ('manager','cashier','technician')), 
    FOREIGN KEY(user_id) REFERENCES users (id), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(created_by) REFERENCES users (id)
);

CREATE INDEX ix_staff_grants_tenant_id ON staff_grants (tenant_id);

ALTER TABLE staff_grants ADD COLUMN last_reason VARCHAR(1000) DEFAULT '' NOT NULL;

UPDATE alembic_version SET version_num='017' WHERE alembic_version.version_num = '016';

-- Running upgrade 017 -> 018

ALTER TABLE tenant_subscriptions ADD COLUMN payment_source_invoice_id VARCHAR(36);

CREATE TABLE subscription_invoices (
    id VARCHAR(36) NOT NULL, 
    tenant_id VARCHAR(36) NOT NULL, 
    plan_id VARCHAR(36) NOT NULL, 
    snapshot JSON NOT NULL, 
    amount BIGINT NOT NULL, 
    currency VARCHAR(3) NOT NULL, 
    period_start BIGINT NOT NULL, 
    period_end BIGINT NOT NULL, 
    subscription_revision INTEGER NOT NULL, 
    revision INTEGER NOT NULL, 
    status VARCHAR(20) NOT NULL, 
    destination VARCHAR(500) NOT NULL, 
    instructions VARCHAR(2000) NOT NULL, 
    method VARCHAR(80), 
    reference VARCHAR(160), 
    evidence VARCHAR(4000), 
    submission_key VARCHAR(80), 
    submission_hash VARCHAR(64), 
    reason VARCHAR(1000), 
    reviewed_by VARCHAR(36), 
    created_by VARCHAR(36) NOT NULL, 
    request_key VARCHAR(80) NOT NULL, 
    request_hash VARCHAR(64) NOT NULL, 
    created BIGINT NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    UNIQUE (created_by, request_key), 
    CHECK (amount > 0), 
    CHECK (period_end > period_start), 
    CHECK (status in ('unpaid','pending','needs_evidence','paid','rejected','void')), 
    FOREIGN KEY(tenant_id) REFERENCES tenants (id), 
    FOREIGN KEY(plan_id) REFERENCES subscription_plans (id), 
    FOREIGN KEY(reviewed_by) REFERENCES users (id), 
    FOREIGN KEY(created_by) REFERENCES users (id)
);

CREATE INDEX ix_subscription_invoices_tenant_id ON subscription_invoices (tenant_id);

CREATE TABLE billing_events (
    id VARCHAR(36) NOT NULL, 
    invoice_id VARCHAR(36) NOT NULL, 
    actor_id VARCHAR(36) NOT NULL, 
    event VARCHAR(40) NOT NULL, 
    payload JSON NOT NULL, 
    created BIGINT NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(invoice_id) REFERENCES subscription_invoices (id), 
    FOREIGN KEY(actor_id) REFERENCES users (id)
);

CREATE INDEX ix_billing_events_invoice_id ON billing_events (invoice_id);

UPDATE alembic_version SET version_num='018' WHERE alembic_version.version_num = '017';

-- Running upgrade 018 -> 019

CREATE TABLE account_mfa (
    user_id VARCHAR(36) NOT NULL, 
    enabled BOOLEAN NOT NULL, 
    encrypted_secret VARCHAR(1000), 
    enrollment_expires BIGINT, 
    last_step BIGINT NOT NULL, 
    recovery_hashes JSON NOT NULL, 
    updated BIGINT NOT NULL, 
    PRIMARY KEY (user_id), 
    FOREIGN KEY(user_id) REFERENCES users (id)
);

UPDATE alembic_version SET version_num='019' WHERE alembic_version.version_num = '018';

COMMIT;

