-- Phase 2 · Bucket B — Magic Coins: old Supabase wallets/ledger -> ported tables.
--
-- Two things the ported schema forces:
--   * `trg_user_magic_coin_wallet` auto-creates a wallet for every user inserted
--     during Phase 1, so a plain INSERT would collide -> UPSERT the balance.
--   * `trg_magic_coin_transaction_immutable` blocks UPDATE/DELETE on the ledger
--     (INSERT is fine), so the ledger is loaded once and never rewritten.
--
-- `owner_id` (old auth user id) IS the preserved `business_id`.
-- The 5 unlock-key columns are carried as ids; their FKs land with those tables.

-- 1) Wallets — upsert, because the users trigger already created empty ones.
INSERT INTO magic_coin_wallets (user_id, business_id, balance, created_at, updated_at)
SELECT w.user_id::uuid, u.business_id, COALESCE(NULLIF(w.balance, '')::numeric, 0),
       COALESCE(NULLIF(w.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(w.updated_at, '')::timestamptz, now())
  FROM _stg_coin_wallets w
  JOIN users u ON u.user_id = w.user_id::uuid
ON CONFLICT (user_id) DO UPDATE
   SET balance = EXCLUDED.balance,
       business_id = EXCLUDED.business_id,
       updated_at = EXCLUDED.updated_at;

-- 2) Ledger — append-only load.
INSERT INTO magic_coin_transactions
    (id, business_id, user_id, delta, balance_after, kind, service_purchase_id, note,
     actor_user_id, webfig_key_activation_id, reseller_add_key_activation_id,
     router_unlock_key_activation_id, magic_dude_unlock_activation_id,
     switch_unlock_key_activation_id, created_at)
SELECT t.id::uuid,
       t.owner_id::uuid,
       t.user_id::uuid,
       NULLIF(t.delta, '')::numeric,
       NULLIF(t.balance_after, '')::numeric,
       CASE WHEN t.kind IN ('agent_earn','agent_reversal','service_payment','service_refund',
                            'feature_key_purchase','purchase_credit','manual_credit','manual_debit')
            THEN t.kind ELSE 'manual_credit' END,
       -- only keep the link when the purchase actually came across
       CASE WHEN t.service_purchase_id <> ''
             AND EXISTS (SELECT 1 FROM purchase_requests p WHERE p.id = t.service_purchase_id::uuid)
            THEN t.service_purchase_id::uuid ELSE NULL END,
       COALESCE(NULLIF(t.note, ''), 'Migrated Magic Coin entry'),
       -- actor must exist, or the FK rejects the row
       CASE WHEN t.actor_user_id <> ''
             AND EXISTS (SELECT 1 FROM users u WHERE u.user_id = t.actor_user_id::uuid)
            THEN t.actor_user_id::uuid ELSE NULL END,
       NULLIF(t.webfig_key_activation_id, '')::uuid,
       NULLIF(t.reseller_add_key_activation_id, '')::uuid,
       NULLIF(t.router_unlock_key_activation_id, '')::uuid,
       NULLIF(t.magic_dude_unlock_activation_id, '')::uuid,
       NULLIF(t.switch_unlock_key_activation_id, '')::uuid,
       COALESCE(NULLIF(t.created_at, '')::timestamptz, now())
  FROM _stg_coin_tx t
 WHERE t.user_id::uuid IN (SELECT user_id FROM users)
   AND t.owner_id::uuid IN (SELECT business_id FROM businesses);
