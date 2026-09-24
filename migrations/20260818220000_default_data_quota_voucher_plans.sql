-- Default data-quota voucher plans: 500MB, 1GB, 2GB, 3GB, 5GB, 7GB, 10GB.
-- New accounts receive them from seed_owner_defaults.
-- Existing accounts receive any missing plan_key (time plans are left as-is).

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
      (owner_id, label, price_label, duration_label, sort, plan_key, duration_minutes, device_limit, rate_limit, price_mmk, is_vip, status)
    VALUES
      (_owner, '1 Day',   '1000 MMK', '24 hours', 0, '1d',  1440,   1, '5M/5M',  1000, false, 'active'),
      (_owner, '7 Days',  '5000 MMK', '7 days',   1, '7d',  10080,  1, '5M/5M',  5000, false, 'active'),
      (_owner, '1 Month', '15000 MMK','30 days',  2, '1m',  43200,  1, '5M/5M', 15000, false, 'active'),
      (_owner, 'VIP',     '50000 MMK','Unlimited',3, 'vip', NULL,   0, NULL,    50000, true,  'active');
  END IF;

  INSERT INTO public.portal_plans
    (owner_id, label, price_label, duration_label, sort, plan_key, duration_minutes, device_limit, rate_limit, price_mmk, is_vip, data_quota_mb, validity_days, status)
  SELECT v.owner_id, v.label, v.price_label, v.duration_label, v.sort, v.plan_key,
         v.duration_minutes, v.device_limit, v.rate_limit, v.price_mmk, v.is_vip,
         v.data_quota_mb, v.validity_days, v.status
  FROM (
    VALUES
      (_owner, '500 MB', '500 MMK',  '500 MB', 10, '500mb', NULL::integer, 1, '5M/5M',  500, false,  500, 30, 'active'),
      (_owner, '1 GB',  '1000 MMK', '1 GB',   11, '1gb',   NULL,          1, '5M/5M', 1000, false, 1000, 30, 'active'),
      (_owner, '2 GB',  '2000 MMK', '2 GB',   12, '2gb',   NULL,          1, '5M/5M', 2000, false, 2000, 30, 'active'),
      (_owner, '3 GB',  '2500 MMK', '3 GB',   13, '3gb',   NULL,          1, '5M/5M', 2500, false, 3000, 30, 'active'),
      (_owner, '5 GB',  '4000 MMK', '5 GB',   14, '5gb',   NULL,          1, '5M/5M', 4000, false, 5000, 30, 'active'),
      (_owner, '7 GB',  '5500 MMK', '7 GB',   15, '7gb',   NULL,          1, '5M/5M', 5500, false, 7000, 30, 'active'),
      (_owner, '10 GB', '7000 MMK', '10 GB',  16, '10gb',  NULL,          1, '5M/5M', 7000, false, 10000, 30, 'active')
  ) AS v(owner_id, label, price_label, duration_label, sort, plan_key, duration_minutes, device_limit, rate_limit, price_mmk, is_vip, data_quota_mb, validity_days, status)
  WHERE NOT EXISTS (
    SELECT 1
    FROM public.portal_plans p
    WHERE p.owner_id = _owner
      AND p.plan_key = v.plan_key
  );
END;
$$;

REVOKE EXECUTE ON FUNCTION public.seed_owner_defaults(uuid) FROM PUBLIC, anon, authenticated;

-- Fill missing data-quota templates on every existing tenant.
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT oid FROM (
      SELECT owner_id AS oid FROM public.portal_settings
      UNION
      SELECT owner_id FROM public.portal_plans
      UNION
      SELECT COALESCE(owner_id, user_id) FROM public.user_roles
    ) s
    WHERE oid IS NOT NULL
  LOOP
    PERFORM public.seed_owner_defaults(r.oid);
  END LOOP;
END;
$$;
