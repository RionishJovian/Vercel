-- Guest wifi (Option C) schema: bring existing DBs in line with init.sql.
--
-- Same class of bug as the RLS/billing drift: schema additions to init.sql must
-- ship an idempotent migration too, or a persistent DB (created before this
-- slice) misses them — the harness's throwaway DB passes while the real dev DB
-- 500s. Everything below is IF-NOT-EXISTS / on-conflict idempotent.

-- 1. portal_configs: guest online-payment window + data plans.
ALTER TABLE portal_configs ADD COLUMN IF NOT EXISTS guest_grace_minutes integer NOT NULL DEFAULT 10;
ALTER TABLE portal_configs ADD COLUMN IF NOT EXISTS guest_plans jsonb NOT NULL DEFAULT '[]'::jsonb;

-- 2. business_telegram_bots: each owner's guest-wifi bot binding.
CREATE TABLE IF NOT EXISTS business_telegram_bots (
    business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
    bot_token text,
    target_chat_id bigint,
    enabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- 3. guest_purchase_requests: owner-approved online-banking wifi submits.
CREATE TABLE IF NOT EXISTS guest_purchase_requests (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    device_id uuid,
    client_mac text,
    client_ip inet,
    plan_key text NOT NULL,
    amount integer NOT NULL DEFAULT 0,
    bank_reference text,
    receipt_path text,
    grace_started_at timestamptz,
    grace_expires_at timestamptz,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_guest_requests_business ON guest_purchase_requests(business_id);
CREATE INDEX IF NOT EXISTS idx_guest_requests_status ON guest_purchase_requests(status);

-- 4. RLS: enable + tenant-isolation policies.
ALTER TABLE business_telegram_bots ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_purchase_requests ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='business_telegram_bots' AND policyname='business_telegram_bots_tenant_isolation') THEN
        EXECUTE 'CREATE POLICY business_telegram_bots_tenant_isolation ON business_telegram_bots
            FOR ALL TO authenticated
            USING (business_id = current_setting(''app.current_business_id'')::uuid)
            WITH CHECK (business_id = current_setting(''app.current_business_id'')::uuid)';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='guest_purchase_requests' AND policyname='guest_requests_tenant_isolation') THEN
        EXECUTE 'CREATE POLICY guest_requests_tenant_isolation ON guest_purchase_requests
            FOR ALL TO authenticated
            USING (business_id = current_setting(''app.current_business_id'')::uuid)
            WITH CHECK (business_id = current_setting(''app.current_business_id'')::uuid)';
    END IF;
END;
$$;

-- 5. authenticated role grants for the new tables (GRANT ON ALL TABLES does not
-- retroactively cover tables created after it ran).
GRANT SELECT, INSERT, UPDATE, DELETE ON business_telegram_bots TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON guest_purchase_requests TO authenticated;