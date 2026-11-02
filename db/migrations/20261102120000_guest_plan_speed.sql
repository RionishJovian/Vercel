-- Guest plan speed tiers. The speed promised when a guest buys a plan is saved with the request,
-- like the time and data allowance, so editing a plan later never changes what a guest bought.
-- 0/0 means no speed limit. Both are set or neither is; each is 1 to 10000 Mbps.
ALTER TABLE guest_purchase_requests
    ADD COLUMN IF NOT EXISTS speed_down_mbps integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS speed_up_mbps integer NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'guest_purchase_requests_speed_chk') THEN
        ALTER TABLE guest_purchase_requests
            ADD CONSTRAINT guest_purchase_requests_speed_chk
            CHECK (speed_down_mbps BETWEEN 0 AND 10000 AND speed_up_mbps BETWEEN 0 AND 10000
                   AND ((speed_down_mbps = 0) = (speed_up_mbps = 0)));
    END IF;
END $$;
