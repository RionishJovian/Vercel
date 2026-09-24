-- Operator feature grants (Users page) — shop-floor actions for client/agent.
-- Same pattern as portal_mode_grants: role defaults + per-user extras.

CREATE TABLE IF NOT EXISTS public.operator_feature_grants (
  user_id uuid NOT NULL,
  feature text NOT NULL,
  granted_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, feature),
  CONSTRAINT operator_feature_grants_feature_check CHECK (feature IN (
    'vouchers',
    'portal_deploy',
    'cash_sales',
    'bank_edit',
    'reboot',
    'alerts',
    'syslog_ai',
    'poe',
    'telegram'
  ))
);

GRANT SELECT ON public.operator_feature_grants TO authenticated;
GRANT ALL ON public.operator_feature_grants TO service_role;
ALTER TABLE public.operator_feature_grants ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "operator_feature_grants self read" ON public.operator_feature_grants;
CREATE POLICY "operator_feature_grants self read" ON public.operator_feature_grants FOR SELECT TO authenticated
USING (
  user_id = auth.uid()
  OR public.is_platform_admin(auth.uid())
  OR public.has_role(auth.uid(), 'owner'::public.app_role)
  OR public.has_role(auth.uid(), 'admin'::public.app_role)
);

DROP POLICY IF EXISTS "operator_feature_grants owner write" ON public.operator_feature_grants;
CREATE POLICY "operator_feature_grants owner write" ON public.operator_feature_grants FOR ALL TO authenticated
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

CREATE TABLE IF NOT EXISTS public.operator_feature_role_defaults (
  role public.app_role NOT NULL,
  feature text NOT NULL,
  updated_by uuid NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (role, feature),
  CONSTRAINT operator_feature_role_defaults_feature_check CHECK (feature IN (
    'vouchers',
    'portal_deploy',
    'cash_sales',
    'bank_edit',
    'reboot',
    'alerts',
    'syslog_ai',
    'poe',
    'telegram'
  )),
  CONSTRAINT operator_feature_role_defaults_role_check CHECK (role IN (
    'client'::public.app_role,
    'agent'::public.app_role,
    'site_manager'::public.app_role
  ))
);

GRANT SELECT ON public.operator_feature_role_defaults TO authenticated;
GRANT ALL ON public.operator_feature_role_defaults TO service_role;
ALTER TABLE public.operator_feature_role_defaults ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "operator_feature_role_defaults read" ON public.operator_feature_role_defaults;
CREATE POLICY "operator_feature_role_defaults read" ON public.operator_feature_role_defaults FOR SELECT TO authenticated USING (true);

DROP POLICY IF EXISTS "operator_feature_role_defaults owner write" ON public.operator_feature_role_defaults;
CREATE POLICY "operator_feature_role_defaults owner write" ON public.operator_feature_role_defaults FOR ALL TO authenticated
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
