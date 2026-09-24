-- =========================================================
-- Increment 3: monetization foundations
-- =========================================================

-- ---------- payment_orders ----------
CREATE TABLE public.payment_orders (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  plan_id uuid REFERENCES public.portal_plans(id) ON DELETE SET NULL,
  plan_key text,
  plan_label text NOT NULL,
  amount_minor integer NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL DEFAULT 'MMK',
  method text NOT NULL DEFAULT 'online' CHECK (method IN ('online','cash')),
  provider text NOT NULL DEFAULT 'manual',
  provider_ref text,
  checkout_url text,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','settled','failed','refunded','cancelled')),
  device_mac text,
  contact_hint text,
  idempotency_key text NOT NULL,
  voucher_code_id uuid REFERENCES public.voucher_codes(id) ON DELETE SET NULL,
  issued_code text,
  fulfilled_at timestamptz,
  settled_at timestamptz,
  refunded_at timestamptz,
  failure_reason text,
  note text,
  created_by uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX payment_orders_idem_key ON public.payment_orders (owner_id, idempotency_key);
CREATE INDEX payment_orders_owner_created ON public.payment_orders (owner_id, created_at DESC);
CREATE INDEX payment_orders_provider_ref ON public.payment_orders (provider, provider_ref);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.payment_orders TO authenticated;
GRANT ALL ON public.payment_orders TO service_role;
ALTER TABLE public.payment_orders ENABLE ROW LEVEL SECURITY;

CREATE POLICY payment_orders_select ON public.payment_orders FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(), 'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));
CREATE POLICY payment_orders_insert ON public.payment_orders FOR INSERT TO authenticated
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()));
CREATE POLICY payment_orders_update ON public.payment_orders FOR UPDATE TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));
CREATE POLICY payment_orders_delete ON public.payment_orders FOR DELETE TO authenticated
  USING (public.has_role(auth.uid(), 'owner'::public.app_role));

CREATE TRIGGER touch_payment_orders BEFORE UPDATE ON public.payment_orders
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- ---------- payment_events ----------
CREATE TABLE public.payment_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid REFERENCES auth.users(id) ON DELETE CASCADE,
  order_id uuid REFERENCES public.payment_orders(id) ON DELETE CASCADE,
  provider text NOT NULL,
  event_id text NOT NULL,
  event_type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  outcome text,
  processed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX payment_events_provider_event ON public.payment_events (provider, event_id);
CREATE INDEX payment_events_order ON public.payment_events (order_id, created_at DESC);

GRANT SELECT ON public.payment_events TO authenticated;
GRANT ALL ON public.payment_events TO service_role;
ALTER TABLE public.payment_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY payment_events_select ON public.payment_events FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(), 'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));

-- ---------- hotspot_sessions ----------
CREATE TABLE public.hotspot_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  voucher_code_id uuid REFERENCES public.voucher_codes(id) ON DELETE SET NULL,
  code text,
  plan_key text,
  plan_label text,
  external_session_id text,
  device_mac text,
  device_ip text,
  username text,
  started_at timestamptz NOT NULL DEFAULT now(),
  ended_at timestamptz,
  duration_seconds integer NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
  bytes_in bigint NOT NULL DEFAULT 0 CHECK (bytes_in >= 0),
  bytes_out bigint NOT NULL DEFAULT 0 CHECK (bytes_out >= 0),
  termination_reason text,
  source_seen_at timestamptz,
  reconcile_status text NOT NULL DEFAULT 'open'
    CHECK (reconcile_status IN ('open','closed','stale','queued')),
  reconcile_note text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX hotspot_sessions_external_key
  ON public.hotspot_sessions (owner_id, router_id, external_session_id)
  WHERE external_session_id IS NOT NULL;
CREATE INDEX hotspot_sessions_owner_started ON public.hotspot_sessions (owner_id, started_at DESC);
CREATE INDEX hotspot_sessions_status ON public.hotspot_sessions (owner_id, reconcile_status);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.hotspot_sessions TO authenticated;
GRANT ALL ON public.hotspot_sessions TO service_role;
ALTER TABLE public.hotspot_sessions ENABLE ROW LEVEL SECURITY;

CREATE POLICY hotspot_sessions_select ON public.hotspot_sessions FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(), 'owner'::public.app_role) OR owner_id = public.effective_owner(auth.uid()));
CREATE POLICY hotspot_sessions_write ON public.hotspot_sessions FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()));

CREATE TRIGGER touch_hotspot_sessions BEFORE UPDATE ON public.hotspot_sessions
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- ---------- portal_checkout_tokens ----------
CREATE TABLE public.portal_checkout_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  token text NOT NULL UNIQUE,
  label text,
  enabled boolean NOT NULL DEFAULT true,
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX portal_checkout_tokens_owner ON public.portal_checkout_tokens (owner_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.portal_checkout_tokens TO authenticated;
GRANT ALL ON public.portal_checkout_tokens TO service_role;
ALTER TABLE public.portal_checkout_tokens ENABLE ROW LEVEL SECURITY;

CREATE POLICY portal_checkout_tokens_all ON public.portal_checkout_tokens FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()));

CREATE TRIGGER touch_portal_checkout_tokens BEFORE UPDATE ON public.portal_checkout_tokens
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- ---------- voucher_codes: link to the order that paid for it ----------
ALTER TABLE public.voucher_codes
  ADD COLUMN order_id uuid REFERENCES public.payment_orders(id) ON DELETE SET NULL;
CREATE INDEX voucher_codes_order ON public.voucher_codes (order_id);