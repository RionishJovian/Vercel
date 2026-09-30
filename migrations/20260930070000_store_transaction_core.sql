-- Tenant-safe Store purchase core for the current Go engine. The migrated
-- deployment retained ledgers and entitlements but not service_purchases.
CREATE TABLE IF NOT EXISTS public.service_purchases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES public.businesses(business_id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES public.users(user_id) ON DELETE RESTRICT,
  service_key text NOT NULL CHECK (service_key IN ('monthly','annual','magic_coins')),
  service_label text NOT NULL,
  price_mmk integer NOT NULL CHECK (price_mmk > 0),
  coin_amount numeric(12,2),
  magic_coin_purchase_amount numeric(12,2),
  payment_method text NOT NULL CHECK (payment_method IN ('bank_transfer','magic_coins')),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled','refunded')),
  bank_account_id uuid REFERENCES public.platform_service_bank_accounts(id) ON DELETE RESTRICT,
  bank_reference text,
  receipt_path text,
  idempotency_key text NOT NULL,
  decided_by uuid REFERENCES public.users(user_id) ON DELETE RESTRICT,
  decided_at timestamptz,
  reject_reason text,
  activated_expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, user_id, idempotency_key),
  CHECK ((payment_method = 'bank_transfer' AND bank_account_id IS NOT NULL AND bank_reference IS NOT NULL)
      OR (payment_method = 'magic_coins' AND bank_account_id IS NULL)),
  CHECK ((service_key = 'magic_coins' AND magic_coin_purchase_amount > 0)
      OR (service_key <> 'magic_coins' AND magic_coin_purchase_amount IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS service_purchases_one_pending
  ON public.service_purchases(business_id, user_id, service_key) WHERE status = 'pending';
CREATE UNIQUE INDEX IF NOT EXISTS service_purchases_bank_reference_once
  ON public.service_purchases(business_id, lower(bank_reference)) WHERE bank_reference IS NOT NULL AND status <> 'rejected';
CREATE INDEX IF NOT EXISTS service_purchases_tenant_history
  ON public.service_purchases(business_id, created_at DESC);

CREATE TABLE IF NOT EXISTS public.service_purchase_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES public.businesses(business_id) ON DELETE CASCADE,
  purchase_id uuid NOT NULL REFERENCES public.service_purchases(id) ON DELETE RESTRICT,
  actor_user_id uuid REFERENCES public.users(user_id) ON DELETE RESTRICT,
  action text NOT NULL,
  note text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS service_purchase_audit_tenant
  ON public.service_purchase_audit(business_id, purchase_id, created_at DESC);

ALTER TABLE public.service_purchases ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.service_purchase_audit ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS service_purchases_tenant_isolation ON public.service_purchases;
CREATE POLICY service_purchases_tenant_isolation ON public.service_purchases
  USING (business_id = current_setting('app.current_business_id', true)::uuid)
  WITH CHECK (business_id = current_setting('app.current_business_id', true)::uuid);
DROP POLICY IF EXISTS service_purchase_audit_tenant_isolation ON public.service_purchase_audit;
CREATE POLICY service_purchase_audit_tenant_isolation ON public.service_purchase_audit
  USING (business_id = current_setting('app.current_business_id', true)::uuid)
  WITH CHECK (business_id = current_setting('app.current_business_id', true)::uuid);

-- Tenant transactions switch to this non-owner role. Table creation by the
-- migration owner does not grant it access automatically.
GRANT SELECT, INSERT, UPDATE ON public.service_purchases TO authenticated;
GRANT SELECT, INSERT ON public.service_purchase_audit TO authenticated;
