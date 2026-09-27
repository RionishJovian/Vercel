-- Phase 2 · Bucket B — terminal, resellers, print layouts, promo, ruijie cloud, Magic Dude KB.
--
-- Guarding rules used throughout:
--   * every FK target is existence-checked (old rows freely reference users,
--     routers and sites that did not migrate);
--   * NOT NULL text columns with CHECKs get a sensible fallback rather than
--     failing the load;
--   * the `Neon Cafe Wi-Fi` demo placeholder is treated as unset (as in portal_configs).

-- 1) Terminal templates
INSERT INTO terminal_templates
    (id, business_id, created_by, name, description, category, method, path, body, is_builtin, created_at, updated_at)
SELECT t.id::uuid, t.owner_id::uuid,
       CASE WHEN t.created_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = t.created_by::uuid)
            THEN t.created_by::uuid ELSE NULL END,
       COALESCE(NULLIF(t.name, ''), 'untitled template'),
       NULLIF(t.description, ''),
       COALESCE(NULLIF(t.category, ''), 'general'),
       CASE WHEN t.method IN ('GET','POST','PATCH','PUT','DELETE') THEN t.method ELSE 'GET' END,
       CASE WHEN left(t.path, 1) = '/' THEN t.path ELSE '/' || t.path END,
       NULLIF(t.body, ''),
       COALESCE(NULLIF(t.is_builtin, '')::boolean, false),
       COALESCE(NULLIF(t.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(t.updated_at, '')::timestamptz, now())
  FROM _stg_term_templates t
 WHERE t.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 2) Terminal history
INSERT INTO terminal_history
    (id, business_id, user_id, router_connection_id, site_id, method, path, body, status, ms, response_snippet, created_at)
SELECT h.id::uuid, h.owner_id::uuid,
       CASE WHEN h.user_id <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = h.user_id::uuid)
            THEN h.user_id::uuid ELSE NULL END,
       CASE WHEN h.router_id <> '' AND EXISTS (SELECT 1 FROM router_connections r WHERE r.id = h.router_id::uuid)
            THEN h.router_id::uuid ELSE NULL END,
       CASE WHEN h.site_id <> '' AND EXISTS (SELECT 1 FROM sites s WHERE s.site_id = h.site_id::uuid)
            THEN h.site_id::uuid ELSE NULL END,
       COALESCE(NULLIF(h.method, ''), 'GET'),
       COALESCE(NULLIF(h.path, ''), '/'),
       NULLIF(h.body, ''),
       NULLIF(h.status, '')::int,
       NULLIF(h.ms, '')::int,
       NULLIF(h.response_snippet, ''),
       COALESCE(NULLIF(h.created_at, '')::timestamptz, now())
  FROM _stg_term_history h
 WHERE h.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 3) Voucher resellers
INSERT INTO voucher_resellers
    (id, business_id, shop_name, contact_name, location, phone, active, code_prefix, created_at, updated_at)
