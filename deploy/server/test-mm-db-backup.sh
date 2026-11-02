#!/usr/bin/env bash
# Tests mm-db-backup against a stand-in for `docker`, `curl` and `logger`, so every failure path is exercised without
# a database:  bash deploy/server/test-mm-db-backup.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/out"
cat >"$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
# docker exec [-i] <container> pg_dump ... | pg_restore -l   (behaviour chosen by files in $STUB_DIR)
args="$*"
case "$args" in
  *pg_dump*) [ -f "$STUB_DIR/dump_fails" ] && exit 1; cat "$STUB_DIR/dump_content" 2>/dev/null; exit 0;;
  *pg_restore*) cat >/dev/null; [ -f "$STUB_DIR/restore_fails" ] && exit 1; cat "$STUB_DIR/restore_list"; exit 0;;
esac
exit 0
STUB
printf '#!/usr/bin/env bash\necho "$@" >>"$STUB_DIR/pings"\n' >"$work/bin/curl"
printf '#!/usr/bin/env bash\necho "$*" >>"$STUB_DIR/log"\n' >"$work/bin/logger"
chmod +x "$work/bin/"*

export PATH="$work/bin:$PATH" STUB_DIR="$work" BACKUP_DIR="$work/out" HEALTHCHECK_BACKUP_URL="https://ping.example/abc"
export KEEP_DAYS=14
pass=0; failn=0
check() { if [ "$2" = "$3" ]; then pass=$((pass + 1)); else failn=$((failn + 1)); echo "FAIL: $1 (got '$2', want '$3')"; fi; }
reset() { rm -f "$work"/dump_fails "$work"/restore_fails "$work"/pings "$work"/log "$work"/out/*; head -c 4000 /dev/zero | tr '\0' 'x' >"$work/dump_content"; echo '1; 1 TABLE public businesses mm' >"$work/restore_list"; }
dumps() { ls "$work/out" 2>/dev/null | grep -c '\.dump$'; }
partials() { ls "$work/out" 2>/dev/null | grep -c '\.partial$'; }
run() { bash "$here/mm-db-backup" >/dev/null 2>>"$work/errors.log"; echo $?; }

reset
check "good dump exits 0" "$(run)" "0"
check "good dump is kept" "$(dumps)" "1"
check "good dump pings success" "$(tail -1 "$work/pings" 2>/dev/null | grep -c 'abc$')" "1"
check "dump file is private" "$(ls -l "$work"/out/*.dump | cut -c1-10)" "-rw-------"

reset; : >"$work/dump_content"
check "empty dump fails" "$(run)" "1"
check "empty dump leaves nothing" "$(dumps)$(partials)" "00"
check "empty dump pings failure" "$(tail -1 "$work/pings" | grep -c '/fail')" "1"

reset; touch "$work/dump_fails"
check "pg_dump failure fails" "$(run)" "1"
check "pg_dump failure leaves nothing" "$(dumps)$(partials)" "00"

reset; touch "$work/restore_fails"
check "unreadable dump fails" "$(run)" "1"
check "unreadable dump is discarded" "$(dumps)$(partials)" "00"

reset; echo '1; 1 SCHEMA - public mm' >"$work/restore_list"
check "dump without tables fails" "$(run)" "1"
check "dump without tables is discarded" "$(dumps)$(partials)" "00"

reset; head -c 10000 /dev/zero | tr '\0' 'x' >"$work/out/mega_magic-20260101T000000Z.dump"
head -c 4000 /dev/zero | tr '\0' 'x' >"$work/dump_content"
check "a dump less than half the previous one fails" "$(run)" "1"
check "the previous good dump is kept" "$(dumps)" "1"
check "no partial file is left behind" "$(partials)" "0"
head -c 6000 /dev/zero | tr '\0' 'x' >"$work/dump_content"
check "a dump over half the previous one passes" "$(run)" "0"
check "both dumps are kept" "$(dumps)" "2"

reset; : >"$work/out/mega_magic-20250101T000000Z.dump"; touch -t 202501010000 "$work/out/mega_magic-20250101T000000Z.dump"
check "run with an old dump present passes" "$(run)" "0"
check "dumps older than the retention are removed" "$(ls "$work/out" | grep -c 20250101)" "0"
check "the new dump survives pruning" "$(dumps)" "1"

reset; HEALTHCHECK_BACKUP_URL="" bash "$here/mm-db-backup" >/dev/null 2>&1
check "no ping URL means no ping" "$(ls "$work/pings" 2>/dev/null | wc -l | tr -d ' ')" "0"

echo "$pass passed, $failn failed"
if [ "$failn" != 0 ] && [ -s "$work/errors.log" ]; then echo "--- what the script printed to stderr:"; sort "$work/errors.log" | uniq -c | sort -rn | head -8; fi
[ "$failn" = 0 ]
