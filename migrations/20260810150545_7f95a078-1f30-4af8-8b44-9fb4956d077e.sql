-- 1. Extend order state machine
ALTER TABLE public.payment_orders DROP CONSTRAINT IF EXISTS payment_orders_status_check;
ALTER TABLE public.payment_orders ADD CONSTRAINT payment_orders_status_check
  CHECK (status = ANY (ARRAY['pending','receipt_submitted','pending_review','approved','rejected','expired','settled','failed','refunded','cancelled']));
ALTER TABLE public.payment_orders DROP CONSTRAINT IF EXISTS payment_orders_method_check;
ALTER TABLE public.payment_orders ADD CONSTRAINT payment_orders_method_check
  CHECK (method = ANY (ARRAY['online','cash','bank_transfer']));

-- 2. Bank accounts (owner-configurable payment info boxes)
CREATE TABLE public.payment_bank_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  slot integer NOT NULL CHECK (slot IN (1,2)),
  holder_name text NOT NULL DEFAULT '',
  bank_name text NOT NULL DEFAULT '',
  account_number text NOT NULL DEFAULT '',
  enabled boolean NOT NULL DEFAULT false,
  sort integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_id, slot)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON public.payment_bank_accounts TO authenticated;
GRANT ALL ON public.payment_bank_accounts TO service_role;
ALTER TABLE public.payment_bank_accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY "bank_accounts_read" ON public.payment_bank_accounts FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "bank_accounts_write" ON public.payment_bank_accounts FOR ALL TO authenticated
  USING ((public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid())) AND NOT public.is_expired(auth.uid()))
  WITH CHECK ((public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid())) AND NOT public.is_expired(auth.uid()));
CREATE TRIGGER touch_payment_bank_accounts BEFORE UPDATE ON public.payment_bank_accounts
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 3. Receipts
CREATE TABLE public.payment_receipts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  order_id uuid NOT NULL REFERENCES public.payment_orders(id) ON DELETE CASCADE,
  object_key text NOT NULL,
  mime_type text NOT NULL,
  size_bytes integer NOT NULL CHECK (size_bytes > 0),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','superseded','rejected','approved')),
  reference text,
  reject_reason text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz,
  reviewed_by uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX payment_receipts_one_active ON public.payment_receipts (order_id) WHERE status = 'active';
CREATE INDEX payment_receipts_owner_idx ON public.payment_receipts (owner_id, submitted_at DESC);
GRANT SELECT, UPDATE ON public.payment_receipts TO authenticated;
GRANT ALL ON public.payment_receipts TO service_role;
ALTER TABLE public.payment_receipts ENABLE ROW LEVEL SECURITY;
CREATE POLICY "receipts_read" ON public.payment_receipts FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));
CREATE POLICY "receipts_update" ON public.payment_receipts FOR UPDATE TO authenticated
  USING ((public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid())) AND NOT public.is_expired(auth.uid()))
  WITH CHECK (public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));
CREATE TRIGGER touch_payment_receipts BEFORE UPDATE ON public.payment_receipts
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 4. One-time review tokens (server-only; no authenticated grants)
CREATE TABLE public.payment_review_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  order_id uuid NOT NULL REFERENCES public.payment_orders(id) ON DELETE CASCADE,
  receipt_id uuid REFERENCES public.payment_receipts(id) ON DELETE SET NULL,
  token_hash text NOT NULL UNIQUE,
  channel text NOT NULL DEFAULT 'telegram',
  actor_ref text,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  used_action text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payment_review_tokens_order_idx ON public.payment_review_tokens (order_id);
GRANT ALL ON public.payment_review_tokens TO service_role;
ALTER TABLE public.payment_review_tokens ENABLE ROW LEVEL SECURITY;

-- 5. Order state audit
CREATE TABLE public.payment_order_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  order_id uuid NOT NULL REFERENCES public.payment_orders(id) ON DELETE CASCADE,
  from_status text,
  to_status text NOT NULL,
  actor text NOT NULL,
  actor_user_id uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  note text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payment_order_audit_order_idx ON public.payment_order_audit (order_id, created_at DESC);
GRANT SELECT ON public.payment_order_audit TO authenticated;
GRANT ALL ON public.payment_order_audit TO service_role;
ALTER TABLE public.payment_order_audit ENABLE ROW LEVEL SECURITY;
CREATE POLICY "order_audit_read" ON public.payment_order_audit FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(),'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));