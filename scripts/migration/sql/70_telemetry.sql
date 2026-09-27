-- Phase 2 · Bucket A — telemetry: old `device_health_samples` -> existing `telemetry_samples`.
--
-- The two tables line up almost 1:1; no schema change was needed.
--   owner_id -> business_id, cpu_usage_pct -> cpu_pct, memory_usage_pct -> memory_pct
--   subject_kind/subject_id/latency/uptime/wan/connector/tunnel/observed_at copied
-- Enums are coerced so no CHECK can fail:
--   subject_kind ∈ {site,router,device,connector}, wan_state ∈ {up,degraded,down,unknown},
--   connector_state ∈ {online,stale,offline,none}, tunnel_state ∈ {up,down,none}
INSERT INTO telemetry_samples
    (id, business_id, device_id, subject_kind, subject_id, cpu_pct, memory_pct,
     latency_ms, uptime_seconds, wan_state, connector_state, tunnel_state, observed_at)
SELECT h.id::uuid,
       h.owner_id::uuid,
       CASE WHEN h.device_id <> '' AND EXISTS (SELECT 1 FROM devices d WHERE d.device_id = h.device_id::uuid)
            THEN h.device_id::uuid ELSE NULL END,
       CASE WHEN h.subject_kind IN ('site','router','device','connector') THEN h.subject_kind ELSE 'router' END,
       COALESCE(NULLIF(h.subject_id, ''), h.device_id, ''),
       NULLIF(h.cpu_usage_pct, '')::numeric,
       NULLIF(h.memory_usage_pct, '')::numeric,
       NULLIF(h.latency_ms, '')::int,
       NULLIF(h.uptime_seconds, '')::bigint,
       CASE WHEN h.wan_state IN ('up','degraded','down','unknown') THEN h.wan_state ELSE 'unknown' END,
       CASE WHEN h.connector_state IN ('online','stale','offline','none') THEN h.connector_state ELSE 'none' END,
       CASE WHEN h.tunnel_state IN ('up','down','none') THEN h.tunnel_state ELSE 'none' END,
       COALESCE(NULLIF(h.observed_at, '')::timestamptz, now())
  FROM _stg_health h
 WHERE h.owner_id::uuid IN (SELECT old_user_id::uuid FROM _mig_accounts);
