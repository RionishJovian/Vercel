-- Phase 2 · Bucket A — purchases: old `service_purchases` -> existing `purchase_requests`.
--
-- bank_reference has CHECK (length(btrim(bank_reference)) > 0) but only 5 of 19 old
-- rows carry a real `reference`, so a synthesised reference is used as fallback and
-- keeps the payment method legible ('MAGIC_COINS:…' for the 3 coin-paid rows).
-- status CHECK allows only {pending, approved, rejected} — the old values
-- (approved 16 / rejected 3) already fit.
INSERT INTO purchase_requests
    (id, business_id, device_id, plan_key, amount, bank_reference, duration_minutes,
     receipt_path, status, decided_at, created_at, updated_at)
SELECT s.id::uuid,
       s.owner_id::uuid,
       NULL,
       COALESCE(NULLIF(s.service_key, ''), 'service'),
       COALESCE(NULLIF(s.price_mmk, '')::int, 0),
       COALESCE(NULLIF(btrim(s.reference), ''),
                CASE WHEN s.payment_method = 'magic_coins'
                     THEN 'MAGIC_COINS:' || left(s.id, 8)
                     ELSE 'LEGACY:' || left(s.id, 8) END),
       0,
       NULLIF(s.receipt_object_key, ''),
       CASE WHEN s.status IN ('pending','approved','rejected') THEN s.status ELSE 'pending' END,
       NULLIF(s.decided_at, '')::timestamptz,
       COALESCE(NULLIF(s.created_at, '')::timestamptz, now()),
       COALESCE(NULLIF(s.updated_at, '')::timestamptz, now())
  FROM _stg_service_purchases s
 WHERE s.owner_id::uuid IN (SELECT old_user_id::uuid FROM _mig_accounts);
