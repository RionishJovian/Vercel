-- Phase 4: expand incident / alert_rule kinds for measured RouterBOARD anomalies.
-- Safe to re-run: drops and recreates the CHECK constraints only.

ALTER TABLE public.alert_rules DROP CONSTRAINT IF EXISTS alert_rules_kind_check;
ALTER TABLE public.incidents DROP CONSTRAINT IF EXISTS incidents_kind_check;

ALTER TABLE public.alert_rules
  ADD CONSTRAINT alert_rules_kind_check CHECK (
    kind IN (
      'site_offline',
      'wan_degraded',
      'connector_stale',
      'poe_ap_offline',
      'provisioning_failed',
      'interface_down',
      'dhcp_pool_high',
      'router_anomaly',
      'wan_saturated',
      'vpn_peer_down'
    )
  );

ALTER TABLE public.incidents
  ADD CONSTRAINT incidents_kind_check CHECK (
    kind IN (
      'site_offline',
      'wan_degraded',
      'connector_stale',
      'poe_ap_offline',
      'provisioning_failed',
      'interface_down',
      'dhcp_pool_high',
      'router_anomaly',
      'wan_saturated',
      'vpn_peer_down'
    )
  );
