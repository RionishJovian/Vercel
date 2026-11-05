-- A database built from baseline.sql gave the tenant role access to almost no tables,
-- so sign-in failed at the first tenant query. Production had these grants from before
-- the baseline was taken. This copies production's exact list and changes nothing there.
-- Tables left out on purpose, such as auth_sessions and hub_pending_enrollments, stay
-- closed to the tenant role.

GRANT DELETE, INSERT, SELECT, UPDATE ON
  account_entitlements,
  admin_notifications,
  ai_usage_events,
  alert_policies,
  audit_events,
  business_metered_quotas,
  business_telegram_bots,
  businesses,
  device_allowances,
  device_change_events,
  device_config_backups,
  device_discovery_labels,
  device_requests,
  devices,
  fleet_scan_runs,
  guest_purchase_requests,
  guest_voucher_batch_resellers,
  i18n_overrides,
  incidents,
  magic_coin_transactions,
  magic_coin_wallets,
  magic_dude_knowledge_notes,
  magic_dude_solved_cases,
  magic_media_publications,
  notification_prefs,
  operator_feature_grants,
  operator_feature_role_defaults,
  owner_daily_closes,
  peer_heartbeats,
  platform_announcement,
  portal_configs,
  portal_mode_role_defaults,
  portal_sessions,
  purchase_requests,
  router_connections,
  ruijie_cloud_connections,
  site_portals,
  site_topology_config,
  sites,
  syslog_events,
  telemetry_samples,
  terminal_history,
  terminal_templates,
  unlock_key_activations,
  user_profile_settings,
  users,
  voucher_print_layouts,
  voucher_resellers,
  vouchers,
  webhook_deliveries,
  webhook_endpoints
  TO authenticated;

GRANT INSERT, SELECT ON
  app_account_pass_renewals,
  app_account_trials,
  guest_voucher_batch_reseller_events,
  service_purchase_audit,
  store_addon_requests
  TO authenticated;

GRANT INSERT, SELECT, UPDATE ON
  guest_access_recovery,
  guest_voucher_grants,
  guest_voucher_issue_batches,
  guest_voucher_issue_items,
  guest_voucher_usage,
  management_address_migrations,
  service_purchases
  TO authenticated;

GRANT SELECT ON
  app_account_pass_transitions,
  app_account_passes,
  business_app_primary_users,
  platform_admins,
  platform_service_bank_accounts,
  pricing_promo,
  store_addons
  TO authenticated;

GRANT USAGE, SELECT ON SEQUENCE guest_voucher_batch_reseller_events_id_seq TO authenticated;

-- Trigger functions: production does not let everyone execute these.
REVOKE EXECUTE ON FUNCTION revoke_changed_user_sessions() FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION revoke_suspended_business_sessions() FROM PUBLIC;
