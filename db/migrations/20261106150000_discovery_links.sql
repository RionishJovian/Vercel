-- Remembers which registered router a discovered device became, so a later discovery can say "already registered".
-- The key is a one-way hash of the device's advertised software id (never the id itself) or, failing that, a MAC.
-- It is a hint for the owner, not proof of identity. Deleting the router record frees the key.
CREATE TABLE discovery_links (
 business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
 device_key text NOT NULL CHECK (device_key ~ '^(sw:[0-9a-f]{16}|mac:[0-9A-F]{2}(:[0-9A-F]{2}){5})$'),
 device_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (business_id, device_key),
 FOREIGN KEY (device_id, business_id) REFERENCES devices(device_id, business_id) ON DELETE CASCADE
);
CREATE INDEX discovery_links_device ON discovery_links(device_id);
ALTER TABLE discovery_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE discovery_links FORCE ROW LEVEL SECURITY;
CREATE POLICY discovery_links_tenant ON discovery_links
 USING (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON discovery_links TO authenticated;
