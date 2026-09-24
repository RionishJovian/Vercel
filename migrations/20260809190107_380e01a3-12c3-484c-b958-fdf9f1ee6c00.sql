CREATE TABLE public.pricing_promo (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  active boolean NOT NULL DEFAULT true,
  starts_on date NOT NULL DEFAULT '2026-08-10',
  ends_on date NOT NULL DEFAULT '2026-09-10',
  label text NOT NULL DEFAULT 'MikroMagic launch gift',
  monthly_promo_mmk integer NOT NULL DEFAULT 70000,
  monthly_standard_mmk integer NOT NULL DEFAULT 100000,
  annual_promo_mmk integer NOT NULL DEFAULT 700000,
  annual_standard_mmk integer NOT NULL DEFAULT 1000000,
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON public.pricing_promo TO anon;
GRANT SELECT, UPDATE ON public.pricing_promo TO authenticated;
GRANT ALL ON public.pricing_promo TO service_role;

ALTER TABLE public.pricing_promo ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Anyone can read the pricing promo" ON public.pricing_promo FOR SELECT USING (true);
CREATE POLICY "Owners can update the pricing promo" ON public.pricing_promo FOR UPDATE TO authenticated USING (public.has_role(auth.uid(), 'owner')) WITH CHECK (public.has_role(auth.uid(), 'owner'));

INSERT INTO public.pricing_promo (id) VALUES (true);