-- Alerts for the platform owner when a connected router stays down. They name routers
-- and customers across accounts, so row security is on with no tenant policy, and the
-- tenant role gets no grant. mm-migrate's runtime-role step adds the service policy.
CREATE TABLE platform_router_alerts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
  device_id uuid NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('offline', 'recovered')),
  -- The drop this alert is about. One alert of each kind per drop.
  offline_since timestamptz NOT NULL,
  detail text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  read_at timestamptz,
  UNIQUE (device_id, kind, offline_since)
);
CREATE INDEX platform_router_alerts_recent ON platform_router_alerts (created_at DESC);
CREATE INDEX platform_router_alerts_unread ON platform_router_alerts (created_at DESC) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_router_alerts_business_id ON platform_router_alerts (business_id);
ALTER TABLE platform_router_alerts ENABLE ROW LEVEL SECURITY;
