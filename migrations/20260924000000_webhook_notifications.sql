-- Webhook notifications: tenant-configured endpoints + delivery audit log
-- The policy evaluator fires incident events; this fanout carries them out.

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