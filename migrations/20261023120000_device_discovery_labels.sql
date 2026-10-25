CREATE TABLE device_discovery_labels (
    business_id UUID NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    mac_address MACADDR NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('other', 'switch', 'ap', 'computer', 'printer', 'controller')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (business_id, device_id, mac_address)
);

CREATE INDEX idx_device_discovery_labels_device ON device_discovery_labels (device_id);
CREATE INDEX idx_device_discovery_labels_mac ON device_discovery_labels (mac_address);

-- RLS: tenant isolation
ALTER TABLE device_discovery_labels ENABLE ROW LEVEL SECURITY;

CREATE POLICY device_discovery_labels_tenant_isolation ON device_discovery_labels
    USING (business_id = current_setting('app.current_business_id')::UUID)
    WITH CHECK (business_id = current_setting('app.current_business_id')::UUID);