-- Phase 2 · Bucket B (special case) — content owned by the EXCLUDED dev account.
--
-- `nish@dev` (7987488d) was deliberately NOT imported as a tenant — `nish@magic.com`
-- is the canonical DEVELOPER. But that account owns PLATFORM content, not customer
-- data:
--   * 51 terminal templates, every one `is_builtin = true` (the shipped RouterOS
--     template library)
--   * 1 Ruijie cloud connection (real provider config)
--   * 1 Magic Dude solved case
-- Dropping them would delete the built-in library from the product. They are
-- therefore re-homed onto the PLATFORM business (the DEVELOPER's), which is what
-- "platform-owned content" means in this schema.
--
-- Reuses the staging tables loaded by step 2.8 — no re-export needed.

-- 1) Built-in terminal templates owned by a non-migrated owner -> platform business
INSERT INTO terminal_templates
    (id, business_id, created_by, name, description, category, method, path, body, is_builtin, created_at, updated_at)
SELECT t.id::uuid,
       (SELECT b.business_id FROM businesses b JOIN users u USING (business_id)
         WHERE u.user_role = 'DEVELOPER' ORDER BY b.created_at LIMIT 1),
       NULL,
       COALESCE(NULLIF(t.name, ''), 'untitled template'),
       NULLIF(t.description, ''),
       COALESCE(NULLIF(t.category, ''), 'general'),
       CASE WHEN t.method IN ('GET','POST','PATCH','PUT','DELETE') THEN t.method ELSE 'GET' END,
       CASE WHEN left(t.path, 1) = '/' THEN t.path ELSE '/' || t.path END,
       NULLIF(t.body, ''),
       true,
       COALESCE(NULLIF(t.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(t.updated_at, '')::timestamptz, now())
  FROM _stg_term_templates t
 WHERE t.owner_id::uuid NOT IN (SELECT business_id FROM businesses)
   AND COALESCE(NULLIF(t.is_builtin, '')::boolean, false) = true     -- built-ins only
   AND EXISTS (SELECT 1 FROM businesses b JOIN users u USING (business_id)
                WHERE u.user_role = 'DEVELOPER')
ON CONFLICT (id) DO NOTHING;

-- 2) Ruijie cloud connection owned by a non-migrated owner -> platform business
INSERT INTO ruijie_cloud_connections
    (id, business_id, region, base_url, legacy_app_id_ciphertext, legacy_app_secret_ciphertext,
     site_ref, device_ref, enabled, last_test_at, last_test_status, last_sync_at,
     created_by, token_path, device_path, created_at, updated_at)
SELECT c.id::uuid,
       (SELECT b.business_id FROM businesses b JOIN users u USING (business_id)
         WHERE u.user_role = 'DEVELOPER' ORDER BY b.created_at LIMIT 1),
       NULLIF(c.region, ''), NULLIF(c.base_url, ''),
       NULLIF(c.app_id_ciphertext, ''), NULLIF(c.app_secret_ciphertext, ''),
       NULLIF(c.site_ref, ''), NULLIF(c.device_ref, ''),
       COALESCE(NULLIF(c.enabled, '')::boolean, true),
       NULLIF(c.last_test_at, '')::timestamptz, NULLIF(c.last_test_status, ''),
       NULLIF(c.last_sync_at, '')::timestamptz,
       NULL,
       NULLIF(c.token_path, ''), NULLIF(c.device_path, ''),
       COALESCE(NULLIF(c.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(c.updated_at, '')::timestamptz, now())
  FROM _stg_ruijie c
 WHERE c.owner_id::uuid NOT IN (SELECT business_id FROM businesses)
   AND EXISTS (SELECT 1 FROM businesses b JOIN users u USING (business_id)
                WHERE u.user_role = 'DEVELOPER')
ON CONFLICT (id) DO NOTHING;

-- 3) Magic Dude solved case owned by a non-migrated owner -> platform business
INSERT INTO magic_dude_solved_cases
    (id, business_id, user_id, question, answer, language, helpful_count, created_at, last_used_at)
SELECT s.id::uuid,
       (SELECT b.business_id FROM businesses b JOIN users u USING (business_id)
         WHERE u.user_role = 'DEVELOPER' ORDER BY b.created_at LIMIT 1),
       NULL,
       left(COALESCE(NULLIF(btrim(s.question), ''), 'Question'), 4000),
       left(COALESCE(NULLIF(btrim(s.answer), ''), '—'), 8000),
       CASE WHEN s.language IN ('en','my','zh') THEN s.language ELSE NULL END,
       GREATEST(COALESCE(NULLIF(s.helpful_count, '')::int, 1), 1),
       COALESCE(NULLIF(s.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(s.last_used_at, '')::timestamptz, now())
  FROM _stg_solved s
 WHERE s.owner_id::uuid NOT IN (SELECT business_id FROM businesses)
   AND EXISTS (SELECT 1 FROM businesses b JOIN users u USING (business_id)
                WHERE u.user_role = 'DEVELOPER')
ON CONFLICT (id) DO NOTHING;
