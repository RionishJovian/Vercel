-- 1. Extend portal_plans with real plan template fields
ALTER TABLE public.portal_plans
  ADD COLUMN IF NOT EXISTS plan_key text,
  ADD COLUMN IF NOT EXISTS duration_minutes integer,
  ADD COLUMN IF NOT EXISTS device_limit integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS rate_limit text,
  ADD COLUMN IF NOT EXISTS price_mmk integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS is_vip boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS manual_code text,
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

DROP TRIGGER IF EXISTS touch_portal_plans ON public.portal_plans;
CREATE TRIGGER touch_portal_plans
  BEFORE UPDATE ON public.portal_plans
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 2. Voucher code tracking (one code = one device)
CREATE TABLE IF NOT EXISTS public.voucher_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  plan_key text NOT NULL,
  plan_label text NOT NULL,
  code text NOT NULL,
  price_mmk integer NOT NULL DEFAULT 0,
  duration_minutes integer,
  device_mac text,
  first_seen_at timestamptz,
  expires_at timestamptz,
  status text NOT NULL DEFAULT 'unused',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_id, code)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.voucher_codes TO authenticated;
GRANT ALL ON public.voucher_codes TO service_role;

ALTER TABLE public.voucher_codes ENABLE ROW LEVEL SECURITY;

CREATE POLICY "voucher_codes read own tenant"
  ON public.voucher_codes FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "voucher_codes write own tenant"
  ON public.voucher_codes FOR ALL TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()) AND NOT public.is_expired(auth.uid()));

CREATE INDEX IF NOT EXISTS voucher_codes_owner_status_idx
  ON public.voucher_codes (owner_id, status, expires_at);

DROP TRIGGER IF EXISTS touch_voucher_codes ON public.voucher_codes;
CREATE TRIGGER touch_voucher_codes
  BEFORE UPDATE ON public.voucher_codes
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- 3. Seed defaults for an owner
CREATE OR REPLACE FUNCTION public.seed_owner_defaults(_owner uuid)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.portal_settings (owner_id, business_name, welcome_text, terms, primary_hex, glass_tint_hex)
  VALUES (
    _owner,
    'Neon Cafe Wi-Fi',
    'Enter the voucher code from the front desk to get online.',
    'By connecting you agree to our fair-use policy.',
    '#ffb547',
    '#7ad0ff'
  )
  ON CONFLICT (owner_id) DO NOTHING;

  IF NOT EXISTS (SELECT 1 FROM public.portal_plans WHERE owner_id = _owner) THEN
    INSERT INTO public.portal_plans
      (owner_id, label, price_label, duration_label, sort, plan_key, duration_minutes, device_limit, rate_limit, price_mmk, is_vip)
    VALUES
      (_owner, '1 Day',   '1000 MMK', '24 hours', 0, '1d',  1440,   1, '5M/5M',  1000, false),
      (_owner, '7 Days',  '5000 MMK', '7 days',   1, '7d',  10080,  1, '5M/5M',  5000, false),
      (_owner, '1 Month', '15000 MMK','30 days',  2, '1m',  43200,  1, '5M/5M', 15000, false),
      (_owner, 'VIP',     '50000 MMK','Unlimited',3, 'vip', NULL,   0, NULL,    50000, true);
  END IF;
END;
$$;

REVOKE EXECUTE ON FUNCTION public.seed_owner_defaults(uuid) FROM PUBLIC, anon, authenticated;

-- 4. Hook seeding into new-user creation
CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.profiles (id, display_name)
  VALUES (NEW.id, COALESCE(NEW.raw_user_meta_data->>'display_name', split_part(NEW.email, '@', 1)));

  INSERT INTO public.user_roles (user_id, role, owner_id, expires_at)
  VALUES (NEW.id, 'client', NEW.id, now() + INTERVAL '1 month');

  PERFORM public.seed_owner_defaults(NEW.id);

  RETURN NEW;
END;
$$;

-- 5. Backfill existing accounts
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN SELECT DISTINCT user_id FROM public.user_roles LOOP
    PERFORM public.seed_owner_defaults(r.user_id);
  END LOOP;
END;
$$;

-- 6. Backfill plan_key for pre-existing custom plans
UPDATE public.portal_plans SET plan_key = 'custom-' || left(id::text, 8) WHERE plan_key IS NULL;