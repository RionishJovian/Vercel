#!/usr/bin/env bash
# Tests mm-offsite-copy against stand-ins for age, rclone, curl and logger:  bash deploy/server/test-mm-offsite-copy.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/out" "$work/remote" "$work/tmp"
cat >"$work/bin/age" <<'STUB'
#!/usr/bin/env bash
# age -r <recipient> -o <out> <in>
[ -f "$STUB_DIR/age_fails" ] && exit 1
out=""; in=""; while [ $# -gt 0 ]; do case "$1" in -r) shift;; -o) out="$2"; shift;; *) in="$1";; esac; shift; done
{ printf 'AGE-ENCRYPTED:'; cat "$in"; } >"$out"
STUB
cat >"$work/bin/rclone" <<'STUB'
#!/usr/bin/env bash
echo "$*" >>"$STUB_DIR/rclone.args"
env | grep -E '^RCLONE_CONFIG_R2_(ACCESS|SECRET|ENDPOINT)' >>"$STUB_DIR/rclone.env" 2>/dev/null
case "$1" in
  lsf) inc=""; prev=""; for a in "$@"; do [ "$prev" = "--include" ] && inc="$a"; prev="$a"; done
       [ -f "$STUB_DIR/remote/$inc" ] && wc -c <"$STUB_DIR/remote/$inc" | tr -d ' ';;
  copyto) [ -f "$STUB_DIR/upload_fails" ] && exit 1
          dest="${3##*/}"; cp "$2" "$STUB_DIR/remote/$dest"
          [ -f "$STUB_DIR/truncate" ] && head -c 3 "$2" >"$STUB_DIR/remote/$dest";;
  delete) [ -f "$STUB_DIR/prune_fails" ] && exit 1; echo "$*" >>"$STUB_DIR/prune.args";;
esac
exit 0
STUB
printf '#!/usr/bin/env bash\necho "$@" >>"$STUB_DIR/pings"\n' >"$work/bin/curl"
printf '#!/usr/bin/env bash\necho "$*" >>"$STUB_DIR/log"\n' >"$work/bin/logger"
chmod +x "$work/bin/"*

