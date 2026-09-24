-- Revoke EXECUTE on SECURITY DEFINER functions that must never be callable
-- through the Data API (trigger functions, seeders, cron/admin-only helpers).

-- Trigger functions: only the trigger machinery may run these.
revoke all on function public.create_magic_coin_wallet_for_profile() from public, anon, authenticated;
revoke all on function public.credit_magic_coin_purchase_on_approval() from public, anon, authenticated;
revoke all on function public.credit_magic_coins_from_agent_point() from public, anon, authenticated;
revoke all on function public.enforce_device_quota_trigger() from public, anon, authenticated;
revoke all on function public.enforce_portal_guest_mode_grant() from public, anon, authenticated;
revoke all on function public.guard_magic_coin_service_purchase() from public, anon, authenticated;
revoke all on function public.handle_new_user() from public, anon, authenticated;
revoke all on function public.notify_owners_client_expired() from public, anon, authenticated;
revoke all on function public.refund_magic_coin_service_purchase() from public, anon, authenticated;
revoke all on function public.reject_retired_plus_service_purchase() from public, anon, authenticated;
revoke all on function public.trg_seed_terminal_common_configuration_templates() from public, anon, authenticated;
revoke all on function public.trg_seed_terminal_templates_on_owner() from public, anon, authenticated;
revoke all on function public.validate_reseller_assignment_owner() from public, anon, authenticated;

-- Seeders and cron/admin-only helpers: service role only.
revoke all on function public.seed_owner_defaults(uuid) from public, anon, authenticated;
revoke all on function public.seed_terminal_templates(uuid) from public, anon, authenticated;
revoke all on function public.seed_terminal_common_configuration_templates(uuid) from public, anon, authenticated;
revoke all on function public.expire_stale_clients() from public, anon, authenticated;
revoke all on function public.connector_rate_hit(text, integer, integer) from public, anon, authenticated;
revoke all on function public.import_legacy_router_vouchers(uuid, uuid, uuid, jsonb, text) from public, anon, authenticated;

grant execute on function public.seed_owner_defaults(uuid) to service_role;
grant execute on function public.seed_terminal_templates(uuid) to service_role;
grant execute on function public.seed_terminal_common_configuration_templates(uuid) to service_role;
grant execute on function public.expire_stale_clients() to service_role;
grant execute on function public.connector_rate_hit(text, integer, integer) to service_role;
grant execute on function public.import_legacy_router_vouchers(uuid, uuid, uuid, jsonb, text) to service_role;

-- Signed-in-user RPCs and RLS helpers stay callable by authenticated only
-- (never anon); they all enforce auth.uid() scoping internally.
revoke execute on function public.can_operate_portal_guest_mode(uuid, text) from anon, public;
revoke execute on function public.create_reseller_with_key(text, text, text, text) from anon, public;
revoke execute on function public.get_magic_coin_wallet() from anon, public;
revoke execute on function public.get_magic_dude_unlock() from anon, public;
revoke execute on function public.get_reseller_add_keys() from anon, public;
revoke execute on function public.get_router_unlock_keys() from anon, public;
revoke execute on function public.get_webfig_unlock_key() from anon, public;
revoke execute on function public.has_active_magic_dude_unlock() from anon, public;
revoke execute on function public.has_active_webfig_unlock_key() from anon, public;
revoke execute on function public.magic_dude_is_active_user(uuid) from anon, public;
revoke execute on function public.owner_operations_is_trial_account(uuid) from anon, public;
revoke execute on function public.pay_service_with_magic_coins(text) from anon, public;
revoke execute on function public.purchase_magic_dude_unlock() from anon, public;
revoke execute on function public.purchase_reseller_add_key() from anon, public;
revoke execute on function public.purchase_router_unlock_key() from anon, public;
revoke execute on function public.purchase_webfig_unlock_key() from anon, public;
revoke execute on function public.reactivate_router_with_key(uuid) from anon, public;
revoke execute on function public.reconcile_voucher_ledger(uuid[], text, text) from anon, public;
revoke execute on function public.router_unlock_key_accessible(uuid) from anon, public;