-- Phase 2 · Bucket A — audit trail: SIX old audit tables -> existing `audit_events`.
--
-- The old product kept one audit table per subsystem; the new schema has a single
-- unified `audit_events`. Each old table is folded in as a UNION ALL, with the
-- fields that have no column preserved in `after_state` JSONB.
--
-- audit_events.user_role is NOT NULL, so the actor's role is looked up from the
-- migrated users (their ids are preserved); 'system' when there is no actor.
-- entity_id/reason/action are NOT NULL -> coalesced.
INSERT INTO audit_events
    (id, business_id, user_id, user_role, action, entity_type, entity_id,
     before_state, after_state, reason, timestamp)
SELECT src.id, src.business_id,
       -- Some old audit rows reference users who were never migrated (deleted
       -- accounts) or the excluded test/dev ones; the FK would reject them.
       CASE WHEN src.user_id IS NOT NULL AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = src.user_id)
            THEN src.user_id ELSE NULL END,
       COALESCE((SELECT u.user_role FROM users u WHERE u.user_id = src.user_id), 'system'),
       COALESCE(NULLIF(src.action, ''), 'unknown'),
       src.entity_type,
       COALESCE(src.entity_id, ''),
       src.before_state,
       src.after_state,
       COALESCE(src.reason, ''),
       src.ts
  FROM (
        -- router operations (3,410)
        SELECT s.id::uuid id, s.owner_id::uuid business_id, NULLIF(s.user_id, '')::uuid user_id,
               s.action, 'router' entity_type, s.router_id entity_id,
               NULL::jsonb before_state,
               jsonb_strip_nulls(jsonb_build_object('outcome', NULLIF(s.outcome,''),
                                                    'environment', NULLIF(s.environment,''),
                                                    'detail', NULLIF(s.detail,''),
                                                    'error_message', NULLIF(s.error_message,''),
                                                    'duration_ms', NULLIF(s.duration_ms,'')::int)) after_state,
               NULLIF(s.outcome,'') reason,
               COALESCE(NULLIF(s.created_at,'')::timestamptz, now()) ts
          FROM _stg_audit_routerops s
        UNION ALL
        -- router save (445)
        SELECT s.id::uuid, s.owner_id::uuid, NULLIF(s.user_id,'')::uuid,
               s.action, 'router', s.router_id,
               NULL::jsonb,
               jsonb_strip_nulls(jsonb_build_object('success', NULLIF(s.success,'')::boolean,
                                                    'error_message', NULLIF(s.error_message,''),
                                                    'error_code', NULLIF(s.error_code,''),
                                                    'attempted_name', NULLIF(s.attempted_name,''),
                                                    'attempted_host', NULLIF(s.attempted_host,''))),
               NULLIF(s.error_message,''),
               COALESCE(NULLIF(s.created_at,'')::timestamptz, now())
          FROM _stg_audit_routersave s
        UNION ALL
        -- portal deploy (12)
        SELECT s.id::uuid, s.owner_id::uuid, NULLIF(s.user_id,'')::uuid,
               'portal_deploy', 'router', s.router_id,
               NULLIF(s.snapshot,'')::jsonb,
               jsonb_strip_nulls(jsonb_build_object('ok', NULLIF(s.ok,'')::boolean,
                                                    'files', NULLIF(s.files,'')::jsonb,
                                                    'error', NULLIF(s.error,''),
                                                    'duration_ms', NULLIF(s.duration_ms,'')::int)),
               NULLIF(s.error,''),
               COALESCE(NULLIF(s.created_at,'')::timestamptz, now())
          FROM _stg_audit_portaldeploy s
        UNION ALL
        -- ruijie cloud (10)
        SELECT s.id::uuid, s.owner_id::uuid, NULLIF(s.user_id,'')::uuid,
               s.action, 'cloud', NULL,
               NULL::jsonb,
               jsonb_strip_nulls(jsonb_build_object('result', NULLIF(s.result,''),
                                                    'metadata', NULLIF(s.metadata,'')::jsonb)),
               NULLIF(s.result,''),
               COALESCE(NULLIF(s.created_at,'')::timestamptz, now())
          FROM _stg_audit_ruijie s
        UNION ALL
        -- notifier (5)
        SELECT s.id::uuid, s.owner_id::uuid, NULLIF(s.actor_user_id,'')::uuid,
               s.action, 'notification', NULL,
               NULL::jsonb,
               jsonb_strip_nulls(jsonb_build_object('channel', NULLIF(s.channel,''),
                                                    'outcome', NULLIF(s.outcome,''),
                                                    'detail', NULLIF(s.detail,''))),
               NULLIF(s.outcome,''),
               COALESCE(NULLIF(s.created_at,'')::timestamptz, now())
          FROM _stg_audit_notifier s
        UNION ALL
        -- service purchase audit (38) — owner resolved from the parent purchase
        SELECT s.id::uuid, s.owner_id::uuid, NULLIF(s.actor_user_id,'')::uuid,
               s.action, 'service_purchase', s.purchase_id,
               NULL::jsonb,
               jsonb_strip_nulls(jsonb_build_object('note', NULLIF(s.note,''))),
               NULLIF(s.note,''),
               COALESCE(NULLIF(s.created_at,'')::timestamptz, now())
          FROM _stg_audit_spa s
       ) src
 WHERE src.business_id IN (SELECT old_user_id::uuid FROM _mig_accounts);
