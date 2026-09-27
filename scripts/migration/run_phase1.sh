#!/usr/bin/env bash
#
# Phase 1 ETL — Mikromagic (old, single-tenant) -> Mega Magic (new, multi-tenant).
#
# WHAT IT DOES
#   Copies accounts + the data that already has a home in the new 20-table schema:
#     accounts -> businesses/users   devices -> devices      vouchers -> vouchers
#     portal_settings/plans -> portal_configs      incidents -> incidents
#     alert_rules -> alert_policies
#
# SAFETY
#   * READ-ONLY against the source (only \copy ... TO STDOUT).
#   * Writes only to $TARGET_DB, which is DROPPED AND RECREATED from $BASELINE_DB
#     on every run — so the baseline is never touched and runs are repeatable.
#   * Refuses to run if TARGET_DB == BASELINE_DB.
#
# USAGE
#   ./run_phase1.sh              # rebuild the copy and migrate
#   SRC_DB=mydb ./run_phase1.sh  # override any variable below
#
# REQUIREMENTS: docker with a running source Postgres and target Postgres.
set -uo pipefail

S="$(cd "$(dirname "$0")" && pwd)"
OUT="$S/csv-out"                     # generated CSV exports (gitignored)

# ---- connection settings (override via env) -----------------------------------
SRC_CONTAINER="${SRC_CONTAINER:-supabase-db}"
SRC_USER="${SRC_USER:-postgres}"
SRC_DB="${SRC_DB:-postgres}"

DST_CONTAINER="${DST_CONTAINER:-mega_magic_db_dev}"
DST_USER="${DST_USER:-mega_admin}"
BASELINE_DB="${BASELINE_DB:-mega_magic_dev}"   # pristine, never written to
TARGET_DB="${TARGET_DB:-mega_merge}"           # working copy, rebuilt each run

SRC="docker exec $SRC_CONTAINER psql -U $SRC_USER -d $SRC_DB"
DST="docker exec $DST_CONTAINER psql -U $DST_USER -d $TARGET_DB"
DSTI="docker exec -i $DST_CONTAINER psql -U $DST_USER -d $TARGET_DB"
ADMIN="docker exec $DST_CONTAINER psql -U $DST_USER -d postgres"

step()  { printf '\n\033[1m== %s\033[0m\n' "$*"; }
fail()  { printf '\033[31mFAILED: %s\033[0m\n' "$*" >&2; exit 1; }
sql()   { $DSTI -q -v ON_ERROR_STOP=1 || fail "$1"; }   # run SQL from the sql/ dir

# ---- guards -------------------------------------------------------------------
[ "$TARGET_DB" = "$BASELINE_DB" ] && fail "TARGET_DB must differ from BASELINE_DB ($BASELINE_DB is pristine)"
docker exec "$DST_CONTAINER" true 2>/dev/null || fail "target container '$DST_CONTAINER' not running"
docker exec "$SRC_CONTAINER" true 2>/dev/null || fail "source container '$SRC_CONTAINER' not running"
mkdir -p "$OUT"

export_csv() {  # <sql> <file>
  $SRC -c "\copy ($1) TO STDOUT WITH CSV HEADER" > "$OUT/$2" || fail "export $2"
}
load() {        # <table> <cols> <csv>
  docker exec -i "$DST_CONTAINER" psql -U "$DST_USER" -d "$TARGET_DB" \
    -c "\copy $1 ($2) FROM STDIN WITH CSV HEADER" < "$OUT/$3" >/dev/null || fail "load $3"
}

step "0. rebuild the working copy from the pristine baseline"
$ADMIN -q -c "DROP DATABASE IF EXISTS $TARGET_DB;" || fail "drop $TARGET_DB"
$ADMIN -q -c "CREATE DATABASE $TARGET_DB TEMPLATE $BASELINE_DB;" || fail "create $TARGET_DB"
$DST -q -c "DELETE FROM businesses WHERE company_name IN ('Test WISP','NATSTest','Audit WISP','Dup Base');"

step "1.1 accounts -> businesses / users   (OLD UUIDs PRESERVED)"
export_csv "SELECT u.email, coalesce(max(p.display_name),'') AS full_name, coalesce(max(p.username),'') AS username, CASE WHEN bool_or(ur.role='agent') AND coalesce(max((SELECT s.name FROM public.sites s WHERE s.owner_id=u.id)),'')='' THEN 'OPERATOR' ELSE 'OWNER' END AS new_role, max(u.encrypted_password) AS password_hash, u.id::text AS old_user_id, max(u.created_at) AS created_at, coalesce(max((SELECT s.name FROM public.sites s WHERE s.owner_id=u.id)),'') AS site_name FROM auth.users u LEFT JOIN public.profiles p ON p.id=u.id LEFT JOIN public.user_roles ur ON ur.user_id=u.id WHERE u.email <> ALL (ARRAY['test@mikromagic','nish@dev']) GROUP BY u.email, u.id ORDER BY u.email" accounts.csv
$DST -q -c "DROP TABLE IF EXISTS _mig_accounts; CREATE TABLE _mig_accounts (email text, full_name text, username text, new_role text, password_hash text, old_user_id text, created_at timestamptz, site_name text);"
load _mig_accounts "email,full_name,username,new_role,password_hash,old_user_id,created_at,site_name" accounts.csv
sql "accounts" < "$S/sql/10_accounts_ids.sql"

