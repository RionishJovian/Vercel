#!/usr/bin/env bash
# Tests mm-migrate against a real PostgreSQL:   MM_TEST_DB_URL=postgres://user:pass@host:port/postgres bash deploy/server/test-mm-migrate.sh
# Needs a role that can create databases (and roles, for the real runtime-role script). Skips without MM_TEST_DB_URL.
set -uo pipefail
if [ -z "${MM_TEST_DB_URL:-}" ]; then echo "skipped: set MM_TEST_DB_URL to an admin connection URL (a throwaway server)"; exit 0; fi
here=$(cd "$(dirname "$0")" && pwd)
tool="$here/mm-migrate"
repo=$(cd "$here/../.." && pwd)
base_url=${MM_TEST_DB_URL%/*}
work=$(mktemp -d); trap 'cleanup' EXIT
cleanup() { for d in $(cat "$work/dbs.list" 2>/dev/null); do psql "$MM_TEST_DB_URL" -X -q -c "drop database if exists \"$d\" with (force)" >/dev/null 2>&1; done; rm -rf "$work"; }
pass=0; failn=0
check() { if [ "$2" = "$3" ]; then pass=$((pass + 1)); else failn=$((failn + 1)); echo "FAIL: $1 (got '$2', want '$3')"; fi; }
newdb() { local n="mmmig_$$_$RANDOM"; echo "$n" >>"$work/dbs.list"; psql "$MM_TEST_DB_URL" -X -q -c "create database \"$n\"" >/dev/null; echo "$n"; }
q() { psql "$base_url/$1" -X -q -At -c "$2" 2>/dev/null; }

# fixture: a tiny baseline and a migrations directory we control
fx="$work/db"; mkdir -p "$fx/migrations"
cat >"$fx/baseline.sql" <<'SQL'
-- mm:baseline-version 20260101000000
create table accounts (id int primary key, name text);
alter table accounts enable row level security;
SQL
mig() { printf '%s\n' "$2" >"$fx/migrations/$1"; }
tool_in() { # db, args... ; fixture directory, no post hook unless MM_POST_SQL given
  local d="$1"; shift
  MM_DB_URL="$base_url/$d" MM_DB_DIR="$fx" MM_POST_SQL="${POST:-}" bash "$tool" "$@" 2>&1
}
code() { tool_in "$@" >/dev/null; echo $?; }
tables() { q "$1" "select string_agg(table_name,',' order by table_name) from information_schema.tables where table_schema='public' and table_name<>'schema_migrations'"; }

# --- not initialised
d=$(newdb)
check "status on a database without bookkeeping is a problem" "$(code "$d" status)" "3"
check "up on a database without bookkeeping refuses" "$(code "$d" up)" "3"

# --- init
check "init on an empty database works" "$(code "$d" init)" "0"
check "init creates the baseline tables" "$(tables "$d")" "accounts"
check "init records the baseline" "$(q "$d" "select kind||':'||version from schema_migrations")" "baseline:20260101000000"
check "status is up to date right after init" "$(code "$d" status)" "0"
check "init on a database that is not empty refuses" "$(code "$d" init)" "3"

# --- apply, in order, once
mig 20260102000000_create_a.sql "create table a (id int primary key);"
mig 20260103000000_create_b.sql "create table b (id int primary key); insert into b values (1);"
check "status shows two pending (exit 2)" "$(code "$d" status)" "2"
check "up applies them" "$(code "$d" up)" "0"
check "both tables exist" "$(tables "$d")" "a,accounts,b"
check "both are recorded with their checksum" "$(q "$d" "select count(*) from schema_migrations where kind='migration' and length(checksum)=64")" "2"
check "status is up to date" "$(code "$d" status)" "0"
check "a second up changes nothing" "$(tool_in "$d" up | grep -c 'nothing to apply')" "1"
check "…and does not re-run the data change" "$(q "$d" "select count(*) from b")" "1"

# --- a failing migration is rolled back whole and stops the rest
mig 20260104000000_bad.sql "create table c (id int); select * from does_not_exist;"
mig 20260105000000_after.sql "create table d (id int);"
check "a failing migration makes up fail" "$([ "$(code "$d" up)" != 0 ] && echo failed)" "failed"
check "its earlier statements were rolled back (no table c)" "$(tables "$d")" "a,accounts,b"
check "later migrations were not applied" "$(q "$d" "select count(*) from schema_migrations where filename like '%after%'")" "0"
check "nothing was recorded for the failed one" "$(q "$d" "select count(*) from schema_migrations where filename like '%bad%'")" "0"
mig 20260104000000_bad.sql "create table c (id int);"
check "after fixing the unapplied file, up succeeds" "$(code "$d" up)" "0"
check "all four tables now exist" "$(tables "$d")" "a,accounts,b,c,d"

# --- tampering and missing files
mig 20260102000000_create_a.sql "create table a (id int primary key); -- edited later"
check "an edited applied file is a problem" "$(code "$d" status)" "3"
check "…and up refuses to run anything" "$(code "$d" up)" "3"
mig 20260102000000_create_a.sql "create table a (id int primary key);"
check "restoring the file clears it" "$(code "$d" status)" "0"
rm "$fx/migrations/20260103000000_create_b.sql"
check "an applied migration whose file is gone is a problem" "$(code "$d" status)" "3"
mig 20260103000000_create_b.sql "create table b (id int primary key); insert into b values (1);"

# --- ordering and naming rules
mig 20260103500000_between.sql "create table between_t (id int);"
check "an unapplied migration older than an applied one is out of order (exit 2)" "$(code "$d" status)" "2"
check "up refuses out-of-order" "$(code "$d" up)" "3"
check "…and applied nothing" "$(tables "$d" | grep -c between_t)" "0"
check "--allow-out-of-order applies it" "$(code "$d" up --allow-out-of-order)" "0"
mig not_a_migration.sql "select 1;"
check "a badly named file is refused" "$(code "$d" status)" "3"
rm "$fx/migrations/not_a_migration.sql"
mig 20251231000000_too_old.sql "select 1;"
check "a migration not newer than the baseline is refused" "$(code "$d" status)" "3"
rm "$fx/migrations/20251231000000_too_old.sql"
mig 20260105000000_duplicate_version.sql "select 1;"
check "two files with the same version are refused" "$(code "$d" status)" "3"
rm "$fx/migrations/20260105000000_duplicate_version.sql"
mig 20260106000000_has_txn.sql $'begin;\ncreate table e (id int);\ncommit;'
check "a file with its own BEGIN/COMMIT is refused" "$(code "$d" status)" "3"
rm "$fx/migrations/20260106000000_has_txn.sql"
check "after removing the bad files everything is clean" "$(code "$d" status)" "0"

# --- no-transaction marker
mig 20260107000000_concurrent_index.sql $'-- mm:no-transaction\ncreate index concurrently a_id_idx on a (id);'
check "a no-transaction migration applies (CREATE INDEX CONCURRENTLY)" "$(code "$d" up)" "0"
check "…and the index exists" "$(q "$d" "select count(*) from pg_indexes where indexname='a_id_idx'")" "1"

# --- post hook runs only when something was applied
cat >"$work/post.sql" <<'SQL'
create table if not exists post_runs (n serial);
insert into post_runs default values;
SQL
POST="$work/post.sql"
mig 20260108000000_x.sql "create table x (id int);"
check "up with a post hook succeeds" "$(code "$d" up)" "0"
check "the post hook ran after applying" "$(q "$d" "select count(*) from post_runs")" "1"
check "nothing to apply: the post hook does not run" "$(code "$d" up)$(q "$d" "select count(*) from post_runs")" "01"
POST=""

# --- baseline an existing database, with verification
e=$(newdb); q "$e" "create table accounts (id int primary key, name text); alter table accounts enable row level security;" >/dev/null
check "an existing database that matches the baseline can be baselined" "$(code "$e" baseline)" "0"
check "a second baseline is refused" "$(code "$e" baseline)" "3"
check "a baselined database reports the fixture migrations as pending" "$(code "$e" status)" "2"
check "…and up applies them on top of the baseline" "$(code "$e" up)" "0"
check "…after which it is up to date" "$(code "$e" status)" "0"
f=$(newdb); q "$f" "create table accounts (id int primary key, name text, extra int); alter table accounts enable row level security;" >/dev/null
check "a database whose schema differs is refused" "$(code "$f" baseline)" "3"
check "…and nothing was recorded" "$(q "$f" "select to_regclass('public.schema_migrations') is not null")" "f"
check "--force records it anyway" "$(code "$f" baseline --force)" "0"
g=$(newdb); q "$g" "create table accounts (id int primary key, name text);" >/dev/null
check "a database missing row-level security is refused" "$(code "$g" baseline)" "3"

# --- verify
h=$(newdb); tool_in "$h" init >/dev/null
check "verify passes on a fresh init" "$(code "$h" verify-baseline)" "0"
q "$h" "alter table accounts add column surprise int" >/dev/null
check "verify fails after a column is added by hand" "$(code "$h" verify-baseline)" "1"
check "verify leaves no scratch database behind" "$(q postgres "select count(*) from pg_database where datname like 'mm_baseline_check_%'")" "0"

# --- the real repository files
r=$(newdb)
real() { MM_DB_URL="$base_url/$r" bash "$tool" "$@" 2>&1; }
check "the real baseline applies to an empty database (init)" "$(real init >/dev/null; echo $?)" "0"
check "the real baseline matches itself (verify)" "$(real verify-baseline >/dev/null; echo $?)" "0"
check "the real migrations directory is valid and nothing is pending or wrong" "$(real status >/dev/null; echo $?)" "$([ -n "$(ls "$repo"/db/migrations/*.sql 2>/dev/null)" ] && echo 2 || echo 0)"
check "applying the real migrations works" "$(real up >/dev/null; echo $?)" "0"
check "the database ends up at the latest version" "$(real status >/dev/null; echo $?)" "0"
check "grants runs the real runtime-role setup" "$(real grants >/dev/null; echo $?)" "0"
check "the runtime-role policies exist afterwards" "$(q "$r" "select count(*) > 0 from pg_policies where policyname='mm_runtime_service'")" "t"

echo "$pass passed, $failn failed"
[ "$failn" = 0 ]
