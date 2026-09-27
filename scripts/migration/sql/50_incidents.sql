-- Phase 1 · Step 1.6a — ops: `_stg_incidents` (old incidents) -> `incidents`.
--
-- Old anchors incidents to an owner (+ optional device); we preserved both uuids,
-- so business_id/device_id copy straight across.
-- `acknowledged_by` is an old auth-user id == the preserved users.user_id, so it
-- resolves; it is nulled if it doesn't (defensive).
-- device_id is nulled when it points at an excluded device, or the FK would fail.
-- Old incidents have no created_at/updated_at -> derived from opened_at.
INSERT INTO incidents
    (id, business_id, device_id, policy_id, kind, severity, subject_id, subject_label,
     detail, opened_at, acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at)
SELECT i.id::uuid,
       i.owner_id::uuid,
       CASE WHEN i.device_id <> '' AND EXISTS (SELECT 1 FROM devices d WHERE d.device_id = i.device_id::uuid)
            THEN i.device_id::uuid ELSE NULL END,
       NULL,
       i.kind,
       CASE WHEN i.severity IN ('info','warning','critical') THEN i.severity ELSE 'warning' END,
       i.subject_id,
       i.subject_label,
       i.detail,
       COALESCE(NULLIF(i.opened_at, '')::timestamptz, now()),
       NULLIF(i.acknowledged_at, '')::timestamptz,
       CASE WHEN i.acknowledged_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = i.acknowledged_by::uuid)
            THEN i.acknowledged_by::uuid ELSE NULL END,
       NULLIF(i.resolved_at, '')::timestamptz,
       COALESCE(NULLIF(i.opened_at, '')::timestamptz, now()),
       COALESCE(NULLIF(i.opened_at, '')::timestamptz, now())
  FROM _stg_incidents i
 WHERE i.owner_id::uuid IN (SELECT old_user_id::uuid FROM _mig_accounts);
