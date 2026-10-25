BEGIN;
-- Historical rows remain NULL: the original duration cannot be invented.
ALTER TABLE public.guest_purchase_requests ADD COLUMN IF NOT EXISTS purchased_minutes integer
 CHECK(purchased_minutes IS NULL OR purchased_minutes BETWEEN 0 AND 525600);
COMMIT;