export PATH="$work/bin:$PATH" STUB_DIR="$work" BACKUP_DIR="$work/out" TMPDIR="$work/tmp"
export R2_ACCOUNT_ID=acct123 R2_ACCESS_KEY_ID=keyid123 R2_SECRET_ACCESS_KEY=SECRETVALUE R2_BUCKET=bucket1
export AGE_RECIPIENT=age1publickeyexample HEALTHCHECK_OFFSITE_URL=https://ping.example/off KEEP_REMOTE_DAYS=30
mkdir -p "$work/hub"; printf 'HUB-SECRET-KEY' >"$work/hub/wg0.conf"; printf '{"peers":[]}' >"$work/hub/peers.json"
export HUB_STATE_PATHS="$work/hub/wg0.conf $work/hub/peers.json"
pass=0; failn=0
check() { if [ "$2" = "$3" ]; then pass=$((pass + 1)); else failn=$((failn + 1)); echo "FAIL: $1 (got '$2', want '$3')"; fi; }
reset() { rm -rf "$work"/out/* "$work"/remote/* "$work"/tmp/* "$work"/pings "$work"/log "$work"/rclone.args "$work"/rclone.env "$work"/prune.args "$work"/age_fails "$work"/upload_fails "$work"/truncate "$work"/prune_fails; echo "PLAINTEXT-DUMP-CONTENT" >"$work/out/mega_magic-20261102T031000Z.dump"; }
run() { bash "$here/mm-offsite-copy" >/dev/null 2>>"$work/errors.log"; echo $?; }
remote_files() { ls "$work/remote" | tr '\n' ' '; }

reset
check "good run exits 0" "$(run)" "0"
hub_copy="hub-state-$(date -u +%Y%m%d).tar.gz.age"
check "the dump and the Hub files are uploaded" "$(remote_files)" "$hub_copy mega_magic-20261102T031000Z.dump.age "
check "the Hub files are encrypted" "$(head -c 14 "$work/remote/$hub_copy")" "AGE-ENCRYPTED:"
check "the Hub's key is not readable in the upload" "$(grep -c HUB-SECRET-KEY "$work/remote/$hub_copy")" "0"
check "what is uploaded is the encrypted data, not the plaintext" "$(head -c 14 "$work/remote/mega_magic-20261102T031000Z.dump.age")" "AGE-ENCRYPTED:"
check "success pings the check" "$(tail -1 "$work/pings" | grep -c 'off$')" "1"
check "no temporary file is left behind" "$(ls "$work/tmp" | wc -l | tr -d ' ')" "0"
check "old remote copies are pruned with the retention, dumps and Hub files alike" "$(grep -c -- '--min-age 30d' "$work/prune.args")" "2"
check "the secret is never on a command line" "$(grep -c SECRETVALUE "$work/rclone.args")" "0"
calls=$(wc -l <"$work/rclone.args" | tr -d ' ')
check "every rclone call gets the secret through its environment" "$(grep -c 'SECRET_ACCESS_KEY=SECRETVALUE' "$work/rclone.env")" "$calls"
check "every rclone call targets the account's R2 address" "$(grep -c 'ENDPOINT=https://acct123.r2.cloudflarestorage.com' "$work/rclone.env")" "$calls"

reset; check "second run skips an already uploaded dump" "$(run)$(run)" "00"
check "…and does not upload the dump twice" "$(grep -c '^copyto.*mega_magic' "$work/rclone.args")" "1"
reset; run >/dev/null; rm -f "$work/remote/$hub_copy"
check "a run with the dump already uploaded still copies the Hub files" "$(run)$(remote_files)" "0$hub_copy mega_magic-20261102T031000Z.dump.age "

reset; export HUB_STATE_PATHS="$work/hub/wg0.conf $work/hub/missing.json"
check "a missing Hub file fails" "$(run)" "1"
check "a missing Hub file pings failure" "$(tail -1 "$work/pings" | grep -c '/fail')" "1"
export HUB_STATE_PATHS="$work/hub/wg0.conf $work/hub/peers.json"

for v in R2_ACCOUNT_ID R2_ACCESS_KEY_ID R2_SECRET_ACCESS_KEY R2_BUCKET AGE_RECIPIENT; do
  reset; old="${!v}"; export "$v="
  check "missing $v fails" "$(run)" "1"
  check "missing $v uploads nothing" "$(remote_files)" ""
  export "$v=$old"
done
reset; export AGE_RECIPIENT=AGE-SECRET-KEY-1ABCDEF
check "a private key is refused as the recipient" "$(run)" "1"
check "a private key uploads nothing" "$(remote_files)" ""
export AGE_RECIPIENT=age1publickeyexample

reset; rm -f "$work"/out/*
check "no dump fails" "$(run)" "1"
check "no dump pings failure" "$(tail -1 "$work/pings" | grep -c '/fail')" "1"

reset; touch -t 202601010000 "$work/out/mega_magic-20261102T031000Z.dump"
check "a stale dump fails (the nightly backup did not run)" "$(run)" "1"
check "a stale dump is not uploaded, but the Hub files still are" "$(remote_files)" "$hub_copy "

reset; touch "$work/age_fails"
check "encryption failure fails" "$(run)" "1"
check "encryption failure uploads nothing and leaves no temp file" "$(remote_files)$(ls "$work/tmp" | wc -l | tr -d ' ')" "0"

reset; touch "$work/upload_fails"
check "upload failure fails" "$(run)" "1"
check "upload failure pings failure" "$(tail -1 "$work/pings" | grep -c '/fail')" "1"
check "upload failure leaves no temp file" "$(ls "$work/tmp" | wc -l | tr -d ' ')" "0"

reset; touch "$work/truncate"
check "a size mismatch after upload fails" "$(run)" "1"

reset; touch "$work/prune_fails"
check "a failing prune does not fail the copy" "$(run)" "0"
check "a failing prune is logged, for dumps and for Hub files" "$(grep -c 'pruning' "$work/log")" "2"

reset; HEALTHCHECK_OFFSITE_URL="" bash "$here/mm-offsite-copy" >/dev/null 2>&1
check "no ping URL means no ping" "$(ls "$work/pings" 2>/dev/null | wc -l | tr -d ' ')" "0"

echo "$pass passed, $failn failed"
if [ "$failn" != 0 ] && [ -s "$work/errors.log" ]; then echo "--- what the script printed to stderr:"; sort "$work/errors.log" | uniq -c | sort -rn | head -8; fi
[ "$failn" = 0 ]
