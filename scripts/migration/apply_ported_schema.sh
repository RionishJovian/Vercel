#!/usr/bin/env bash
#
# Apply the Phase 2 ported schema (the `-- PHASE 2 — ported Mikromagic features`
# section of database/init.sql) to the BASELINE database.
#
# Why this exists: the runner rebuilds the working copy from the baseline every
# run, so new tables must exist in the BASELINE or the whole migration misses
# them. init.sql is the single source of truth — this script only extracts and
# applies, so the baseline can never drift from what a fresh install gets.
#
# Why it drops first: the ported section ends with CREATE POLICY statements,
# which are not idempotent. ALTER TABLE devices ADD COLUMN IF NOT EXISTS is also
# a trap — on a second run the column already exists, so IF NOT EXISTS skips it
# and the FOREIGN KEY is never created. Dropping the columns first avoids that.
set -uo pipefail

S="$(cd "$(dirname "$0")" && pwd)"
INIT_SQL="${INIT_SQL:-$S/../../database/init.sql}"
DST_CONTAINER="${DST_CONTAINER:-mega_magic_db_dev}"
DST_USER="${DST_USER:-mega_admin}"
BASELINE_DB="${BASELINE_DB:-mega_magic_dev}"

PSQL="docker exec $DST_CONTAINER psql -U $DST_USER -d $BASELINE_DB"

PORTED_TABLES="magic_coin_wallets magic_coin_transactions unlock_key_activations \
account_entitlements operator_feature_grants operator_feature_role_defaults \
portal_mode_role_defaults sites router_connections site_topology_config \
terminal_templates terminal_history voucher_resellers voucher_print_layouts \
pricing_promo ruijie_cloud_connections magic_dude_knowledge_notes magic_dude_solved_cases \
i18n_overrides owner_daily_closes device_allowances device_requests \
notification_prefs platform_announcement platform_admins platform_service_bank_accounts \
syslog_events fleet_scan_runs ai_usage_events admin_notifications"

[ -f "$INIT_SQL" ] || { echo "FAILED: init.sql not found at $INIT_SQL" >&2; exit 1; }

echo "1/3 dropping ported objects from $BASELINE_DB"
# devices columns must go first so their FKs are recreated by the ALTER below
$PSQL -q -c "ALTER TABLE devices DROP COLUMN IF EXISTS site_id, DROP COLUMN IF EXISTS router_connection_id;" || exit 1
# DROP TABLE takes a comma-separated list, not a space-separated one
$PSQL -q -c "DROP TABLE IF EXISTS $(echo $PORTED_TABLES | tr ' ' ',') CASCADE;" || exit 1

echo "2/3 extracting the ported section from init.sql"
sed -n '/^-- PHASE 2 — ported Mikromagic features/,$p' "$INIT_SQL" > "$S/.ported.sql"
[ -s "$S/.ported.sql" ] || { echo "FAILED: section not found in init.sql" >&2; exit 1; }
echo "    $(grep -c . "$S/.ported.sql") non-blank lines"

echo "3/3 applying to $BASELINE_DB"
docker exec -i "$DST_CONTAINER" psql -U "$DST_USER" -d "$BASELINE_DB" -q -v ON_ERROR_STOP=1 \
  < "$S/.ported.sql" 2>&1 | grep -vE '^NOTICE' || true
rc=${PIPESTATUS[0]}
[ "$rc" -eq 0 ] || { echo "FAILED: apply exited $rc" >&2; exit 1; }

echo "ok — ported tables now in $BASELINE_DB:"
$PSQL -tAc "select string_agg(tablename, ', ' order by tablename) from pg_tables
            where schemaname='public' and tablename = any(string_to_array('$(echo $PORTED_TABLES)', ' '))"
