-- Captive portal guest modes + owner/role grants (RouterOS portal feature).
-- Hardened for Lovable SQL Editor: one ADD COLUMN per statement.

ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS guest_mode text NOT NULL DEFAULT 'voucher_only';
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS commerce_channels jsonb NOT NULL DEFAULT '{"pix":false,"online":false,"whatsapp":true,"pos":true}'::jsonb;
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS seller_phone text;
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS seller_label text NOT NULL DEFAULT 'Talk to our seller';
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS trial_minutes integer NOT NULL DEFAULT 10;
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS trial_cooldown_hours integer NOT NULL DEFAULT 12;
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS need_code_label text NOT NULL DEFAULT 'I do not have a code';
ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS ask_desk_hint text NOT NULL DEFAULT 'Ask the front desk for a Wi-Fi voucher code.';

ALTER TABLE public.portal_settings DROP CONSTRAINT IF EXISTS portal_settings_guest_mode_check;
ALTER TABLE public.portal_settings ADD CONSTRAINT portal_settings_guest_mode_check CHECK (guest_mode IN ('voucher_only', 'hybrid_light', 'commerce'));

ALTER TABLE public.portal_settings DROP CONSTRAINT IF EXISTS portal_settings_trial_minutes_check;
ALTER TABLE public.portal_settings ADD CONSTRAINT portal_settings_trial_minutes_check CHECK (trial_minutes >= 1 AND trial_minutes <= 120);

ALTER TABLE public.portal_settings DROP CONSTRAINT IF EXISTS portal_settings_trial_cooldown_check;
ALTER TABLE public.portal_settings ADD CONSTRAINT portal_settings_trial_cooldown_check CHECK (trial_cooldown_hours >= 0 AND trial_cooldown_hours <= 168);

CREATE TABLE IF NOT EXISTS public.portal_mode_grants (
  user_id uuid NOT NULL,
  mode text NOT NULL,
  granted_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, mode),
  CONSTRAINT portal_mode_grants_mode_check CHECK (mode IN ('hybrid_light', 'commerce'))
);

GRANT SELECT ON public.portal_mode_grants TO authenticated;
GRANT ALL ON public.portal_mode_grants TO service_role;
ALTER TABLE public.portal_mode_grants ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "portal_mode_grants self read" ON public.portal_mode_grants;
CREATE POLICY "portal_mode_grants self read" ON public.portal_mode_grants FOR SELECT TO authenticated
USING (
  user_id = auth.uid()
  OR public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::public.app_role)
  OR public.has_role(auth.uid(), 'admin'::public.app_role)
);

DROP POLICY IF EXISTS "portal_mode_grants owner write" ON public.portal_mode_grants;
CREATE POLICY "portal_mode_grants owner write" ON public.portal_mode_grants FOR ALL TO authenticated
USING (
  public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::public.app_role)
  OR public.has_role(auth.uid(), 'admin'::public.app_role)
)
WITH CHECK (
  public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::public.app_role)
  OR public.has_role(auth.uid(), 'admin'::public.app_role)
);

CREATE TABLE IF NOT EXISTS public.portal_mode_role_defaults (
  role public.app_role NOT NULL,
  mode text NOT NULL,
  updated_by uuid NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (role, mode),
  CONSTRAINT portal_mode_role_defaults_mode_check CHECK (mode IN ('hybrid_light', 'commerce')),
  CONSTRAINT portal_mode_role_defaults_role_check CHECK (role IN ('client'::public.app_role, 'agent'::public.app_role, 'site_manager'::public.app_role))
);

GRANT SELECT ON public.portal_mode_role_defaults TO authenticated;
GRANT ALL ON public.portal_mode_role_defaults TO service_role;
ALTER TABLE public.portal_mode_role_defaults ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "portal_mode_role_defaults read" ON public.portal_mode_role_defaults;
CREATE POLICY "portal_mode_role_defaults read" ON public.portal_mode_role_defaults FOR SELECT TO authenticated USING (true);

DROP POLICY IF EXISTS "portal_mode_role_defaults owner write" ON public.portal_mode_role_defaults;
CREATE POLICY "portal_mode_role_defaults owner write" ON public.portal_mode_role_defaults FOR ALL TO authenticated
USING (
  public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::public.app_role)
  OR public.has_role(auth.uid(), 'admin'::public.app_role)
)
WITH CHECK (
  public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::public.app_role)
  OR public.has_role(auth.uid(), 'admin'::public.app_role)
);
