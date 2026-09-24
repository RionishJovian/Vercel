CREATE TABLE public.service_purchases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL,
  developer_id uuid,
  service_key text NOT NULL CHECK (service_key IN ('monthly', 'annual', 'plus')),
  service_label text NOT NULL,
  price_mmk integer NOT NULL CHECK (price_mmk >= 0),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  receipt_object_key text,
  receipt_mime text,
  receipt_size_bytes integer,
  reference text,
  idempotency_key text NOT NULL,
  reject_reason text,
  decided_by uuid,
  decided_at timestamp with time zone,
  activated_expires_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now(),
  UNIQUE (user_id, idempotency_key)
);

CREATE UNIQUE INDEX service_purchases_one_pending
  ON public.service_purchases (user_id, service_key)
  WHERE status = 'pending';

CREATE INDEX service_purchases_status_idx ON public.service_purchases (status, created_at DESC);

GRANT SELECT, INSERT ON public.service_purchases TO authenticated;
GRANT ALL ON public.service_purchases TO service_role;

ALTER TABLE public.service_purchases ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users read their own purchases"
  ON public.service_purchases FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.has_role(auth.uid(), 'owner'::public.app_role)
    OR public.has_role(auth.uid(), 'admin'::public.app_role)
  );

CREATE POLICY "Users submit their own pending purchases"
  ON public.service_purchases FOR INSERT TO authenticated
  WITH CHECK (user_id = auth.uid() AND status = 'pending');

CREATE TRIGGER touch_service_purchases
  BEFORE UPDATE ON public.service_purchases
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE TABLE public.account_entitlements (
  user_id uuid PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid,
  tier text NOT NULL DEFAULT 'trial' CHECK (tier IN ('trial', 'monthly', 'annual')),
  tier_expires_at timestamp with time zone,
  plus boolean NOT NULL DEFAULT false,
  plus_since timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now()
);

GRANT SELECT ON public.account_entitlements TO authenticated;
GRANT ALL ON public.account_entitlements TO service_role;

ALTER TABLE public.account_entitlements ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users read their own entitlement"
  ON public.account_entitlements FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.has_role(auth.uid(), 'owner'::public.app_role)
    OR public.has_role(auth.uid(), 'admin'::public.app_role)
  );

CREATE TRIGGER touch_account_entitlements
  BEFORE UPDATE ON public.account_entitlements
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE TABLE public.service_purchase_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  purchase_id uuid NOT NULL REFERENCES public.service_purchases(id) ON DELETE CASCADE,
  actor_user_id uuid,
  action text NOT NULL,
  note text,
  created_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX service_purchase_audit_purchase_idx ON public.service_purchase_audit (purchase_id, created_at DESC);

GRANT SELECT ON public.service_purchase_audit TO authenticated;
GRANT ALL ON public.service_purchase_audit TO service_role;

ALTER TABLE public.service_purchase_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Owners and admins read the purchase audit trail"
  ON public.service_purchase_audit FOR SELECT TO authenticated
  USING (
    public.has_role(auth.uid(), 'owner'::public.app_role)
    OR public.has_role(auth.uid(), 'admin'::public.app_role)
  );