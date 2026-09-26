#!/usr/bin/env bash
# E2E HTTP-boot proof: boot the REAL server binary and drive live HTTP.
# Closes the "mechanism-proven, not server-proven" gap — these are real
# sockets over a booted process, not in-process harness calls.
#
# Usage: bash scripts/e2e_boot_test.sh
# Requires: dev stack up (db/nats/redis on localhost), config.yaml present.
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${E2E_PORT:-8080}"
BASE="http://127.0.0.1:$PORT"
DEV_EMAIL="${E2E_DEV_EMAIL:-audit2@test.com}"
DEV_PASS="${E2E_DEV_PASS:-test12345678}"
LOG="$(mktemp)"
PASS=0; FAIL=0

pass() { PASS=$((PASS+1)); printf "  ok   %s\n" "$1"; }
fail() { FAIL=$((FAIL+1)); printf "  FAIL %s  -> %s\n" "$1" "$2"; }

echo "== booting server on :$PORT (log $LOG) =="
# Launch in a dedicated process group so cleanup can kill the whole tree.
setsid bash -c "cd \"$ROOT\" && go run ./cmd/server" >"$LOG" 2>&1 &
SRV=$!
SRV_PGID="$SRV"
# wait for listen
for i in $(seq 1 40); do
  if sudo ss -ltn 2>/dev/null | grep -q ":$PORT "; then break; fi
  sleep 0.5
done
if ! sudo ss -ltn 2>/dev/null | grep -q ":$PORT "; then
  echo "server did not bind :$PORT"; tail -15 "$LOG"; exit 1
fi
echo "server listening"

cleanup() {
  # Kill the whole `go run` process group so the compiled child can't be orphaned.
  # `go run` may place the exe under $TMPDIR (e.g. hermes scratch) on top of the
  # default cache, so by-PID group-kill (not a name/cache match) is authoritative.
  if [ -n "${SRV_PGID:-}" ]; then
    kill -9 -- "-$SRV_PGID" 2>/dev/null
  fi
  kill "$SRV" 2>/dev/null
  pkill -9 -f "/home/ubuntu/.cache/go-build" 2>/dev/null
  rm -f "$LOG"
}
trap cleanup EXIT

echo "== 1. anonymous portal public config (real HTTP) =="
# pickup a device id from the db for a real tenant anchor
DEVID="$(sudo docker exec -i mega_magic_db_dev psql -U mega_admin -d mega_magic_dev -tAc \
  "select device_id from devices limit 1" 2>/dev/null | tr -d '[:space:]')"
if [ -z "$DEVID" ]; then
  echo "  SKIP: no device row in dev db (create one to test device-anchored portal)"; fail "portal-config" "no device id"
else
  code="$(curl -s -o /tmp/e2e_pub.json -w "%{http_code}" "$BASE/api/v1/portal/$DEVID/config")"
  if [ "$code" = "200" ]; then pass "portal public config 200"; else fail "portal public config" "http $code"; fi
fi

echo "== 2. login for a real token =="
TOKEN="$(curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$DEV_EMAIL\",\"password\":\"$DEV_PASS\"}" | \
  python3 -c 'import sys,json;print((json.load(sys.stdin).get("token") or ""))' 2>/dev/null)"
if [ -n "$TOKEN" ]; then
  pass "login token (${#TOKEN} chars)"
else
  fail "login" "no token returned"
fi

echo "== 3. authenticated staff portal config (protected chain over live HTTP) =="
if [ -n "$TOKEN" ]; then
  code="$(curl -s -o /tmp/e2e_staff.json -w "%{http_code}" "$BASE/api/v1/portal/config" -H "Authorization: Bearer $TOKEN")"
  if [ "$code" = "200" ]; then pass "staff config 200 (auth chain live)"; else fail "staff config" "http $code"; fi
fi

echo "== 4. unauthenticated protected route MUST be denied =="
code="$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/v1/portal/config")"
if [ "$code" = "401" ] || [ "$code" = "403" ]; then
  pass "unauthenticated denied ($code)"
else
  fail "unauthenticated denied" "expected 401/403, got $code"
fi

echo "== 5. authenticated topology map over live HTTP =="
if [ -n "$TOKEN" ]; then
  code="$(curl -s -o /tmp/e2e_topo.json -w "%{http_code}" "$BASE/api/v1/topology/map" -H "Authorization: Bearer $TOKEN")"
  if [ "$code" = "200" ]; then
    # must be valid JSON with a business-scoped roots array
    if python3 -c 'import json;d=json.load(open("/tmp/e2e_topo.json"));assert "roots" in d and "summary" in d' 2>/dev/null; then
      pass "topology map 200 (roots+summary)"
    else
      fail "topology map" "200 but malformed payload"
    fi
  else
    fail "topology map" "http $code"
  fi
else
  fail "topology map" "no token (skip ran before login)"
fi

echo
echo "RESULT: pass=$PASS fail=$FAIL"
[ "$FAIL" -eq 0 ]