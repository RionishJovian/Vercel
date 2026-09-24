
-- voucher_prices
CREATE TABLE public.voucher_prices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL,
  site_id UUID REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id UUID REFERENCES public.router_connections(id) ON DELETE SET NULL,
  profile TEXT NOT NULL,
  price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
  currency TEXT NOT NULL DEFAULT 'USD',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX voucher_prices_owner_idx ON public.voucher_prices(owner_id);
CREATE UNIQUE INDEX voucher_prices_scope_uniq ON public.voucher_prices(
  owner_id, profile, COALESCE(site_id, '00000000-0000-0000-0000-000000000000'::uuid),
  COALESCE(router_id, '00000000-0000-0000-0000-000000000000'::uuid)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.voucher_prices TO authenticated;
GRANT ALL ON public.voucher_prices TO service_role;

ALTER TABLE public.voucher_prices ENABLE ROW LEVEL SECURITY;

CREATE POLICY "voucher_prices_select"
  ON public.voucher_prices FOR SELECT TO authenticated
  USING (
    public.has_role(auth.uid(), 'owner'::app_role)
    OR owner_id = public.effective_owner(auth.uid())
  );

CREATE POLICY "voucher_prices_write"
  ON public.voucher_prices FOR ALL TO authenticated
  USING (
    (public.has_role(auth.uid(), 'owner'::app_role) OR owner_id = public.effective_owner(auth.uid()))
    AND NOT public.has_role(auth.uid(), 'read_only'::app_role)
    AND NOT public.has_role(auth.uid(), 'expired'::app_role)
  )
  WITH CHECK (
    (public.has_role(auth.uid(), 'owner'::app_role) OR owner_id = public.effective_owner(auth.uid()))
    AND NOT public.has_role(auth.uid(), 'read_only'::app_role)
    AND NOT public.has_role(auth.uid(), 'expired'::app_role)
  );

CREATE TRIGGER trg_voucher_prices_touch
  BEFORE UPDATE ON public.voucher_prices
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- voucher_sales
CREATE TABLE public.voucher_sales (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL,
  site_id UUID REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id UUID REFERENCES public.router_connections(id) ON DELETE SET NULL,
  profile TEXT NOT NULL,
  code TEXT NOT NULL,
  price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
  currency TEXT NOT NULL DEFAULT 'USD',
  note TEXT,
  sold_by UUID,
  sold_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX voucher_sales_owner_idx ON public.voucher_sales(owner_id, sold_at DESC);
CREATE INDEX voucher_sales_site_idx ON public.voucher_sales(site_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.voucher_sales TO authenticated;
GRANT ALL ON public.voucher_sales TO service_role;

ALTER TABLE public.voucher_sales ENABLE ROW LEVEL SECURITY;

CREATE POLICY "voucher_sales_select"
  ON public.voucher_sales FOR SELECT TO authenticated
  USING (
    public.has_role(auth.uid(), 'owner'::app_role)
    OR owner_id = public.effective_owner(auth.uid())
  );

CREATE POLICY "voucher_sales_insert"
  ON public.voucher_sales FOR INSERT TO authenticated
  WITH CHECK (
    (public.has_role(auth.uid(), 'owner'::app_role) OR owner_id = public.effective_owner(auth.uid()))
    AND NOT public.has_role(auth.uid(), 'read_only'::app_role)
    AND NOT public.has_role(auth.uid(), 'expired'::app_role)
  );

CREATE POLICY "voucher_sales_modify"
  ON public.voucher_sales FOR UPDATE TO authenticated
  USING (
    public.has_role(auth.uid(), 'owner'::app_role)
    OR owner_id = public.effective_owner(auth.uid())
  );

CREATE POLICY "voucher_sales_delete"
  ON public.voucher_sales FOR DELETE TO authenticated
  USING (
    public.has_role(auth.uid(), 'owner'::app_role)
    OR owner_id = public.effective_owner(auth.uid())
  );
