-- Separate high-impact router provisioning from reboot/Multi-WAN permission.
ALTER TABLE public.operator_feature_grants
  DROP CONSTRAINT IF EXISTS operator_feature_grants_feature_check;
ALTER TABLE public.operator_feature_grants
  ADD CONSTRAINT operator_feature_grants_feature_check CHECK (feature IN (
    'vouchers', 'portal_deploy', 'cash_sales', 'bank_edit', 'router_config',
    'reboot', 'alerts', 'syslog_ai', 'poe', 'telegram'
  ));

ALTER TABLE public.operator_feature_role_defaults
  DROP CONSTRAINT IF EXISTS operator_feature_role_defaults_feature_check;
ALTER TABLE public.operator_feature_role_defaults
  ADD CONSTRAINT operator_feature_role_defaults_feature_check CHECK (feature IN (
    'vouchers', 'portal_deploy', 'cash_sales', 'bank_edit', 'router_config',
    'reboot', 'alerts', 'syslog_ai', 'poe', 'telegram'
  ));