SELECT r.id::uuid, r.owner_id::uuid,
       left(COALESCE(NULLIF(r.shop_name, ''), 'Reseller'), 120),
       left(COALESCE(NULLIF(r.contact_name, ''), 'Unknown'), 120),
       NULLIF(r.location, ''), NULLIF(r.phone, ''),
       COALESCE(NULLIF(r.active, '')::boolean, true),
       COALESCE(NULLIF(r.code_prefix, ''), 'RS'),
       COALESCE(NULLIF(r.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(r.updated_at, '')::timestamptz, now())
  FROM _stg_resellers r
 WHERE r.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 4) Voucher print layouts (one per business; every CHECK'd text column falls back)
INSERT INTO voucher_print_layouts
    (business_id, business_name, wifi_name, support_contact, terms, paper_width_mm,
     show_qr, show_price, show_expiry, a4_columns, a4_rows_per_page, thermal_columns,
     thermal_rows_per_strip, created_at, updated_at)
SELECT p.owner_id::uuid,
       left(CASE WHEN NULLIF(p.business_name, '') IS NULL OR p.business_name = 'Neon Cafe Wi-Fi'
                 THEN b.company_name ELSE p.business_name END, 80),
       left(COALESCE(NULLIF(p.wifi_name, ''), 'WiFi'), 80),
       left(COALESCE(NULLIF(p.support_contact, ''), b.company_name || ' support'), 120),
       left(COALESCE(NULLIF(p.terms, ''), 'By connecting you agree to our fair-use policy.'), 600),
       CASE WHEN p.paper_width_mm IN ('58','80') THEN p.paper_width_mm::int ELSE 80 END,
       COALESCE(NULLIF(p.show_qr, '')::boolean, true),
       COALESCE(NULLIF(p.show_price, '')::boolean, true),
       COALESCE(NULLIF(p.show_expiry, '')::boolean, true),
       CASE WHEN p.a4_columns IN ('2','3','4','5') THEN p.a4_columns::int ELSE 3 END,
       CASE WHEN p.a4_rows_per_page ~ '^[0-9]+$' AND p.a4_rows_per_page::int BETWEEN 4 AND 50
            THEN p.a4_rows_per_page::int ELSE 10 END,
       CASE WHEN p.thermal_columns IN ('1','2') THEN p.thermal_columns::int ELSE 2 END,
       CASE WHEN p.thermal_rows_per_strip ~ '^[0-9]+$' AND p.thermal_rows_per_strip::int BETWEEN 4 AND 40
            THEN p.thermal_rows_per_strip::int ELSE 10 END,
       COALESCE(NULLIF(p.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(p.updated_at, '')::timestamptz, now())
  FROM _stg_print_layouts p
  JOIN businesses b ON b.business_id = p.owner_id::uuid
ON CONFLICT (business_id) DO NOTHING;

-- 5) Pricing promo (platform singleton)
INSERT INTO pricing_promo (id, active, starts_on, ends_on, label,
                           monthly_promo_mmk, monthly_standard_mmk,
                           annual_promo_mmk, annual_standard_mmk, updated_at)
SELECT true,
       COALESCE(NULLIF(p.active, '')::boolean, false),
       NULLIF(p.starts_on, '')::date, NULLIF(p.ends_on, '')::date, NULLIF(p.label, ''),
       NULLIF(p.monthly_promo_mmk, '')::int, NULLIF(p.monthly_standard_mmk, '')::int,
       NULLIF(p.annual_promo_mmk, '')::int, NULLIF(p.annual_standard_mmk, '')::int,
       COALESCE(NULLIF(p.updated_at, '')::timestamptz, now())
  FROM _stg_promo p
ON CONFLICT (id) DO NOTHING;

-- 6) Ruijie cloud connections (legacy ciphertext carried, not usable)
INSERT INTO ruijie_cloud_connections
    (id, business_id, region, base_url, legacy_app_id_ciphertext, legacy_app_secret_ciphertext,
     site_ref, device_ref, enabled, last_test_at, last_test_status, last_sync_at,
     created_by, token_path, device_path, created_at, updated_at)
SELECT c.id::uuid, c.owner_id::uuid, NULLIF(c.region, ''), NULLIF(c.base_url, ''),
       NULLIF(c.app_id_ciphertext, ''), NULLIF(c.app_secret_ciphertext, ''),
       NULLIF(c.site_ref, ''), NULLIF(c.device_ref, ''),
       COALESCE(NULLIF(c.enabled, '')::boolean, true),
       NULLIF(c.last_test_at, '')::timestamptz, NULLIF(c.last_test_status, ''),
       NULLIF(c.last_sync_at, '')::timestamptz,
       CASE WHEN c.created_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = c.created_by::uuid)
            THEN c.created_by::uuid ELSE NULL END,
       NULLIF(c.token_path, ''), NULLIF(c.device_path, ''),
       COALESCE(NULLIF(c.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(c.updated_at, '')::timestamptz, now())
  FROM _stg_ruijie c
 WHERE c.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;

-- 7) Magic Dude knowledge notes (platform-wide editorial content)
INSERT INTO magic_dude_knowledge_notes (id, title, body_md, sort, active, updated_by, created_at, updated_at)
SELECT n.id::uuid,
       left(COALESCE(NULLIF(btrim(n.title), ''), 'Note'), 200),
       left(COALESCE(NULLIF(btrim(n.body_md), ''), '—'), 20000),
       COALESCE(NULLIF(n.sort, '')::int, 0),
       COALESCE(NULLIF(n.active, '')::boolean, true),
       CASE WHEN n.updated_by <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = n.updated_by::uuid)
            THEN n.updated_by::uuid ELSE NULL END,
       COALESCE(NULLIF(n.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(n.updated_at, '')::timestamptz, now())
  FROM _stg_kb_notes n
ON CONFLICT (id) DO NOTHING;

-- 8) Magic Dude solved cases
INSERT INTO magic_dude_solved_cases
    (id, business_id, user_id, question, answer, language, helpful_count, created_at, last_used_at)
SELECT s.id::uuid, s.owner_id::uuid,
       CASE WHEN s.user_id <> '' AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = s.user_id::uuid)
            THEN s.user_id::uuid ELSE NULL END,
       left(COALESCE(NULLIF(btrim(s.question), ''), 'Question'), 4000),
       left(COALESCE(NULLIF(btrim(s.answer), ''), '—'), 8000),
       CASE WHEN s.language IN ('en','my','zh') THEN s.language ELSE NULL END,
       GREATEST(COALESCE(NULLIF(s.helpful_count, '')::int, 1), 1),
       COALESCE(NULLIF(s.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(s.last_used_at, '')::timestamptz, now())
  FROM _stg_solved s
 WHERE s.owner_id::uuid IN (SELECT business_id FROM businesses)
ON CONFLICT (id) DO NOTHING;