step "1.4 fleet: managed_devices -> devices"
export_csv "SELECT id::text, owner_id::text, coalesce(name,'') name, coalesce(model,'') model, coalesce(vendor,'') vendor, coalesce(category,'') category, coalesce(transport,'') transport, coalesce(mac,'') mac, coalesce(host,'') host, created_at, updated_at FROM managed_devices" devices.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_devices; CREATE TABLE _stg_devices (id text, owner_id text, name text, model text, vendor text, category text, transport text, mac text, host text, created_at text, updated_at text);"
load _stg_devices "id,owner_id,name,model,vendor,category,transport,mac,host,created_at,updated_at" devices.csv
sql "devices" < "$S/sql/20_devices.sql"

step "1.5a money: voucher_codes -> vouchers"
export_csv "SELECT id::text, owner_id::text, coalesce(plan_key,'') plan_key, coalesce(plan_label,'') plan_label, code, price_mmk, duration_minutes, coalesce(device_mac,'') device_mac, status, coalesce(first_seen_at::text,'') first_seen_at, coalesce(expires_at::text,'') expires_at, created_at, updated_at FROM voucher_codes" vouchers.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_vouchers; CREATE TABLE _stg_vouchers (id text, owner_id text, plan_key text, plan_label text, code text, price_mmk int, duration_minutes int, device_mac text, status text, first_seen_at text, expires_at text, created_at text, updated_at text);"
load _stg_vouchers "id,owner_id,plan_key,plan_label,code,price_mmk,duration_minutes,device_mac,status,first_seen_at,expires_at,created_at,updated_at" vouchers.csv
sql "vouchers" < "$S/sql/30_vouchers.sql"

