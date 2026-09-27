#!/usr/bin/env bash
# ============================================================================
# E2 — public end-to-end check for Mega Magic.
#
# Exercises the PUBLIC surface over HTTPS against a live domain (default
# https://mikromagic.com), including a real sign-in and every key authenticated
# endpoint. Fails LOUDLY (non-zero exit + message) on any assertion miss.
#
# Credentials: NEVER committed. Provide the DEVELOPER login via env
#   MM_E2E_EMAIL / MM_E2E_PASSWORD
# or an untracked env file (default ~/.mm-e2e.env, override MM_E2E_CREDS):
#   MM_E2E_EMAIL=you@example.com
#   MM_E2E_PASSWORD=...
#
# Usage:
#   scripts/e2e-public.sh                 # full run (needs credentials)
#   scripts/e2e-public.sh --public-only   # unauthenticated steps only
#   scripts/e2e-public.sh --dry-run       # print the plan, make no requests
#   MM_E2E_BASE=http://localhost:8080 scripts/e2e-public.sh
#
# Requires: curl, node. (No jq dependency.)
# ============================================================================
set -euo pipefail

BASE="${MM_E2E_BASE:-https://mikromagic.com}"
MODE="full"
case "${1:-}" in
  --dry-run) MODE="dry" ;;
  --public-only) MODE="public" ;;
  "") ;;
  *) echo "unknown flag: $1" >&2; exit 2 ;;
esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PASSED=0
step() { printf '\n▶ %s\n' "$*"; }
ok()   { PASSED=$((PASSED+1)); printf '  ✓ %s\n' "$*"; }
die()  { printf '\n✗ E2E FAIL: %s\n' "$*" >&2; exit 1; }

# jshape <file> <dotted.path>  — succeeds when the JSON path exists
jshape() {
  node -e '
    const fs=require("fs");
    let d; try { d=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); } catch(e){ process.exit(1); }
    let v=d; for (const k of process.argv[2].split(".")) { if (v==null) process.exit(1); v=v[k]; }
    if (v===undefined) process.exit(1);
  ' "$1" "$2"
}

# http <method> <path> <outfile> [json-body] [bearer]  -> echoes status code
http() {
  local method="$1" path="$2" out="$3" body="${4:-}" bearer="${5:-}"
  local args=(-sS -o "$out" -w '%{http_code}' -X "$method" --max-time 20 "$BASE$path")
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' -d "$body")
  [[ -n "$bearer" ]] && args+=(-H "Authorization: Bearer $bearer")
  curl "${args[@]}" 2>/dev/null || echo "000"
}

# expect <method> <path> <outfile> <want-status> <shape|-> [body] [bearer]
expect() {
  local method="$1" path="$2" out="$3" want="$4" shape="$5" body="${6:-}" bearer="${7:-}"
  local code; code="$(http "$method" "$path" "$out" "$body" "$bearer")"
  [[ "$code" == "$want" ]] || { sed -n '1,4p' "$out" 2>/dev/null | sed 's/^/    /'; die "$method $path -> HTTP $code (want $want)"; }
  if [[ "$shape" != "-" ]]; then
    jshape "$out" "$shape" || die "$method $path -> JSON shape '$shape' missing"
  fi
  ok "$method $path -> $code"
}

# check_inline_js <url-path>  — fetch an HTML page and node --check its inline JS
check_inline_js() {
  local p="$1" f="$TMP/page.html" code
  code="$(http GET "$p" "$f")"
  [[ "$code" == "200" ]] || return 1
  node -e '
    const fs=require("fs");
    const html=fs.readFileSync(process.argv[1],"utf8");
    const m=html.match(/<script>([\s\S]*?)<\/script>/);
    if(!m){ process.exit(0); }               // no inline script: nothing to check
    fs.writeFileSync(process.argv[2], m[1]);
  ' "$f" "$TMP/page.js"
  if [[ -s "$TMP/page.js" ]]; then
    node --check "$TMP/page.js" || die "inline JS in $p failed node --check"
    ok "inline JS in $p parses ($(wc -c <"$TMP/page.js" | tr -d ' ') bytes)"
  else
    ok "$p has no inline JS to check"
  fi
}

