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

COMMIT;

