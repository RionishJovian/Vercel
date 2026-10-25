-- Phase 2 · Bucket B — account entitlements + feature grants + role defaults.
--
-- `role` values are translated from the old app_role vocabulary onto the new one
-- (client -> OWNER, agent -> ADMIN, site_manager -> ADMIN); the original is
-- kept in `legacy_role` so nothing is lost in the collapse. OPERATOR was
-- consolidated into ADMIN, so the schema's role CHECK constraints only accept
-- OWNER/ADMIN/VIEWER/DEVELOPER.
-- Rows whose user/business did not migrate are excluded (FK integrity).

-- 1) Account entitlements (one row per user)
INSERT INTO account_entitlements
    (user_id, business_id, tier, tier_expires_at, plus, plus_since, created_at, updated_at)
SELECT e.user_id::uuid, u.business_id,
       CASE WHEN e.tier IN ('spark','trial','monthly','annual') THEN e.tier ELSE 'trial' END,
       NULLIF(e.tier_expires_at, '')::timestamptz,
       COALESCE(NULLIF(e.plus, '')::boolean, false),
       NULLIF(e.plus_since, '')::timestamptz,
       COALESCE(NULLIF(e.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(e.updated_at, '')::timestamptz, now())
  FROM _stg_entitlements e
  JOIN users u ON u.user_id = e.user_id::uuid
 WHERE e.owner_id = '' OR e.owner_id::uuid = u.business_id
ON CONFLICT (user_id) DO NOTHING;

-- 2) Operator feature grants (per user)
INSERT INTO operator_feature_grants (user_id, business_id, feature, granted_by, created_at)
SELECT g.user_id::uuid, u.business_id, g.feature,
       CASE WHEN g.granted_by <> '' AND EXISTS (SELECT 1 FROM users x WHERE x.user_id = g.granted_by::uuid)
            THEN g.granted_by::uuid ELSE NULL END,
       COALESCE(NULLIF(g.created_at, '')::timestamptz, now())
  FROM _stg_feature_grants g
  JOIN users u ON u.user_id = g.user_id::uuid
 WHERE g.feature IN ('vouchers','portal_deploy','cash_sales','bank_edit','router_config',
                     'reboot','alerts','syslog_ai','poe','telegram')
ON CONFLICT (user_id, feature) DO NOTHING;

-- 3) Role defaults (platform-wide, keyed by role+feature)
INSERT INTO operator_feature_role_defaults (role, feature, legacy_role, updated_by, updated_at)
SELECT CASE d.legacy_role WHEN 'client' THEN 'OWNER'
                          WHEN 'agent'  THEN 'ADMIN'
                          WHEN 'site_manager' THEN 'ADMIN'
                          ELSE 'ADMIN' END,
       d.feature, d.legacy_role,
       CASE WHEN d.updated_by <> '' AND EXISTS (SELECT 1 FROM users x WHERE x.user_id = d.updated_by::uuid)
            THEN d.updated_by::uuid ELSE NULL END,
       COALESCE(NULLIF(d.updated_at, '')::timestamptz, now())
  FROM _stg_role_defaults d
 WHERE d.feature IN ('vouchers','portal_deploy','cash_sales','bank_edit','router_config',
                     'reboot','alerts','syslog_ai','poe','telegram')
   AND d.legacy_role IN ('client','agent','site_manager')
ON CONFLICT (legacy_role, feature) DO NOTHING;

-- 4) Portal mode role defaults
INSERT INTO portal_mode_role_defaults (role, mode, legacy_role, updated_by, updated_at)
SELECT CASE m.legacy_role WHEN 'client' THEN 'OWNER'
                          WHEN 'agent'  THEN 'ADMIN'
                          WHEN 'site_manager' THEN 'ADMIN'
                          ELSE 'ADMIN' END,
       m.mode, m.legacy_role,
       CASE WHEN m.updated_by <> '' AND EXISTS (SELECT 1 FROM users x WHERE x.user_id = m.updated_by::uuid)
            THEN m.updated_by::uuid ELSE NULL END,
       COALESCE(NULLIF(m.updated_at, '')::timestamptz, now())
  FROM _stg_portal_mode m
 WHERE m.mode IN ('hybrid_light','commerce')
   AND m.legacy_role IN ('client','agent','site_manager')
ON CONFLICT (legacy_role, mode) DO NOTHING;
