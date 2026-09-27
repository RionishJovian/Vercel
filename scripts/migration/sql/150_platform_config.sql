-- Phase 2 · Bucket B — i18n, daily closes, device allowances/requests, notification
-- prefs, platform announcement, platform admins, platform bank accounts.

-- 1) i18n overrides (platform-wide). Upsert-by-key so a re-run cannot duplicate.
INSERT INTO i18n_overrides (id, locale, ns, key, value, updated_by, updated_at)
SELECT i.id::uuid, i.locale, i.ns, i.key, i.value,
       CASE WHEN i.updated_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = i.updated_by::uuid)
            THEN i.updated_by::uuid ELSE NULL END,
       COALESCE(NULLIF(i.updated_at, '')::timestamptz, now())
  FROM _stg_i18n i
 WHERE i.locale IN ('en','zh','my') AND i.ns IN ('ui','copy')
ON CONFLICT (locale, ns, key) DO NOTHING;

-- 2) Owner daily closes
INSERT INTO owner_daily_closes
    (id, business_id, site_id, router_connection_id, business_day, counted_cash_mmk, note, closed_by, closed_at)
SELECT c.id::uuid, c.owner_id::uuid,
       CASE WHEN c.site_id <> '' AND EXISTS (SELECT 1 FROM sites s WHERE s.site_id = c.site_id::uuid)
            THEN c.site_id::uuid ELSE NULL END,
       CASE WHEN c.router_id <> '' AND EXISTS (SELECT 1 FROM router_connections r WHERE r.id = c.router_id::uuid)
            THEN c.router_id::uuid ELSE NULL END,
       c.business_day::date,
       COALESCE(NULLIF(c.counted_cash_mmk, '')::int, 0),
       NULLIF(c.note, ''),
       CASE WHEN c.closed_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = c.closed_by::uuid)
            THEN c.closed_by::uuid ELSE NULL END,
       COALESCE(NULLIF(c.closed_at, '')::timestamptz, now())
  FROM _stg_daily_closes c
 WHERE c.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 3) Device allowances
INSERT INTO device_allowances
    (business_id, routers, controllers, sites, included_router_id, created_at, updated_at)
SELECT a.owner_id::uuid,
       COALESCE(NULLIF(a.routers, '')::int, 0),
       COALESCE(NULLIF(a.controllers, '')::int, 0),
       COALESCE(NULLIF(a.sites, '')::int, 0),
       CASE WHEN a.included_router_id <> '' AND EXISTS (SELECT 1 FROM router_connections r WHERE r.id = a.included_router_id::uuid)
            THEN a.included_router_id::uuid ELSE NULL END,
       COALESCE(NULLIF(a.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(a.updated_at, '')::timestamptz, now())
  FROM _stg_allowances a
 WHERE a.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (business_id) DO NOTHING;

-- 4) Device requests
INSERT INTO device_requests
    (id, business_id, user_id, kind, reason, status, decided_by, decided_at, created_at, updated_at)
SELECT r.id::uuid, r.owner_id::uuid,
       CASE WHEN r.user_id <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = r.user_id::uuid)
            THEN r.user_id::uuid ELSE NULL END,
       COALESCE(NULLIF(r.kind, ''), 'routers'),
       NULLIF(r.reason, ''),
       COALESCE(NULLIF(r.status, ''), 'pending'),
       CASE WHEN r.decided_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = r.decided_by::uuid)
            THEN r.decided_by::uuid ELSE NULL END,
       NULLIF(r.decided_at, '')::timestamptz,
       COALESCE(NULLIF(r.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(r.updated_at, '')::timestamptz, now())
  FROM _stg_dev_requests r
 WHERE r.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 5) Notification preferences
INSERT INTO notification_prefs
    (business_id, in_app, email, quiet_hours_start, quiet_hours_end, min_severity, created_at, updated_at)
SELECT p.owner_id::uuid,
       COALESCE(NULLIF(p.in_app, '')::boolean, true),
       COALESCE(NULLIF(p.email, '')::boolean, false),
       CASE WHEN p.quiet_hours_start ~ '^[0-9]+$' AND p.quiet_hours_start::int BETWEEN 0 AND 23
            THEN p.quiet_hours_start::smallint ELSE NULL END,
       CASE WHEN p.quiet_hours_end ~ '^[0-9]+$' AND p.quiet_hours_end::int BETWEEN 0 AND 23
            THEN p.quiet_hours_end::smallint ELSE NULL END,
       CASE WHEN p.min_severity IN ('info','warning','critical') THEN p.min_severity ELSE 'warning' END,
       COALESCE(NULLIF(p.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(p.updated_at, '')::timestamptz, now())
  FROM _stg_notif_prefs p
 WHERE p.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (business_id) DO NOTHING;

-- 6) Platform announcement (singleton)
INSERT INTO platform_announcement (id, active, message, speed_seconds, updated_by, updated_at)
SELECT true,
       COALESCE(NULLIF(a.active, '')::boolean, false),
       NULLIF(a.message, ''),
       CASE WHEN a.speed_seconds ~ '^[0-9]+$' AND a.speed_seconds::int BETWEEN 8 AND 120
            THEN a.speed_seconds::int ELSE NULL END,
       CASE WHEN a.updated_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = a.updated_by::uuid)
            THEN a.updated_by::uuid ELSE NULL END,
       COALESCE(NULLIF(a.updated_at, '')::timestamptz, now())
  FROM _stg_announcement a
ON CONFLICT (id) DO NOTHING;

-- 7) Platform admins (record only; only admins who were actually imported)
INSERT INTO platform_admins (user_id, note, created_at)
SELECT pa.user_id::uuid, NULLIF(pa.note, ''),
       COALESCE(NULLIF(pa.created_at, '')::timestamptz, now())
  FROM _stg_platform_admins pa
 WHERE EXISTS (SELECT 1 FROM users u WHERE u.user_id = pa.user_id::uuid)
ON CONFLICT (user_id) DO NOTHING;

-- 8) Platform service bank accounts
INSERT INTO platform_service_bank_accounts
    (id, slot, holder_name, bank_name, account_number, enabled, sort, configured_by, created_at, updated_at)
SELECT b.id::uuid,
       b.slot::int,
       COALESCE(NULLIF(btrim(b.holder_name), ''), 'Unknown'),
       COALESCE(NULLIF(btrim(b.bank_name), ''), 'Unknown'),
       COALESCE(NULLIF(btrim(b.account_number), ''), '-'),
       COALESCE(NULLIF(b.enabled, '')::boolean, true),
       COALESCE(NULLIF(b.sort, '')::int, 0),
       CASE WHEN b.configured_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = b.configured_by::uuid)
            THEN b.configured_by::uuid ELSE NULL END,
       COALESCE(NULLIF(b.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(b.updated_at, '')::timestamptz, now())
  FROM _stg_bank_accounts b
 WHERE b.slot IN ('1','2')
ON CONFLICT (id) DO NOTHING;