step "1.5b portal_settings + portal_plans -> portal_configs"
export_csv "SELECT owner_id::text, coalesce(business_name,'') business_name, coalesce(welcome_text,'') welcome_text, coalesce(terms,'') terms, coalesce(primary_hex,'') primary_hex, coalesce(glass_tint_hex,'') glass_tint_hex, coalesce(trial_minutes,0) trial_minutes, coalesce(seller_label,'') seller_label, coalesce(guest_mode,'') guest_mode, updated_at FROM portal_settings" psettings.csv
export_csv "SELECT owner_id::text, coalesce(plan_key,'') plan_key, coalesce(label,'') label, coalesce(duration_minutes,0) duration_minutes, coalesce(price_mmk,0) price_mmk, coalesce(data_quota_mb,0) data_quota_mb, coalesce(sort,0) sort, coalesce(status,'active') status, created_at FROM portal_plans" plans.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_psettings; CREATE TABLE _stg_psettings (owner_id text, business_name text, welcome_text text, terms text, primary_hex text, glass_tint_hex text, trial_minutes int, seller_label text, guest_mode text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_plans; CREATE TABLE _stg_plans (owner_id text, plan_key text, label text, duration_minutes int, price_mmk int, data_quota_mb int, sort int, status text, created_at text);"
load _stg_psettings "owner_id,business_name,welcome_text,terms,primary_hex,glass_tint_hex,trial_minutes,seller_label,guest_mode,updated_at" psettings.csv
load _stg_plans "owner_id,plan_key,label,duration_minutes,price_mmk,data_quota_mb,sort,status,created_at" plans.csv
sql "portal configs" < "$S/sql/40_portal.sql"

step "1.6a ops: incidents -> incidents"
export_csv "SELECT id::text, owner_id::text, coalesce(device_id::text,'') device_id, coalesce(kind,'') kind, coalesce(severity,'') severity, coalesce(subject_id,'') subject_id, coalesce(subject_label,'') subject_label, coalesce(detail,'') detail, coalesce(opened_at::text,'') opened_at, coalesce(acknowledged_at::text,'') acknowledged_at, coalesce(acknowledged_by::text,'') acknowledged_by, coalesce(resolved_at::text,'') resolved_at FROM incidents" incidents.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_incidents; CREATE TABLE _stg_incidents (id text, owner_id text, device_id text, kind text, severity text, subject_id text, subject_label text, detail text, opened_at text, acknowledged_at text, acknowledged_by text, resolved_at text);"
load _stg_incidents "id,owner_id,device_id,kind,severity,subject_id,subject_label,detail,opened_at,acknowledged_at,acknowledged_by,resolved_at" incidents.csv
sql "incidents" < "$S/sql/50_incidents.sql"

step "1.6b alert_rules -> alert_policies"
export_csv "SELECT id::text, owner_id::text, coalesce(kind,'') kind, coalesce(enabled,true) enabled, coalesce(severity,'') severity, coalesce(threshold,0) threshold, coalesce(cooldown_minutes,30) cooldown_minutes, created_at, updated_at FROM alert_rules" alertrules.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_alert_rules; CREATE TABLE _stg_alert_rules (id text, owner_id text, kind text, enabled boolean, severity text, threshold numeric, cooldown_minutes int, created_at text, updated_at text);"
load _stg_alert_rules "id,owner_id,kind,enabled,severity,threshold,cooldown_minutes,created_at,updated_at" alertrules.csv
sql "alert policies" < "$S/sql/60_alert_policies.sql"

# ---------------------------------------------------------------------------
# PHASE 2 · Bucket A — old rows that fit EXISTING new tables (no schema change)
# ---------------------------------------------------------------------------
step "2.1 telemetry: device_health_samples -> telemetry_samples"
export_csv "SELECT id::text, owner_id::text, coalesce(device_id::text,'') device_id, coalesce(subject_kind,'') subject_kind, coalesce(subject_id,'') subject_id, coalesce(cpu_usage_pct::text,'') cpu_usage_pct, coalesce(memory_usage_pct::text,'') memory_usage_pct, coalesce(latency_ms::text,'') latency_ms, coalesce(uptime_seconds::text,'') uptime_seconds, coalesce(wan_state,'') wan_state, coalesce(connector_state,'') connector_state, coalesce(tunnel_state,'') tunnel_state, coalesce(observed_at::text,'') observed_at FROM device_health_samples" health.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_health; CREATE TABLE _stg_health (id text, owner_id text, device_id text, subject_kind text, subject_id text, cpu_usage_pct text, memory_usage_pct text, latency_ms text, uptime_seconds text, wan_state text, connector_state text, tunnel_state text, observed_at text);"
load _stg_health "id,owner_id,device_id,subject_kind,subject_id,cpu_usage_pct,memory_usage_pct,latency_ms,uptime_seconds,wan_state,connector_state,tunnel_state,observed_at" health.csv
sql "telemetry" < "$S/sql/70_telemetry.sql"

step "2.2 audit trail: 6 old audit tables -> audit_events"
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(action,'') action, coalesce(router_id::text,'') router_id, coalesce(outcome,'') outcome, coalesce(environment,'') environment, coalesce(detail,'') detail, coalesce(error_message,'') error_message, coalesce(duration_ms::text,'') duration_ms, coalesce(created_at::text,'') created_at FROM router_ops_audit" audit_routerops.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(action,'') action, coalesce(router_id::text,'') router_id, coalesce(success::text,'') success, coalesce(error_message,'') error_message, coalesce(error_code,'') error_code, coalesce(attempted_name,'') attempted_name, coalesce(attempted_host,'') attempted_host, coalesce(created_at::text,'') created_at FROM router_save_audit" audit_routersave.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(router_id::text,'') router_id, coalesce(ok::text,'') ok, coalesce(files::text,'') files, coalesce(snapshot::text,'') snapshot, coalesce(error,'') error, coalesce(duration_ms::text,'') duration_ms, coalesce(created_at::text,'') created_at FROM portal_deploy_audit" audit_portaldeploy.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(action,'') action, coalesce(result,'') result, coalesce(metadata::text,'') metadata, coalesce(created_at::text,'') created_at FROM ruijie_cloud_audit" audit_ruijie.csv
export_csv "SELECT id::text, owner_id::text, coalesce(actor_user_id::text,'') actor_user_id, coalesce(channel,'') channel, coalesce(action,'') action, coalesce(outcome,'') outcome, coalesce(detail,'') detail, coalesce(created_at::text,'') created_at FROM notifier_audit" audit_notifier.csv
export_csv "SELECT a.id::text, coalesce(p.owner_id::text,'') owner_id, coalesce(a.actor_user_id::text,'') actor_user_id, coalesce(a.action,'') action, coalesce(a.note,'') note, coalesce(a.purchase_id::text,'') purchase_id, coalesce(a.created_at::text,'') created_at FROM service_purchase_audit a LEFT JOIN service_purchases p ON p.id = a.purchase_id" audit_spa.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_audit_routerops; CREATE TABLE _stg_audit_routerops (id text, owner_id text, user_id text, action text, router_id text, outcome text, environment text, detail text, error_message text, duration_ms text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_audit_routersave; CREATE TABLE _stg_audit_routersave (id text, owner_id text, user_id text, action text, router_id text, success text, error_message text, error_code text, attempted_name text, attempted_host text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_audit_portaldeploy; CREATE TABLE _stg_audit_portaldeploy (id text, owner_id text, user_id text, router_id text, ok text, files text, snapshot text, error text, duration_ms text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_audit_ruijie; CREATE TABLE _stg_audit_ruijie (id text, owner_id text, user_id text, action text, result text, metadata text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_audit_notifier; CREATE TABLE _stg_audit_notifier (id text, owner_id text, actor_user_id text, channel text, action text, outcome text, detail text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_audit_spa; CREATE TABLE _stg_audit_spa (id text, owner_id text, actor_user_id text, action text, note text, purchase_id text, created_at text);"
load _stg_audit_routerops   "id,owner_id,user_id,action,router_id,outcome,environment,detail,error_message,duration_ms,created_at" audit_routerops.csv
load _stg_audit_routersave  "id,owner_id,user_id,action,router_id,success,error_message,error_code,attempted_name,attempted_host,created_at" audit_routersave.csv
load _stg_audit_portaldeploy "id,owner_id,user_id,router_id,ok,files,snapshot,error,duration_ms,created_at" audit_portaldeploy.csv
load _stg_audit_ruijie      "id,owner_id,user_id,action,result,metadata,created_at" audit_ruijie.csv
load _stg_audit_notifier    "id,owner_id,actor_user_id,channel,action,outcome,detail,created_at" audit_notifier.csv
load _stg_audit_spa         "id,owner_id,actor_user_id,action,note,purchase_id,created_at" audit_spa.csv
sql "audit events" < "$S/sql/80_audit_events.sql"

step "2.3 purchases: service_purchases -> purchase_requests"
export_csv "SELECT id::text, owner_id::text, coalesce(service_key,'') service_key, coalesce(price_mmk::text,'') price_mmk, coalesce(status,'') status, coalesce(reference,'') reference, coalesce(receipt_object_key,'') receipt_object_key, coalesce(decided_at::text,'') decided_at, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at, coalesce(payment_method,'') payment_method FROM service_purchases" service_purchases.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_service_purchases; CREATE TABLE _stg_service_purchases (id text, owner_id text, service_key text, price_mmk text, status text, reference text, receipt_object_key text, decided_at text, created_at text, updated_at text, payment_method text);"
load _stg_service_purchases "id,owner_id,service_key,price_mmk,status,reference,receipt_object_key,decided_at,created_at,updated_at,payment_method" service_purchases.csv
sql "purchase requests" < "$S/sql/90_purchase_requests.sql"

step "2.4 Magic Coins: wallets + ledger -> ported tables"
export_csv "SELECT user_id::text, coalesce(balance::text,'0') balance, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM magic_coin_wallets" coin_wallets.csv
export_csv "SELECT id::text, user_id::text, owner_id::text, coalesce(delta::text,'') delta, coalesce(balance_after::text,'') balance_after, coalesce(kind,'') kind, coalesce(service_purchase_id::text,'') service_purchase_id, coalesce(note,'') note, coalesce(actor_user_id::text,'') actor_user_id, coalesce(created_at::text,'') created_at, coalesce(webfig_key_activation_id::text,'') webfig_key_activation_id, coalesce(reseller_add_key_activation_id::text,'') reseller_add_key_activation_id, coalesce(router_unlock_key_activation_id::text,'') router_unlock_key_activation_id, coalesce(magic_dude_unlock_activation_id::text,'') magic_dude_unlock_activation_id, coalesce(switch_unlock_key_activation_id::text,'') switch_unlock_key_activation_id FROM magic_coin_transactions" coin_tx.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_coin_wallets; CREATE TABLE _stg_coin_wallets (user_id text, balance text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_coin_tx; CREATE TABLE _stg_coin_tx (id text, user_id text, owner_id text, delta text, balance_after text, kind text, service_purchase_id text, note text, actor_user_id text, created_at text, webfig_key_activation_id text, reseller_add_key_activation_id text, router_unlock_key_activation_id text, magic_dude_unlock_activation_id text, switch_unlock_key_activation_id text);"
load _stg_coin_wallets "user_id,balance,created_at,updated_at" coin_wallets.csv
load _stg_coin_tx "id,user_id,owner_id,delta,balance_after,kind,service_purchase_id,note,actor_user_id,created_at,webfig_key_activation_id,reseller_add_key_activation_id,router_unlock_key_activation_id,magic_dude_unlock_activation_id,switch_unlock_key_activation_id" coin_tx.csv
sql "magic coins" < "$S/sql/100_magic_coins.sql"

step "2.5 unlock-key activations: 5 old tables -> 1 consolidated table"
export_csv "SELECT id::text, owner_id::text, user_id::text, 'webfig' key_kind, key_id::text, key_name, price_coins::text, attributes, account_bound::text, non_transferable::text, 'false' unique_router_key, '' grace_days, '' consumed_at, '' consumed_ref_id, created_at::text, expires_at::text FROM webfig_unlock_key_activations UNION ALL SELECT id::text, owner_id::text, user_id::text, 'magic_dude', '', '', price_coins::text, attributes, account_bound::text, non_transferable::text, 'false', '30', '', '', created_at::text, expires_at::text FROM magic_dude_unlock_activations UNION ALL SELECT id::text, owner_id::text, user_id::text, 'router', key_id::text, key_name, price_coins::text, attributes, account_bound::text, non_transferable::text, unique_router_key::text, '', coalesce(consumed_at::text,''), coalesce(consumed_router_id::text,''), created_at::text, expires_at::text FROM router_unlock_key_activations UNION ALL SELECT id::text, owner_id::text, user_id::text, 'switch', key_id::text, key_name, price_coins::text, attributes, account_bound::text, non_transferable::text, 'false', '', coalesce(consumed_at::text,''), coalesce(consumed_switch_id::text,''), created_at::text, expires_at::text FROM switch_unlock_key_activations UNION ALL SELECT id::text, owner_id::text, user_id::text, 'reseller', key_id::text, key_name, price_coins::text, attributes, account_bound::text, non_transferable::text, 'false', '', coalesce(consumed_at::text,''), coalesce(consumed_reseller_id::text,''), created_at::text, expires_at::text FROM reseller_add_key_activations" unlock_keys.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_unlock_keys; CREATE TABLE _stg_unlock_keys (id text, owner_id text, user_id text, key_kind text, key_id text, key_name text, price_coins text, attributes text, account_bound text, non_transferable text, unique_router_key text, grace_days text, consumed_at text, consumed_ref_id text, created_at text, expires_at text);"
load _stg_unlock_keys "id,owner_id,user_id,key_kind,key_id,key_name,price_coins,attributes,account_bound,non_transferable,unique_router_key,grace_days,consumed_at,consumed_ref_id,created_at,expires_at" unlock_keys.csv
sql "unlock keys" < "$S/sql/110_unlock_keys.sql"

step "2.6 entitlements + feature grants + role defaults"
export_csv "SELECT user_id::text, coalesce(owner_id::text,'') owner_id, coalesce(tier,'') tier, coalesce(tier_expires_at::text,'') tier_expires_at, coalesce(plus::text,'') plus, coalesce(plus_since::text,'') plus_since, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM account_entitlements" entitlements.csv
export_csv "SELECT user_id::text, coalesce(feature,'') feature, coalesce(granted_by::text,'') granted_by, coalesce(created_at::text,'') created_at FROM operator_feature_grants" feature_grants.csv
export_csv "SELECT role::text legacy_role, coalesce(feature,'') feature, coalesce(updated_by::text,'') updated_by, coalesce(updated_at::text,'') updated_at FROM operator_feature_role_defaults" role_defaults.csv
export_csv "SELECT role::text legacy_role, coalesce(mode,'') mode, coalesce(updated_by::text,'') updated_by, coalesce(updated_at::text,'') updated_at FROM portal_mode_role_defaults" portal_mode.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_entitlements; CREATE TABLE _stg_entitlements (user_id text, owner_id text, tier text, tier_expires_at text, plus text, plus_since text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_feature_grants; CREATE TABLE _stg_feature_grants (user_id text, feature text, granted_by text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_role_defaults; CREATE TABLE _stg_role_defaults (legacy_role text, feature text, updated_by text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_portal_mode; CREATE TABLE _stg_portal_mode (legacy_role text, mode text, updated_by text, updated_at text);"
load _stg_entitlements   "user_id,owner_id,tier,tier_expires_at,plus,plus_since,created_at,updated_at" entitlements.csv
load _stg_feature_grants "user_id,feature,granted_by,created_at" feature_grants.csv
load _stg_role_defaults  "legacy_role,feature,updated_by,updated_at" role_defaults.csv
load _stg_portal_mode    "legacy_role,mode,updated_by,updated_at" portal_mode.csv
sql "entitlements + features" < "$S/sql/120_entitlements_features.sql"

step "2.7 sites + router connections + topology (+ device links)"
export_csv "SELECT id::text, owner_id::text, coalesce(name,'') name, coalesce(location,'') location, coalesce(timezone,'') timezone, coalesce(latitude::text,'') latitude, coalesce(longitude::text,'') longitude, coalesce(notes,'') notes, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM sites" sites.csv
export_csv "SELECT id::text, owner_id::text, coalesce(name,'') name, coalesce(host,'') host, coalesce(port::text,'') port, coalesce(use_tls::text,'') use_tls, coalesce(is_default::text,'') is_default, coalesce(site_id::text,'') site_id, coalesce(connection_mode,'') connection_mode, coalesce(cloud_status,'') cloud_status, coalesce(cloud_last_seen_at::text,'') cloud_last_seen_at, coalesce(environment,'') environment, coalesce(password_ciphertext,'') password_ciphertext, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM router_connections" router_conn.csv
export_csv "SELECT site_id::text, router_id::text, coalesce(port_labels::text,'') port_labels, coalesce(downstream_devices::text,'') downstream_devices, coalesce(updated_at::text,'') updated_at FROM site_topology_config" topology.csv
export_csv "SELECT id::text, coalesce(site_id::text,'') site_id, coalesce(router_id::text,'') router_id FROM managed_devices" device_links.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_sites; CREATE TABLE _stg_sites (id text, owner_id text, name text, location text, timezone text, latitude text, longitude text, notes text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_router_conn; CREATE TABLE _stg_router_conn (id text, owner_id text, name text, host text, port text, use_tls text, is_default text, site_id text, connection_mode text, cloud_status text, cloud_last_seen_at text, environment text, password_ciphertext text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_topology; CREATE TABLE _stg_topology (site_id text, router_id text, port_labels text, downstream_devices text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_device_links; CREATE TABLE _stg_device_links (device_id text, site_id text, router_id text);"
load _stg_sites        "id,owner_id,name,location,timezone,latitude,longitude,notes,created_at,updated_at" sites.csv
load _stg_router_conn  "id,owner_id,name,host,port,use_tls,is_default,site_id,connection_mode,cloud_status,cloud_last_seen_at,environment,password_ciphertext,created_at,updated_at" router_conn.csv
load _stg_topology     "site_id,router_id,port_labels,downstream_devices,updated_at" topology.csv
load _stg_device_links "device_id,site_id,router_id" device_links.csv
sql "sites + topology" < "$S/sql/130_sites_topology.sql"

step "2.8 terminal + resellers + print + promo + ruijie + Magic Dude KB"
export_csv "SELECT id::text, owner_id::text, coalesce(created_by::text,'') created_by, coalesce(name,'') name, coalesce(description,'') description, coalesce(category,'') category, coalesce(method,'') method, coalesce(path,'') path, coalesce(body,'') body, coalesce(is_builtin::text,'') is_builtin, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM terminal_templates" term_templates.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(router_id::text,'') router_id, coalesce(site_id::text,'') site_id, coalesce(method,'') method, coalesce(path,'') path, coalesce(body,'') body, coalesce(status::text,'') status, coalesce(ms::text,'') ms, coalesce(response_snippet,'') response_snippet, coalesce(created_at::text,'') created_at FROM terminal_history" term_history.csv
export_csv "SELECT id::text, owner_id::text, coalesce(shop_name,'') shop_name, coalesce(contact_name,'') contact_name, coalesce(location,'') location, coalesce(phone,'') phone, coalesce(active::text,'') active, coalesce(code_prefix,'') code_prefix, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM voucher_resellers" resellers.csv
export_csv "SELECT owner_id::text, coalesce(business_name,'') business_name, coalesce(wifi_name,'') wifi_name, coalesce(support_contact,'') support_contact, coalesce(terms,'') terms, coalesce(paper_width_mm::text,'') paper_width_mm, coalesce(show_qr::text,'') show_qr, coalesce(show_price::text,'') show_price, coalesce(show_expiry::text,'') show_expiry, coalesce(a4_columns::text,'') a4_columns, coalesce(a4_rows_per_page::text,'') a4_rows_per_page, coalesce(thermal_columns::text,'') thermal_columns, coalesce(thermal_rows_per_strip::text,'') thermal_rows_per_strip, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM voucher_print_layouts" print_layouts.csv
export_csv "SELECT coalesce(active::text,'') active, coalesce(starts_on::text,'') starts_on, coalesce(ends_on::text,'') ends_on, coalesce(label,'') label, coalesce(monthly_promo_mmk::text,'') monthly_promo_mmk, coalesce(monthly_standard_mmk::text,'') monthly_standard_mmk, coalesce(annual_promo_mmk::text,'') annual_promo_mmk, coalesce(annual_standard_mmk::text,'') annual_standard_mmk, coalesce(updated_at::text,'') updated_at FROM pricing_promo" promo.csv
export_csv "SELECT id::text, owner_id::text, coalesce(region,'') region, coalesce(base_url,'') base_url, coalesce(app_id_ciphertext,'') app_id_ciphertext, coalesce(app_secret_ciphertext,'') app_secret_ciphertext, coalesce(site_ref,'') site_ref, coalesce(device_ref,'') device_ref, coalesce(enabled::text,'') enabled, coalesce(last_test_at::text,'') last_test_at, coalesce(last_test_status,'') last_test_status, coalesce(last_sync_at::text,'') last_sync_at, coalesce(created_by::text,'') created_by, coalesce(token_path,'') token_path, coalesce(device_path,'') device_path, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM ruijie_cloud_connections" ruijie.csv
export_csv "SELECT id::text, coalesce(title,'') title, coalesce(body_md,'') body_md, coalesce(sort::text,'') sort, coalesce(active::text,'') active, coalesce(updated_by::text,'') updated_by, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM magic_dude_knowledge_notes" kb_notes.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(question,'') question, coalesce(answer,'') answer, coalesce(language,'') language, coalesce(helpful_count::text,'') helpful_count, coalesce(created_at::text,'') created_at, coalesce(last_used_at::text,'') last_used_at FROM magic_dude_solved_cases" solved.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_term_templates; CREATE TABLE _stg_term_templates (id text, owner_id text, created_by text, name text, description text, category text, method text, path text, body text, is_builtin text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_term_history; CREATE TABLE _stg_term_history (id text, owner_id text, user_id text, router_id text, site_id text, method text, path text, body text, status text, ms text, response_snippet text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_resellers; CREATE TABLE _stg_resellers (id text, owner_id text, shop_name text, contact_name text, location text, phone text, active text, code_prefix text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_print_layouts; CREATE TABLE _stg_print_layouts (owner_id text, business_name text, wifi_name text, support_contact text, terms text, paper_width_mm text, show_qr text, show_price text, show_expiry text, a4_columns text, a4_rows_per_page text, thermal_columns text, thermal_rows_per_strip text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_promo; CREATE TABLE _stg_promo (active text, starts_on text, ends_on text, label text, monthly_promo_mmk text, monthly_standard_mmk text, annual_promo_mmk text, annual_standard_mmk text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_ruijie; CREATE TABLE _stg_ruijie (id text, owner_id text, region text, base_url text, app_id_ciphertext text, app_secret_ciphertext text, site_ref text, device_ref text, enabled text, last_test_at text, last_test_status text, last_sync_at text, created_by text, token_path text, device_path text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_kb_notes; CREATE TABLE _stg_kb_notes (id text, title text, body_md text, sort text, active text, updated_by text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_solved; CREATE TABLE _stg_solved (id text, owner_id text, user_id text, question text, answer text, language text, helpful_count text, created_at text, last_used_at text);"
load _stg_term_templates "id,owner_id,created_by,name,description,category,method,path,body,is_builtin,created_at,updated_at" term_templates.csv
load _stg_term_history   "id,owner_id,user_id,router_id,site_id,method,path,body,status,ms,response_snippet,created_at" term_history.csv
load _stg_resellers      "id,owner_id,shop_name,contact_name,location,phone,active,code_prefix,created_at,updated_at" resellers.csv
load _stg_print_layouts  "owner_id,business_name,wifi_name,support_contact,terms,paper_width_mm,show_qr,show_price,show_expiry,a4_columns,a4_rows_per_page,thermal_columns,thermal_rows_per_strip,created_at,updated_at" print_layouts.csv
load _stg_promo          "active,starts_on,ends_on,label,monthly_promo_mmk,monthly_standard_mmk,annual_promo_mmk,annual_standard_mmk,updated_at" promo.csv
load _stg_ruijie         "id,owner_id,region,base_url,app_id_ciphertext,app_secret_ciphertext,site_ref,device_ref,enabled,last_test_at,last_test_status,last_sync_at,created_by,token_path,device_path,created_at,updated_at" ruijie.csv
load _stg_kb_notes       "id,title,body_md,sort,active,updated_by,created_at,updated_at" kb_notes.csv
load _stg_solved         "id,owner_id,user_id,question,answer,language,helpful_count,created_at,last_used_at" solved.csv
sql "terminal + resellers" < "$S/sql/140_terminals_resellers.sql"

step "2.9 platform-owned content from the excluded dev account (built-in template library)"
sql "platform owned" < "$S/sql/145_platform_owned.sql"

step "2.10 i18n + daily closes + allowances/requests + notif prefs + platform config"
export_csv "SELECT id::text, locale, ns, key, value, coalesce(updated_by::text,'') updated_by, coalesce(updated_at::text,'') updated_at FROM i18n_overrides" i18n.csv
export_csv "SELECT id::text, owner_id::text, coalesce(site_id::text,'') site_id, coalesce(router_id::text,'') router_id, business_day::text, counted_cash_mmk::text, coalesce(note,'') note, closed_by::text, closed_at::text FROM owner_daily_closes" daily_closes.csv
export_csv "SELECT owner_id::text, routers::text, controllers::text, sites::text, created_at::text, updated_at::text, coalesce(included_router_id::text,'') included_router_id FROM device_allowances" allowances.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(kind,'') kind, coalesce(reason,'') reason, coalesce(status,'') status, coalesce(decided_by::text,'') decided_by, coalesce(decided_at::text,'') decided_at, coalesce(created_at::text,'') created_at, coalesce(updated_at::text,'') updated_at FROM device_requests" dev_requests.csv
export_csv "SELECT owner_id::text, in_app::text, email::text, coalesce(quiet_hours_start::text,'') quiet_hours_start, coalesce(quiet_hours_end::text,'') quiet_hours_end, min_severity, created_at::text, updated_at::text FROM notification_prefs" notif_prefs.csv
export_csv "SELECT active::text, coalesce(message,'') message, coalesce(speed_seconds::text,'') speed_seconds, coalesce(updated_at::text,'') updated_at, coalesce(updated_by::text,'') updated_by FROM platform_announcement" announcement.csv
export_csv "SELECT user_id::text, coalesce(note,'') note, created_at::text FROM platform_admins" platform_admins.csv
export_csv "SELECT id::text, slot::text, holder_name, bank_name, account_number, enabled::text, sort::text, coalesce(configured_by::text,'') configured_by, created_at::text, updated_at::text FROM platform_service_bank_accounts" bank_accounts.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_i18n; CREATE TABLE _stg_i18n (id text, locale text, ns text, key text, value text, updated_by text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_daily_closes; CREATE TABLE _stg_daily_closes (id text, owner_id text, site_id text, router_id text, business_day text, counted_cash_mmk text, note text, closed_by text, closed_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_allowances; CREATE TABLE _stg_allowances (owner_id text, routers text, controllers text, sites text, created_at text, updated_at text, included_router_id text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_dev_requests; CREATE TABLE _stg_dev_requests (id text, owner_id text, user_id text, kind text, reason text, status text, decided_by text, decided_at text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_notif_prefs; CREATE TABLE _stg_notif_prefs (owner_id text, in_app text, email text, quiet_hours_start text, quiet_hours_end text, min_severity text, created_at text, updated_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_announcement; CREATE TABLE _stg_announcement (active text, message text, speed_seconds text, updated_at text, updated_by text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_platform_admins; CREATE TABLE _stg_platform_admins (user_id text, note text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_bank_accounts; CREATE TABLE _stg_bank_accounts (id text, slot text, holder_name text, bank_name text, account_number text, enabled text, sort text, configured_by text, created_at text, updated_at text);"
load _stg_i18n            "id,locale,ns,key,value,updated_by,updated_at" i18n.csv
load _stg_daily_closes    "id,owner_id,site_id,router_id,business_day,counted_cash_mmk,note,closed_by,closed_at" daily_closes.csv
load _stg_allowances      "owner_id,routers,controllers,sites,created_at,updated_at,included_router_id" allowances.csv
load _stg_dev_requests    "id,owner_id,user_id,kind,reason,status,decided_by,decided_at,created_at,updated_at" dev_requests.csv
load _stg_notif_prefs     "owner_id,in_app,email,quiet_hours_start,quiet_hours_end,min_severity,created_at,updated_at" notif_prefs.csv
load _stg_announcement    "active,message,speed_seconds,updated_at,updated_by" announcement.csv
load _stg_platform_admins "user_id,note,created_at" platform_admins.csv
load _stg_bank_accounts   "id,slot,holder_name,bank_name,account_number,enabled,sort,configured_by,created_at,updated_at" bank_accounts.csv
sql "platform config" < "$S/sql/150_platform_config.sql"
$DST -tAc "select 'i18n_overrides='||count(*) from i18n_overrides"
$DST -tAc "select 'owner_daily_closes='||count(*) from owner_daily_closes"
$DST -tAc "select 'device_allowances='||count(*) from device_allowances"
$DST -tAc "select 'device_requests='||count(*) from device_requests"
$DST -tAc "select 'notification_prefs='||count(*) from notification_prefs"
$DST -tAc "select 'platform_announcement='||count(*) from platform_announcement"
$DST -tAc "select 'platform_admins='||count(*) from platform_admins"
$DST -tAc "select 'platform_service_bank_accounts='||count(*) from platform_service_bank_accounts"

step "2.11 ops history — syslog + scan runs + AI usage + admin notifications"
export_csv "SELECT id::text, owner_id::text, coalesce(router_id::text,'') router_id, coalesce(site_id::text,'') site_id, coalesce(source_ip,'') source_ip, coalesce(facility,'') facility, coalesce(severity,'') severity, coalesce(program,'') program, coalesce(message,'') message, coalesce(ai_summary,'') ai_summary, coalesce(ai_severity,'') ai_severity, coalesce(received_at::text,'') received_at FROM syslog_events" syslog.csv
export_csv "SELECT id::text, owner_id::text, coalesce(kind,'') kind, coalesce(payload::text,'') payload, coalesce(max_severity,'') max_severity, coalesce(router_count::text,'') router_count, coalesce(generated_at::text,'') generated_at, coalesce(triggered_by::text,'') triggered_by FROM fleet_scan_runs" scan_runs.csv
export_csv "SELECT id::text, owner_id::text, coalesce(user_id::text,'') user_id, coalesce(feature,'') feature, coalesce(model,'') model, coalesce(prompt_tokens::text,'') prompt_tokens, coalesce(completion_tokens::text,'') completion_tokens, coalesce(total_tokens::text,'') total_tokens, coalesce(cached::text,'') cached, coalesce(created_at::text,'') created_at FROM ai_usage_events" ai_usage.csv
export_csv "SELECT id::text, recipient_id::text, coalesce(kind,'') kind, coalesce(title,'') title, coalesce(body,'') body, coalesce(data::text,'') data, coalesce(read_at::text,'') read_at, coalesce(created_at::text,'') created_at FROM admin_notifications" admin_notifs.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_syslog; CREATE TABLE _stg_syslog (id text, owner_id text, router_id text, site_id text, source_ip text, facility text, severity text, program text, message text, ai_summary text, ai_severity text, received_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_scan_runs; CREATE TABLE _stg_scan_runs (id text, owner_id text, kind text, payload text, max_severity text, router_count text, generated_at text, triggered_by text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_ai_usage; CREATE TABLE _stg_ai_usage (id text, owner_id text, user_id text, feature text, model text, prompt_tokens text, completion_tokens text, total_tokens text, cached text, created_at text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_admin_notifs; CREATE TABLE _stg_admin_notifs (id text, recipient_id text, kind text, title text, body text, data text, read_at text, created_at text);"
load _stg_syslog       "id,owner_id,router_id,site_id,source_ip,facility,severity,program,message,ai_summary,ai_severity,received_at" syslog.csv
load _stg_scan_runs    "id,owner_id,kind,payload,max_severity,router_count,generated_at,triggered_by" scan_runs.csv
load _stg_ai_usage     "id,owner_id,user_id,feature,model,prompt_tokens,completion_tokens,total_tokens,cached,created_at" ai_usage.csv
load _stg_admin_notifs "id,recipient_id,kind,title,body,data,read_at,created_at" admin_notifs.csv
sql "ops history" < "$S/sql/160_ops_history.sql"
$DST -tAc "select 'syslog_events='||count(*) from syslog_events"
$DST -tAc "select 'fleet_scan_runs='||count(*) from fleet_scan_runs"
$DST -tAc "select 'ai_usage_events='||count(*) from ai_usage_events"
$DST -tAc "select 'admin_notifications='||count(*) from admin_notifications"

step "3.0 cleanup — remove baseline demo/seed residue (source-only enforcement)"
export_csv "SELECT id::text FROM managed_devices" src_device_ids.csv
export_csv "SELECT owner_id::text FROM portal_settings" src_portal_owners.csv
$DST -q -c "DROP TABLE IF EXISTS _stg_src_device_ids; CREATE TABLE _stg_src_device_ids (id text);"
$DST -q -c "DROP TABLE IF EXISTS _stg_src_portal_owners; CREATE TABLE _stg_src_portal_owners (owner_id text);"
load _stg_src_device_ids  "id" src_device_ids.csv
load _stg_src_portal_owners "owner_id" src_portal_owners.csv
sql "cleanup residue" < "$S/sql/200_cleanup_baseline_residue.sql"
$DST -tAc "select 'devices after cleanup='||count(*) from devices"
$DST -tAc "select 'portal_configs after cleanup='||count(*) from portal_configs"

step "RECONCILIATION"
$DST -tAc "select 'businesses='||count(*) from businesses"
$DST -tAc "select 'users='||count(*) from users"
$DST -tAc "select 'devices='||count(*) from devices"
$DST -tAc "select 'vouchers='||count(*) from vouchers"
$DST -tAc "select 'portal_configs='||count(*) from portal_configs"
$DST -tAc "select 'incidents='||count(*) from incidents"
$DST -tAc "select 'alert_policies='||count(*) from alert_policies"
$DST -tAc "select 'telemetry_samples='||count(*) from telemetry_samples"
$DST -tAc "select 'audit_events='||count(*) from audit_events"
$DST -tAc "select 'purchase_requests='||count(*) from purchase_requests"
$DST -tAc "select 'magic_coin_wallets='||count(*) from magic_coin_wallets"
$DST -tAc "select 'magic_coin_transactions='||count(*) from magic_coin_transactions"
$DST -tAc "select 'coin balance total='||coalesce(sum(balance),0) from magic_coin_wallets"
$DST -tAc "select 'unlock_key_activations='||count(*) from unlock_key_activations"
$DST -tAc "select 'account_entitlements='||count(*) from account_entitlements"
$DST -tAc "select 'operator_feature_grants='||count(*) from operator_feature_grants"
$DST -tAc "select 'operator_feature_role_defaults='||count(*) from operator_feature_role_defaults"
$DST -tAc "select 'portal_mode_role_defaults='||count(*) from portal_mode_role_defaults"
$DST -tAc "select 'sites='||count(*) from sites"
$DST -tAc "select 'router_connections='||count(*) from router_connections"
$DST -tAc "select 'site_topology_config='||count(*) from site_topology_config"
$DST -tAc "select 'devices with site link='||count(*) from devices where site_id is not null"
$DST -tAc "select 'devices with connection link='||count(*) from devices where router_connection_id is not null"
$DST -tAc "select 'terminal_templates='||count(*) from terminal_templates"
$DST -tAc "select 'terminal_history='||count(*) from terminal_history"
$DST -tAc "select 'voucher_resellers='||count(*) from voucher_resellers"
$DST -tAc "select 'voucher_print_layouts='||count(*) from voucher_print_layouts"
$DST -tAc "select 'pricing_promo='||count(*) from pricing_promo"
$DST -tAc "select 'ruijie_cloud_connections='||count(*) from ruijie_cloud_connections"
$DST -tAc "select 'magic_dude_knowledge_notes='||count(*) from magic_dude_knowledge_notes"
$DST -tAc "select 'magic_dude_solved_cases='||count(*) from magic_dude_solved_cases"

step "INTEGRITY (all must be 0)"
$DST -tAc "select 'orphan business_id rows='||(select count(*) from devices d left join businesses b using(business_id) where b.business_id is null)+(select count(*) from vouchers v left join businesses b using(business_id) where b.business_id is null)+(select count(*) from incidents i left join businesses b using(business_id) where b.business_id is null)+(select count(*) from portal_configs pc left join businesses b using(business_id) where b.business_id is null)+(select count(*) from alert_policies a left join businesses b using(business_id) where b.business_id is null)"
$DST -tAc "select 'orphan device refs='||count(*) from incidents i left join devices d using(device_id) where i.device_id is not null and d.device_id is null"
$DST -tAc "select 'users without business='||count(*) from users u left join businesses b using(business_id) where b.business_id is null"

step "PHASE 3 — independent reconciliation (id-set fingerprints vs source)"
DST_DB="$TARGET_DB" "$S/verify_phase3.sh" || { echo "phase 3 verification FAILED" >&2; exit 1; }

echo; echo "done — target: $TARGET_DB (baseline $BASELINE_DB untouched)"
