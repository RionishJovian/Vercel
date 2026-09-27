-- Phase 1 · Step 1.5b — `portal_settings` (+ its `portal_plans`) -> `portal_configs`.
--
-- portal_configs is one row per business; old settings are one row per owner.
-- The old per-owner plan ROWS become the new guest_plans JSONB ARRAY on that row.
INSERT INTO portal_configs
    (business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex,
     consent_required, consent_text, bank_name, bank_account, bank_holder,
     guest_grace_minutes, guest_plans, mmqr_url, updated_at)
SELECT s.owner_id::uuid,
       -- 'Neon Cafe Wi-Fi' is the old app's DEMO DEFAULT: 10 of 13 owners carry it
       -- verbatim, so it is not a real venue name. Fall back to the owning
       -- business's real name rather than shipping a placeholder to guests.
       CASE WHEN NULLIF(s.business_name, '') IS NULL OR s.business_name = 'Neon Cafe Wi-Fi'
            THEN b.company_name
            ELSE s.business_name END,
       COALESCE(s.welcome_text, 'Enter your voucher code to connect.'),
       COALESCE(s.terms, ''),
       COALESCE(NULLIF(s.primary_hex, ''), '#ffb547'),
       COALESCE(NULLIF(s.glass_tint_hex, ''), '#7ad0ff'),
       true,
       '',
       '',                                                   -- old stores seller_phone/label,
       '',                                                   -- not bank details; left blank
       COALESCE(s.seller_label, ''),
       COALESCE(NULLIF(s.trial_minutes, 0), 10),
       COALESCE(
           (SELECT jsonb_agg(jsonb_build_object(
                       'key',      p.plan_key,
                       'label',    p.label,
                       'minutes',  p.duration_minutes,
                       'amount',   p.price_mmk,
                       'data_mb',  p.data_quota_mb)
                    ORDER BY p.sort NULLS LAST, p.created_at)
              FROM _stg_plans p
             WHERE p.owner_id = s.owner_id
               AND COALESCE(p.status, 'active') <> 'archived'),
           '[]'::jsonb),
       COALESCE(s.guest_mode, ''),
       COALESCE(s.updated_at::timestamptz, now())
  FROM _stg_psettings s
  JOIN businesses b ON b.business_id = s.owner_id::uuid;
