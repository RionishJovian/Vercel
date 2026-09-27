-- Phase 2 · Bucket B — sites, router connections, topology config (+ device links).
--
-- Order matters: sites -> router_connections (FKs to sites) -> topology config
-- (FKs to both) -> devices back-links (FKs to both).
-- `owner_id` IS the preserved business_id throughout.

-- 1) Sites (preserve the old site_id so topology + device links resolve)
INSERT INTO sites (site_id, business_id, name, location, timezone, latitude, longitude, notes, created_at, updated_at)
SELECT s.id::uuid, s.owner_id::uuid, COALESCE(NULLIF(s.name, ''), 'Unnamed site'),
       NULLIF(s.location, ''), NULLIF(s.timezone, ''),
       NULLIF(s.latitude, '')::numeric, NULLIF(s.longitude, '')::numeric,
       NULLIF(s.notes, ''),
       COALESCE(NULLIF(s.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(s.updated_at, '')::timestamptz, now())
  FROM _stg_sites s
 WHERE s.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (site_id) DO NOTHING;

-- 2) Router connections. The old ciphertext is carried verbatim and explicitly
--    labelled legacy — it cannot be decrypted by this stack (see init.sql note).
INSERT INTO router_connections
    (id, business_id, name, host, port, use_tls, is_default, site_id,
     connection_mode, cloud_status, cloud_last_seen_at, environment,
     legacy_password_ciphertext, created_at, updated_at)
SELECT r.id::uuid, r.owner_id::uuid, COALESCE(NULLIF(r.name, ''), 'unnamed-router'),
       NULLIF(r.host, ''), NULLIF(r.port, '')::int,
       COALESCE(NULLIF(r.use_tls, '')::boolean, true),
       COALESCE(NULLIF(r.is_default, '')::boolean, false),
       CASE WHEN r.site_id <> '' AND EXISTS (SELECT 1 FROM sites x WHERE x.site_id = r.site_id::uuid)
            THEN r.site_id::uuid ELSE NULL END,
       NULLIF(r.connection_mode, ''), NULLIF(r.cloud_status, ''),
       NULLIF(r.cloud_last_seen_at, '')::timestamptz, NULLIF(r.environment, ''),
       NULLIF(r.password_ciphertext, ''),
       COALESCE(NULLIF(r.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(r.updated_at, '')::timestamptz, now())
  FROM _stg_router_conn r
 WHERE r.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 3) Topology config (only where both sides actually migrated)
INSERT INTO site_topology_config (site_id, router_id, port_labels, downstream_devices, updated_at)
SELECT t.site_id::uuid, t.router_id::uuid,
       COALESCE(NULLIF(t.port_labels, '')::jsonb, '[]'::jsonb),
       COALESCE(NULLIF(t.downstream_devices, '')::jsonb, '[]'::jsonb),
       COALESCE(NULLIF(t.updated_at, '')::timestamptz, now())
  FROM _stg_topology t
 WHERE EXISTS (SELECT 1 FROM sites s WHERE s.site_id = t.site_id::uuid)
   AND EXISTS (SELECT 1 FROM router_connections r WHERE r.id = t.router_id::uuid)
ON CONFLICT (site_id, router_id) DO NOTHING;

-- 4) Devices recover their old site + connection links
UPDATE devices d
   SET site_id = CASE WHEN l.site_id <> ''
                       AND EXISTS (SELECT 1 FROM sites s WHERE s.site_id = l.site_id::uuid)
                      THEN l.site_id::uuid ELSE NULL END,
       router_connection_id = CASE WHEN l.router_id <> ''
                       AND EXISTS (SELECT 1 FROM router_connections r WHERE r.id = l.router_id::uuid)
                      THEN l.router_id::uuid ELSE NULL END
  FROM _stg_device_links l
 WHERE d.device_id = l.device_id::uuid;
