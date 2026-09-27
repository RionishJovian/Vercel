-- Phase 2 · Bucket B (final) — ops history bulk: syslog, fleet scan runs, AI usage,
-- admin notifications.
--
-- DELIBERATELY NOT MIGRATED: `connector_rate_limits` (31 rows). It is transient
-- rate-limiter state, not history — its newest window is 2026-09-19 (9 days stale).
-- Carrying stale buckets into the new stack risks throttling live users on day one
-- in exchange for zero recoverable information. Rebuilt naturally by the limiter.

-- 1) Syslog events
INSERT INTO syslog_events
    (id, business_id, router_connection_id, site_id, source_ip, facility, severity,
     program, message, ai_summary, ai_severity, received_at)
SELECT s.id::uuid, s.owner_id::uuid,
       CASE WHEN s.router_id <> '' AND EXISTS (SELECT 1 FROM router_connections r WHERE r.id = s.router_id::uuid)
            THEN s.router_id::uuid ELSE NULL END,
       CASE WHEN s.site_id <> '' AND EXISTS (SELECT 1 FROM sites t WHERE t.site_id = s.site_id::uuid)
            THEN s.site_id::uuid ELSE NULL END,
       NULLIF(s.source_ip, ''), NULLIF(s.facility, ''),
       CASE WHEN s.severity IN ('info','warning','critical') THEN s.severity ELSE 'info' END,
       NULLIF(s.program, ''),
       COALESCE(NULLIF(s.message, ''), '(empty)'),
       NULLIF(s.ai_summary, ''),
       CASE WHEN s.ai_severity IN ('info','warning','critical') THEN s.ai_severity ELSE NULL END,
       COALESCE(NULLIF(s.received_at, '')::timestamptz, now())
  FROM _stg_syslog s
 WHERE s.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 2) Fleet scan runs
INSERT INTO fleet_scan_runs
    (id, business_id, kind, payload, max_severity, router_count, generated_at, triggered_by)
SELECT f.id::uuid, f.owner_id::uuid,
       CASE WHEN f.kind IN ('ai','security') THEN f.kind ELSE 'security' END,
       CASE WHEN f.payload IS NULL OR btrim(f.payload) = '' THEN '{}'::jsonb ELSE f.payload::jsonb END,
       CASE WHEN f.max_severity IN ('critical','warning','info','ok') THEN f.max_severity ELSE 'ok' END,
       COALESCE(NULLIF(f.router_count, '')::int, 0),
       COALESCE(NULLIF(f.generated_at, '')::timestamptz, now()),
       CASE WHEN f.triggered_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = f.triggered_by::uuid)
            THEN f.triggered_by::uuid ELSE NULL END
  FROM _stg_scan_runs f
 WHERE f.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 3) AI usage events
INSERT INTO ai_usage_events
    (id, business_id, user_id, feature, model, prompt_tokens, completion_tokens, total_tokens, cached, created_at)
SELECT a.id::uuid, a.owner_id::uuid,
       CASE WHEN a.user_id <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = a.user_id::uuid)
            THEN a.user_id::uuid ELSE NULL END,
       COALESCE(NULLIF(a.feature, ''), 'unknown'),
       NULLIF(a.model, ''),
       COALESCE(NULLIF(a.prompt_tokens, '')::int, 0),
       COALESCE(NULLIF(a.completion_tokens, '')::int, 0),
       COALESCE(NULLIF(a.total_tokens, '')::int, 0),
       COALESCE(NULLIF(a.cached, '')::boolean, false),
       COALESCE(NULLIF(a.created_at, '')::timestamptz, now())
  FROM _stg_ai_usage a
 WHERE a.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 4) Admin notifications (recipient must exist; skips notifications addressed to
--    users who did not migrate)
INSERT INTO admin_notifications
    (id, recipient_user_id, kind, title, body, data, read_at, created_at)
SELECT n.id::uuid, n.recipient_id::uuid,
       COALESCE(NULLIF(n.kind, ''), 'info'),
       COALESCE(NULLIF(n.title, ''), '(untitled)'),
       NULLIF(n.body, ''),
       CASE WHEN n.data IS NULL OR btrim(n.data) = '' THEN '{}'::jsonb ELSE n.data::jsonb END,
       NULLIF(n.read_at, '')::timestamptz,
       COALESCE(NULLIF(n.created_at, '')::timestamptz, now())
  FROM _stg_admin_notifs n
 WHERE EXISTS (SELECT 1 FROM users u WHERE u.user_id = n.recipient_id::uuid)
ON CONFLICT (id) DO NOTHING;
