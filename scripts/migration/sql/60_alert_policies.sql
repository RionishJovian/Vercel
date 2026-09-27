-- Phase 1 · Step 1.6b — `_stg_alert_rules` (old alert_rules) -> `alert_policies`.
--
-- Kind vocabulary differs: old `router_cpu_high`/`router_memory_high` ->
-- new `cpu_high`/`memory_high`. Unmappable kinds are EXCLUDED rather than
-- coerced, because alert_policies.kind has a CHECK constraint.
-- 49 of 72 old rules point at orphaned owners and are excluded by the tenant filter.
INSERT INTO alert_policies
    (id, business_id, kind, enabled, severity, threshold, cooldown_minutes,
     notification_channels, created_at, updated_at)
SELECT a.id::uuid,
       a.owner_id::uuid,
       CASE a.kind WHEN 'router_cpu_high'    THEN 'cpu_high'
                   WHEN 'router_memory_high' THEN 'memory_high'
                   ELSE a.kind END,
       COALESCE(a.enabled, true),
       CASE WHEN a.severity IN ('info','warning','critical') THEN a.severity ELSE 'warning' END,
       a.threshold::numeric,
       COALESCE(a.cooldown_minutes, 30),
       '{}',
       COALESCE(a.created_at::timestamptz, now()),
       COALESCE(a.updated_at::timestamptz, now())
  FROM _stg_alert_rules a
 WHERE a.owner_id::uuid IN (SELECT old_user_id::uuid FROM _mig_accounts)
   AND a.kind IN ('router_cpu_high','router_memory_high','site_offline','wan_degraded',
                  'connector_stale','poe_ap_offline','provisioning_failed',
                  'cpu_high','memory_high','handshake_stale');
