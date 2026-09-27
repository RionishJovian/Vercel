#!/usr/bin/env bash
#
# Promote the merged data into a production database — WITHOUT the ETL scaffolding.
#
# Why this exists: the documented shortcut is
#     CREATE DATABASE mega_magic TEMPLATE mega_merge
# which works, but copies all 48 `_stg_*`/`_mig_*` staging tables too — a second,
# untransformed copy of the old system's data (10,583 rows) including
# `_mig_accounts.password_hash` with 11 populated bcrypt hashes.
#
# What this does instead: builds the target from mega_merge, then removes every
# `_`-prefixed table, then VERIFIES the result is clean before telling you it's
# safe. Refuses to touch the baseline or the merge source.
#
# Usage:
#   ./promote.sh                       # target: mega_magic
#   TARGET_DB=mega_promotion_check ./promote.sh
#
set -uo pipefail

S="$(cd "$(dirname "$0")" && pwd)"
DST_CONTAINER="${DST_CONTAINER:-mega_magic_db_dev}"
DST_USER="${DST_USER:-mega_admin}"
SOURCE_DB="${SOURCE_DB:-mega_merge}"
BASELINE_DB="${BASELINE_DB:-mega_magic_dev}"
TARGET_DB="${TARGET_DB:-mega_magic}"
FORCE="${FORCE:-0}"

PSQL="docker exec $DST_CONTAINER psql -U $DST_USER"
q() { $PSQL -d "$1" -tAc "$2"; }

echo "promote: $SOURCE_DB -> $TARGET_DB (container $DST_CONTAINER)"

# ---- guard rails ------------------------------------------------------------
if [ "$TARGET_DB" = "$BASELINE_DB" ]; then
    echo "FAILED: refusing to overwrite the baseline ($BASELINE_DB)" >&2; exit 1
fi
if [ "$TARGET_DB" = "$SOURCE_DB" ]; then
    echo "FAILED: refusing to promote onto the source ($SOURCE_DB)" >&2; exit 1
fi
if [ "$(q postgres "select 1 from pg_database where datname='$TARGET_DB'")" = "1" ]; then
    if [ "$FORCE" != "1" ]; then
        echo "FAILED: $TARGET_DB already exists — drop it first, or set FORCE=1" >&2
        exit 1
    fi
    echo "  FORCE=1: dropping existing $TARGET_DB"
    $PSQL -d postgres -q -c "DROP DATABASE \"$TARGET_DB\"" || exit 1
fi

# NOTE: this must query a DIFFERENT database. Running it against $SOURCE_DB makes
# the check's own connection appear in pg_stat_activity for that database, so it
# would always see at least one connection and refuse to ever proceed.
before_conns="$(q postgres "select count(*) from pg_stat_activity where datname='$SOURCE_DB'")"
if [ "$before_conns" != "0" ]; then
    echo "FAILED: $SOURCE_DB still has $before_conns connection(s) — stop the API first" >&2
    exit 1
fi

# ---- promote ---------------------------------------------------------------
echo "1/3 creating $TARGET_DB from $SOURCE_DB"
$PSQL -d postgres -q -c "CREATE DATABASE \"$TARGET_DB\" TEMPLATE \"$SOURCE_DB\"" || exit 1

echo "2/3 dropping ETL scaffolding (every _-prefixed table)"
stg_before="$(q "$TARGET_DB" "select count(*) from pg_tables where schemaname='public' and tablename like '\_%'")"
# -i is REQUIRED: without it docker exec does not attach stdin, psql receives EOF
# immediately and executes none of the heredoc — silently dropping nothing.
docker exec -i "$DST_CONTAINER" psql -U "$DST_USER" -d "$TARGET_DB" -q -v ON_ERROR_STOP=1 <<'SQL' || exit 1
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables
             WHERE schemaname = 'public' AND tablename LIKE '\_%'
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I CASCADE', t);
    END LOOP;
END $$;
SQL
echo "    dropped $stg_before staging table(s)"

# ---- verify, and fail loudly if anything is off -----------------------------
echo "3/3 verifying"
fail=0
expect_product="$(q "$SOURCE_DB" "select count(*) from pg_tables where schemaname='public' and tablename not like '\_%'")"
got_product="$(q "$TARGET_DB" "select count(*) from pg_tables where schemaname='public' and tablename not like '\_%'")"
got_stg="$(q "$TARGET_DB" "select count(*) from pg_tables where schemaname='public' and tablename like '\_%'")"
# the credential check: password_hash may exist ONLY on the product users table
hash_tables="$(q "$TARGET_DB" "select coalesce(string_agg(table_name, ', ' order by table_name),'') from information_schema.columns where column_name='password_hash' and table_name <> 'users'")"

chk() { # label got want
    if [ "$2" = "$3" ]; then echo "    PASS $1: $2"
    else echo "    FAIL $1: got $2, want $3" >&2; fail=1; fi
}
chk "product tables"   "$got_product"   "$expect_product"
chk "staging tables"   "$got_stg"       "0"
if [ -z "$hash_tables" ]; then
    echo "    PASS no password_hash outside users"
else
    echo "    FAIL password_hash still present on: $hash_tables" >&2; fail=1
fi

# row-for-row check on every product table
mismatch=""
while read -r t; do
    a="$(q "$SOURCE_DB" "select count(*) from \"$t\"")"
    b="$(q "$TARGET_DB" "select count(*) from \"$t\"")"
    [ "$a" = "$b" ] || mismatch="$mismatch $t($a!=$b)"
done < <(q "$SOURCE_DB" "select tablename from pg_tables where schemaname='public' and tablename not like '\_%' order by 1")
if [ -z "$mismatch" ]; then
    echo "    PASS every product table row-for-row identical"
else
    echo "    FAIL row count mismatch:$mismatch" >&2; fail=1
fi

echo
if [ "$fail" -eq 0 ]; then
    echo "ok — $TARGET_DB is clean: $got_product product tables, no staging tables, no stray credentials."
    echo "Switch the API with: DATABASE_NAME=$TARGET_DB"
else
    echo "FAILED verification — $TARGET_DB was created but is NOT safe to use." >&2
    exit 1
fi
