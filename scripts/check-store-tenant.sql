-- Run as the migration owner with psql -X -v ON_ERROR_STOP=1.
-- Tests real runtime grants and RLS without retaining purchases or sending
-- notifications. An error closes the connection and rolls back the transaction.
BEGIN;
SELECT u.business_id AS business_id, u.user_id AS user_id, a.id AS bank_id
FROM public.users u CROSS JOIN public.platform_service_bank_accounts a
WHERE u.business_id IS NOT NULL AND a.enabled=true
ORDER BY u.user_id, a.id LIMIT 1
\gset store_test_
SELECT set_config('app.current_business_id', :'store_test_business_id', true),
       set_config('app.store_test_business_id', :'store_test_business_id', true),
       set_config('app.store_test_user_id', :'store_test_user_id', true),
       set_config('app.store_test_bank_id', :'store_test_bank_id', true)
\gset
SET LOCAL ROLE authenticated;
INSERT INTO public.service_purchases
  (business_id,user_id,service_key,service_label,price_mmk,payment_method,
   bank_account_id,bank_reference,idempotency_key)
VALUES (:'store_test_business_id', :'store_test_user_id', 'annual',
        'ROLLBACK ONLY permission check', 1, 'bank_transfer', :'store_test_bank_id',
        gen_random_uuid()::text, gen_random_uuid()::text)
RETURNING id
\gset store_test_
INSERT INTO public.service_purchase_audit
  (business_id,purchase_id,actor_user_id,action,note)
VALUES (:'store_test_business_id', :'store_test_id', :'store_test_user_id',
        'submitted', 'ROLLBACK ONLY permission check');
UPDATE public.service_purchases SET status='rejected' WHERE id=:'store_test_id';
SELECT set_config('app.store_test_purchase_id', :'store_test_id', true)
\gset
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.service_purchases
                 WHERE id=current_setting('app.store_test_purchase_id')::uuid AND status='rejected')
  THEN RAISE EXCEPTION 'same-tenant purchase read/update failed'; END IF;
END $$;
SELECT set_config('app.current_business_id', gen_random_uuid()::text, true)
\gset
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.service_purchases) OR
     EXISTS (SELECT 1 FROM public.service_purchase_audit)
  THEN RAISE EXCEPTION 'cross-tenant rows are visible'; END IF;
  BEGIN
    INSERT INTO public.service_purchases
      (business_id,user_id,service_key,service_label,price_mmk,payment_method,
       bank_account_id,bank_reference,idempotency_key)
    VALUES (current_setting('app.store_test_business_id')::uuid,
            current_setting('app.store_test_user_id')::uuid,
            'annual','ROLLBACK ONLY forbidden tenant check',1,'bank_transfer',
            current_setting('app.store_test_bank_id')::uuid,
            gen_random_uuid()::text,gen_random_uuid()::text);
    RAISE EXCEPTION 'cross-tenant insert unexpectedly succeeded';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;
ROLLBACK;
\echo Store runtime insert/audit/update and tenant isolation passed; all test writes rolled back.
