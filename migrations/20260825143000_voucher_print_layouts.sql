-- Tenant-owned counter receipt layout. This is separate from guest portal presentation.
CREATE TABLE IF NOT EXISTS public.voucher_print_layouts (
  owner_id uuid PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
  business_name text NOT NULL CHECK (char_length(business_name) BETWEEN 1 AND 80),
  wifi_name text NOT NULL CHECK (char_length(wifi_name) BETWEEN 1 AND 80),
  support_contact text NOT NULL CHECK (char_length(support_contact) BETWEEN 1 AND 120),
  terms text NOT NULL CHECK (char_length(terms) BETWEEN 1 AND 600),
  paper_width_mm integer NOT NULL DEFAULT 80 CHECK (paper_width_mm IN (58, 80)),
  show_qr boolean NOT NULL DEFAULT true,
  show_price boolean NOT NULL DEFAULT true,
  show_expiry boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE public.voucher_print_layouts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS voucher_print_layouts_read ON public.voucher_print_layouts;
CREATE POLICY voucher_print_layouts_read ON public.voucher_print_layouts FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid()));
DROP POLICY IF EXISTS voucher_print_layouts_write ON public.voucher_print_layouts;
CREATE POLICY voucher_print_layouts_write ON public.voucher_print_layouts FOR ALL TO authenticated
  USING ((owner_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid())) AND (public.has_role(auth.uid(), 'primary'::public.app_role) OR public.is_platform_admin(auth.uid())))
  WITH CHECK ((owner_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid())) AND (public.has_role(auth.uid(), 'primary'::public.app_role) OR public.is_platform_admin(auth.uid())));

DROP TRIGGER IF EXISTS touch_voucher_print_layouts ON public.voucher_print_layouts;
CREATE TRIGGER touch_voucher_print_layouts BEFORE UPDATE ON public.voucher_print_layouts
FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();
