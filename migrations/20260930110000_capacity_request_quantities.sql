BEGIN;

ALTER TABLE device_requests
  ADD COLUMN IF NOT EXISTS quantity integer NOT NULL DEFAULT 1;

ALTER TABLE device_requests DROP CONSTRAINT IF EXISTS device_requests_quantity_check;
ALTER TABLE device_requests
  ADD CONSTRAINT device_requests_quantity_check CHECK (quantity BETWEEN 1 AND 100);

ALTER TABLE device_allowances ALTER COLUMN routers SET DEFAULT 1;
ALTER TABLE device_allowances DROP CONSTRAINT IF EXISTS device_allowances_routers_check;
ALTER TABLE device_allowances DROP CONSTRAINT IF EXISTS device_allowances_controllers_check;
ALTER TABLE device_allowances DROP CONSTRAINT IF EXISTS device_allowances_sites_check;
ALTER TABLE device_allowances ADD CONSTRAINT device_allowances_routers_check CHECK (routers >= 0);
ALTER TABLE device_allowances ADD CONSTRAINT device_allowances_controllers_check CHECK (controllers >= 0);
ALTER TABLE device_allowances ADD CONSTRAINT device_allowances_sites_check CHECK (sites >= 0);

-- Every tenant receives the included first-router slot. Existing usage is
-- grandfathered so migration cannot turn a working tenant into an over-limit
-- tenant merely because the legacy allowance row was missing or zeroed.
INSERT INTO device_allowances (business_id, routers, controllers, sites)
SELECT b.business_id,
       GREATEST(1, (SELECT count(*) FROM devices d WHERE d.business_id=b.business_id AND d.deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH'))),
       (SELECT count(*) FROM devices d WHERE d.business_id=b.business_id AND d.deployment_layer='CONTROLLER'),
       (SELECT count(*) FROM sites s WHERE s.business_id=b.business_id)
  FROM businesses b
ON CONFLICT (business_id) DO UPDATE SET
  routers = GREATEST(device_allowances.routers, EXCLUDED.routers),
  controllers = GREATEST(device_allowances.controllers, EXCLUDED.controllers),
  sites = GREATEST(device_allowances.sites, EXCLUDED.sites),
  updated_at = now();

COMMIT;
