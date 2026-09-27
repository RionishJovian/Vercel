#!/usr/bin/env bash
#
# Phase 3 — INDEPENDENT reconciliation.
#
# WHY A SEPARATE VERIFIER
# The runner's own reconciliation counts what the runner just inserted, so a
# shared bug looks like success. This script checks the merged copy by a
# DIFFERENT method:
#   * fingerprint = md5 of the sorted id set, not a row count — a count alone
#     cannot distinguish "same 642 rows" from "642 different rows";
#   * the migrated-owner list is derived INDEPENDENTLY from source auth.users,
#     never from the ETL's own _mig_accounts staging table.
#
# Two classes of table:
#   EXACT   — every source row belongs to a migrated owner; the id sets must match
#             the FULL source table.
#   FILTER  — source rows exist for owners who never migrated (their rows are
#             legitimately excluded); the id sets must match source RESTRICTED to
#             migrated owners.
set -uo pipefail

SRC_CONTAINER="${SRC_CONTAINER:-supabase-db}"
SRC_DB="${SRC_DB:-postgres}"
SRC_USER="${SRC_USER:-postgres}"
DST_CONTAINER="${DST_CONTAINER:-mega_magic_db_dev}"
DST_DB="${DST_DB:-mega_merge}"
DST_USER="${DST_USER:-mega_admin}"

S="docker exec $SRC_CONTAINER psql -U $SRC_USER -d $SRC_DB -tAc"
D="docker exec $DST_CONTAINER psql -U $DST_USER -d $DST_DB -tAc"

# Migrated owners, derived from source independently of the ETL.
OWN="owner_id in (select id from auth.users where email not in ('test@mikromagic','nish@dev'))"

# label|class|dst_table|dst_pk|src_table|src_pk
TABLES="
vouchers|EXACT|vouchers|id|voucher_codes|id
devices|FILTER|devices|device_id|managed_devices|id
incidents|EXACT|incidents|id|incidents|id
portal_configs|FILTER|portal_configs|business_id|portal_settings|owner_id
coin_tx|EXACT|magic_coin_transactions|id|magic_coin_transactions|id
term_templates|EXACT|terminal_templates|id|terminal_templates|id
term_history|FILTER|terminal_history|id|terminal_history|id
syslog|EXACT|syslog_events|id|syslog_events|id
scan_runs|FILTER|fleet_scan_runs|id|fleet_scan_runs|id
ai_usage|FILTER|ai_usage_events|id|ai_usage_events|id
daily_closes|EXACT|owner_daily_closes|id|owner_daily_closes|id
i18n|EXACT|i18n_overrides|id|i18n_overrides|id
telemetry|EXACT|telemetry_samples|id|device_health_samples|id
sites|EXACT|sites|site_id|sites|id
router_conn|EXACT|router_connections|id|router_connections|id
resellers|EXACT|voucher_resellers|id|voucher_resellers|id
bank_accounts|EXACT|platform_service_bank_accounts|id|platform_service_bank_accounts|id
"

src_parts=""; dst_parts=""
while IFS='|' read -r label cls dt dpk st spk; do
  [ -z "$label" ] && continue
  if [ "$cls" = "FILTER" ]; then
    # the source column to filter on is the source FK; portal_settings uses owner_id,
    # everything else in this class carries owner_id too
    src_parts="$src_parts select '$label' t, (select count(*) from $st where $OWN) n, coalesce(md5((select string_agg($spk::text, ',' order by $spk::text) from $st where $OWN)),'-') h union all"
  else
    src_parts="$src_parts select '$label' t, (select count(*) from $st) n, coalesce(md5((select string_agg($spk::text, ',' order by $spk::text) from $st)),'-') h union all"
  fi
  dst_parts="$dst_parts select '$label' t, count(*) n, coalesce(md5(string_agg($dpk::text, ',' order by $dpk::text)),'-') h from $dt union all"
done <<< "$TABLES"

# strip the trailing "union all" and keep it on ONE line (a literal newline breaks psql)
src_sql="${src_parts%union all}"; src_sql="${src_sql//$'\n'/ }"
dst_sql="${dst_parts%union all}"; dst_sql="${dst_sql//$'\n'/ }"

$S "$src_sql" > /tmp/.p3src 2>/tmp/.p3srcerr || { echo "FAILED: source query"; cat /tmp/.p3srcerr; exit 1; }
$D "$dst_sql" > /tmp/.p3dst 2>/tmp/.p3dsterr || { echo "FAILED: target query"; cat /tmp/.p3dsterr; exit 1; }

printf "%-16s %-7s %8s %8s  %-18s %s\n" table class src dst "id-set md5" verdict
printf '%.0s-' {1..72}; echo
fail=0; total=0
while IFS='|' read -r label cls dt dpk st spk; do
  [ -z "$label" ] && continue
  total=$((total+1))
  s_line=$(grep "^$label|" /tmp/.p3src); d_line=$(grep "^$label|" /tmp/.p3dst)
  s_n=${s_line#*|}; s_n=${s_n%%|*}; s_h=${s_line##*|}
  d_n=${d_line#*|}; d_n=${d_n%%|*}; d_h=${d_line##*|}
  if [ -n "$s_n" ] && [ "$s_n" = "$d_n" ] && [ "$s_h" = "$d_h" ]; then
    verdict="PASS"
  else
    verdict="FAIL"; fail=$((fail+1))
  fi
  printf "%-16s %-7s %8s %8s  %-18s %s\n" "$label" "$cls" "$s_n" "$d_n" "${d_h:0:16}" "$verdict"
done <<< "$TABLES"

echo
if [ "$fail" -eq 0 ]; then
  echo "PHASE 3 OK — $total/$total tables match the source by id-set fingerprint"
  exit 0
fi
echo "PHASE 3 FAILED — $fail of $total tables disagree with the source"
exit 1