printf 'E2 public E2E — base=%s mode=%s\n' "$BASE" "$MODE"

if [[ "$MODE" == "dry" ]]; then
  printf '\nPLAN:\n'
  printf '  1. GET  /                      -> 200 (landing)\n'
  printf '  2. POST /api/v1/auth/login     -> 200 + token\n'
  printf '  3. GET  /api/v1/me             -> 200 (user.email)\n'
  printf '  4. GET  /api/v1/fleet/health   -> 200 (total_peers)\n'
  printf '  5. GET  /api/v1/topology/map   -> 200 (business_id)\n'
  printf '  6. GET  /api/v1/security/posture -> 200 (posture)\n'
  printf '  7. GET  /api/v1/team           -> 200 (team[])\n'
  printf '  8. GET  /api/v1/routers        -> 200; then backups list for first device\n'
  printf '  9. node --check inline JS on /portal.html (and /app.html if served)\n'
  printf '\nNo requests made (dry-run).\n'
  exit 0
fi

# ---- 1. landing ------------------------------------------------------------
step "landing"
expect GET / "$TMP/home.html" 200 - || true

# ---- 9. inline JS (public, do early) --------------------------------------
step "inline JS syntax"
check_js_seen=0
for p in /portal.html /app.html; do
  if check_inline_js "$p"; then check_js_seen=1; fi
done
[[ "$check_js_seen" == "1" ]] || die "no portal/app page served inline JS to check"

if [[ "$MODE" == "public" ]]; then
  printf '\nPUBLIC-ONLY run complete: %d checks passed.\n' "$PASSED"
  exit 0
fi

# ---- credentials -----------------------------------------------------------
CREDS_FILE="${MM_E2E_CREDS:-$HOME/.mm-e2e.env}"
if [[ -z "${MM_E2E_EMAIL:-}" && -f "$CREDS_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a; . "$CREDS_FILE"; set +a
fi
[[ -n "${MM_E2E_EMAIL:-}" && -n "${MM_E2E_PASSWORD:-}" ]] || \
  die "credentials missing: set MM_E2E_EMAIL / MM_E2E_PASSWORD (env or $CREDS_FILE)"

# ---- 2. sign-in ------------------------------------------------------------
step "sign-in"
LOGIN_BODY="$(node -e 'process.stdout.write(JSON.stringify({email:process.argv[1],password:process.argv[2]}))' "$MM_E2E_EMAIL" "$MM_E2E_PASSWORD")"
expect POST /api/v1/auth/login "$TMP/login.json" 200 token "$LOGIN_BODY"
TOKEN="$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).token||"")' "$TMP/login.json")"
[[ -n "$TOKEN" ]] || die "login returned no token"

# ---- 3-8. authenticated surface -------------------------------------------
step "authenticated endpoints"
expect GET /api/v1/me               "$TMP/me.json"       200 user.email       "" "$TOKEN"
expect GET /api/v1/fleet/health     "$TMP/fleet.json"    200 total_peers      "" "$TOKEN"
expect GET /api/v1/topology/map     "$TMP/topo.json"     200 business_id      "" "$TOKEN"
expect GET /api/v1/security/posture "$TMP/sec.json"      200 count            "" "$TOKEN"
expect GET /api/v1/team             "$TMP/team.json"     200 team             "" "$TOKEN"
expect GET /api/v1/routers          "$TMP/routers.json"  200 -                "" "$TOKEN"

DEV_ID="$(node -e '
  const d=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));
  const rs=d.routers||d||[]; process.stdout.write(rs[0]?(rs[0].device_id||""):"");
' "$TMP/routers.json")"
if [[ -n "$DEV_ID" ]]; then
  expect GET "/api/v1/devices/$DEV_ID/backups" "$TMP/bk.json" 200 backups "" "$TOKEN"
  expect GET "/api/v1/devices/$DEV_ID/changes" "$TMP/ch.json" 200 changes "" "$TOKEN"
else
  printf '  (no devices registered — skipping per-device backup checks)\n'
fi

printf '\n✓ E2E PASS — %d checks passed against %s\n' "$PASSED" "$BASE"
