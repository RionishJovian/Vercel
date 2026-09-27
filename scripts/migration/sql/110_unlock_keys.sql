-- Phase 2 · Bucket B — the five old unlock-key tables -> ONE `unlock_key_activations`.
--
-- Each old table becomes a `key_kind`; the columns they don't have stay NULL
-- (magic_dude has no key_id/key_name). `consumed_*_id` columns collapse into the
-- generic `consumed_ref_id`, which preserves the original name of what it points at.
-- Expiry/consistency invariants the old rigid CHECKs enforced are preserved by the
-- ported CHECKs (expires_at > created_at, consumed_at/null pairing).
INSERT INTO unlock_key_activations
    (id, business_id, user_id, key_kind, key_id, key_name, price_coins, attributes,
     account_bound, non_transferable, unique_router_key, grace_days,
     consumed_at, consumed_ref_id, created_at, expires_at)
SELECT k.id::uuid, k.owner_id::uuid, k.user_id::uuid, k.key_kind,
       NULLIF(k.key_id, '')::int,
       NULLIF(k.key_name, ''),
       COALESCE(NULLIF(k.price_coins, '')::numeric, 0),
       COALESCE(NULLIF(k.attributes, ''), ''),
       COALESCE(NULLIF(k.account_bound, '')::boolean, true),
       COALESCE(NULLIF(k.non_transferable, '')::boolean, true),
       COALESCE(NULLIF(k.unique_router_key, '')::boolean, false),
       NULLIF(k.grace_days, '')::int,
       NULLIF(k.consumed_at, '')::timestamptz,
       NULLIF(k.consumed_ref_id, '')::uuid,
       COALESCE(NULLIF(k.created_at, '')::timestamptz, now()),
       -- expires_at has CHECK (> created_at); nudge forward if the source is degenerate
       GREATEST(
           COALESCE(NULLIF(k.expires_at, '')::timestamptz, now()),
           COALESCE(NULLIF(k.created_at, '')::timestamptz, now()) + interval '1 second'
       )
  FROM _stg_unlock_keys k
 WHERE k.user_id::uuid IN (SELECT user_id FROM users)
   AND k.owner_id::uuid IN (SELECT business_id FROM businesses);
