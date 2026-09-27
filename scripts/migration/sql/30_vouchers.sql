-- Phase 1 · Step 1.5a — money: `_stg_vouchers` (old voucher_codes) -> `vouchers`.
--
-- Nearly 1:1 with the new table. Two adjustments:
--   * status: old has `cancelled` (13 rows) which is NOT in the new enum
--     {unused,active,expired,revoked,banned} -> mapped to `revoked`.
--   * device_id: old anchors vouchers to a router/site, not a device, so it stays NULL.
--   * device_mac: old column is TEXT -> cast only when mac-shaped.
INSERT INTO vouchers
    (id, business_id, device_id, plan_key, plan_label, code, price_mmk, duration_minutes,
     device_mac, status, first_seen_at, expires_at, created_at, updated_at)
SELECT v.id::uuid,
       v.owner_id::uuid,
       NULL,
       v.plan_key,
       v.plan_label,
       v.code,
       v.price_mmk,
       v.duration_minutes,
       CASE WHEN v.device_mac ~ '^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$' THEN v.device_mac::macaddr ELSE NULL END,
       CASE v.status WHEN 'cancelled' THEN 'revoked'
                     WHEN 'unused' THEN 'unused'
                     WHEN 'active' THEN 'active'
                     WHEN 'expired' THEN 'expired'
                     WHEN 'revoked' THEN 'revoked'
                     ELSE 'unused' END,
       NULLIF(v.first_seen_at, '')::timestamptz,
       NULLIF(v.expires_at, '')::timestamptz,
       COALESCE(v.created_at::timestamptz, now()),
       COALESCE(v.updated_at::timestamptz, now())
  FROM _stg_vouchers v
 WHERE v.owner_id::uuid IN (SELECT old_user_id::uuid FROM _mig_accounts);
