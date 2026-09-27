-- Phase 1 · Step 1.4 — fleet: `_stg_devices` (old managed_devices export) -> `devices`.
--
-- Old is single-tenant: `owner_id` IS the tenant key, and we preserved it as
-- `business_id`, so this is a direct copy.
-- CHECK-safe: vendor/category/transport are coerced into the new enums so no row
-- can violate devices' CHECK constraints; mac/ip are cast only when well-formed.
-- Rows whose owner is an orphan (no surviving account) are excluded.
INSERT INTO devices
    (device_id, business_id, hardware_name, device_model, deployment_layer,
     vendor, category, transport, mac_address, management_ip, is_online, created_at, updated_at)
SELECT s.id::uuid,
       s.owner_id::uuid,
       COALESCE(NULLIF(s.name, ''), 'unnamed-device'),
       NULLIF(s.model, ''),
       CASE s.category
           WHEN 'switch'     THEN 'DOWNSTREAM_SWITCH'
           WHEN 'ap'         THEN 'ACCESS_POINT'
           WHEN 'controller' THEN 'CONTROLLER'
           ELSE 'MAIN_GATEWAY'                       -- router / gateway / unknown
       END,
       CASE WHEN s.vendor IN ('mikrotik','ruijie','cisco','tplink','ubiquiti','generic')
            THEN s.vendor ELSE 'generic' END,
       CASE WHEN s.category IN ('router','gateway','switch','ap','controller')
            THEN s.category ELSE 'router' END,
       CASE WHEN s.transport IN ('direct','connector','tunnel','controller')
            THEN s.transport ELSE 'direct' END,
       CASE WHEN s.mac  ~ '^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$' THEN s.mac::macaddr  ELSE NULL END,
       CASE WHEN s.host ~ '^[0-9]{1,3}(\.[0-9]{1,3}){3}$'        THEN s.host::inet    ELSE NULL END,
       false,
       COALESCE(s.created_at::timestamptz, now()),
       COALESCE(s.updated_at::timestamptz, now())
  FROM _stg_devices s
 WHERE s.owner_id::uuid IN (SELECT old_user_id::uuid FROM _mig_accounts);
