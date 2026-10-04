#!/usr/bin/env bash
# deploy.sh — canonical, single-command deploy for Mega Magic.
#
# Source of truth:
#   - Frontend : /home/ubuntu/mega-magic-project/web/  (git-tracked)
#   - Backend  : /home/ubuntu/mega-magic-project/server (git-tracked Go binary source)
#
# Serve/run targets:
#   - Frontend : /opt/mega-magic-web/  (www-data must read it; /home/ubuntu is
#                drwxr-x--- so nginx CANNOT read the repo web/ directly — the
#                copy to /opt is a hard permissions requirement, not drift)
#   - Backend  : systemd mega-magic-api.service runs /home/ubuntu/mega-magic-project/server
#
# Usage:
#   ./deploy.sh web      # publish frontend only (sync + reload nginx)
#   ./deploy.sh api      # rebuild Go backend + swap binary + restart service
#   ./deploy.sh all      # both (default)
set -euo pipefail
cd "$(dirname "$0")"
ROOT="$PWD"

# Resolve the Go toolchain at runtime. It is installed at /usr/local/go/bin on
# this host, not /usr/bin, so a hardcoded path breaks every schema check and the
# build. Probe the usual locations and fail early with an actionable message
# rather than aborting mid-script.
GO=""
for candidate in /usr/local/go/bin/go /usr/lib/go/bin/go /usr/bin/go; do
  if [ -x "$candidate" ]; then GO="$candidate"; break; fi
done
if [ -z "$GO" ] && command -v go >/dev/null 2>&1; then GO="$(command -v go)"; fi
if [ -z "$GO" ]; then
  echo "FATAL: the Go toolchain was not found in /usr/local/go/bin, /usr/lib/go/bin, /usr/bin or on PATH." >&2
  echo "  Install Go, then re-run. Nothing has been deployed." >&2
  exit 1
fi
export GOROOT="${GOROOT:-$(dirname "$(dirname "$GO")")}"

# Resolve the operator-owned config once. It is deliberately kept OUT of the
# git-tracked tree, and on this host it lives one level above the checkout
# (/home/mmapp/app/config.yaml while the source is /home/mmapp/app/src). A bare
# relative path therefore finds nothing and every schema check fails closed.
CONFIG=""
for candidate in "$ROOT/config.yaml" "$ROOT/../config.yaml" /etc/mega-magic/config.yaml; do
  if [ -f "$candidate" ]; then CONFIG="$(cd "$(dirname "$candidate")" && pwd)/$(basename "$candidate")"; break; fi
done
if [ -z "$CONFIG" ]; then
  echo "FATAL: config.yaml not found." >&2
  echo "  Looked in $ROOT, $ROOT/.. and /etc/mega-magic." >&2
  echo "  Nothing has been deployed." >&2
  exit 1
fi
echo ">> Using config: $CONFIG"

deploy_web() {
  echo ">> Syncing frontend web/ -> /opt/mega-magic-web/"
  sudo rsync -a --delete "$ROOT/web/" /opt/mega-magic-web/

  # Cache-busting. Cloudflare serves /assets/* as `public, max-age=604800,
  # immutable` while app.html is DYNAMIC (never cached). So a changed stylesheet
  # would keep its old edge copy for a WEEK while the new HTML went live
  # immediately - the CDN would serve fresh markup against stale CSS/JS.
  # Versioning each asset URL by its content hash makes every change a new URL
  # that the edge has to fetch from origin. HTML is always revalidated. A
  # `?v=` query string is part of the Cloudflare cache key (verified: HIT with
  # no query, MISS with ?v=), so this busts the edge cache without an API purge.
  #
  # The asset list is DISCOVERED, not hardcoded. A hardcoded list silently skips
  # any asset added later - which is how hub-enrollment.js and the media-*
  # files ended up unstamped and able to serve a week-old copy.
  #
  # Every shared page is restamped so marketing, auth, pricing and the console
  # cannot drift across an immutable-cache deployment.
  local stamp_dir page asset f hash pat missing=0 ref
  stamp_dir=$(mktemp -d /tmp/mm-web-stamp.XXXXXX)
  sed_args=()

  # Build one -e expression per asset actually referenced by the pages.
  while IFS= read -r asset; do
    f="$ROOT/web/$asset"
    [ -f "$f" ] || continue
    hash=$(md5sum "$f" | cut -c1-8)
    # Escape dots so they match literally, and swallow any existing ?v=.
    pat=$(printf '%s' "$asset" | sed 's/[.]/[.]/g')
    # Replace the path AND any existing ?v=<hash> that follows it, in one shot.
    # Matching the optional query explicitly is what makes this idempotent - a
    # greedy [^\"']* instead re-appends the hash onto the old one and produces
    # ?v=00f3442626 on a second run. \" keeps the outer quoting intact.
    # BRE needs the literal ? escaped, and the whole tail optional so an
    # unstamped path and a stamped one both match exactly once.
    sed_args+=(-e "s#\($pat\)\(\\?v=[0-9a-f]*\)\{0,1\}#\1?v=$hash#g")
  done < <(grep -rhoE '/assets/(css|js)/[A-Za-z0-9._-]+\.(css|js)' "$ROOT"/web/*.html | sort -u)

  if [ ${#sed_args[@]} -eq 0 ]; then
    echo "ERROR: no assets discovered - refusing to publish unstamped HTML" >&2
    rm -rf "$stamp_dir"
    return 1
  fi
  echo ">> Stamping ${#sed_args[@]} assets by content hash"

  for page in $(cd "$ROOT/web" && ls *.html); do
    sed "${sed_args[@]}" "$ROOT/web/$page" > "$stamp_dir/$page"
    sudo install -m 644 "$stamp_dir/$page" "/opt/mega-magic-web/$page"
    rm -f "$stamp_dir/$page"
  done
  rmdir "$stamp_dir"

  # Fail loudly rather than letting a week-old asset reach a browser. Any
  # /assets/ reference in the published HTML must carry a ?v= stamp.
  for page in /opt/mega-magic-web/*.html; do
    while IFS= read -r ref; do
      if ! grep -q "$(printf '%s' "$ref" | sed 's/\./\\./g')?v=" "$page"; then
        echo "ERROR: $(basename "$page") references $ref with no ?v= stamp" >&2
        missing=1
      fi
    done < <(grep -rhoE '/assets/(css|js)/[A-Za-z0-9._-]+\.(css|js)' "$page" | sort -u)
  done
  if [ "$missing" -ne 0 ]; then
    echo "ERROR: cache-busting incomplete - fix before serving" >&2
    return 1
  fi

  sudo systemctl reload nginx
  echo ">> Frontend published (all assets content-hash stamped) + nginx reloaded"
}

deploy_api() {
  echo ">> Validating production configuration"
  # ENVIRONMENT is injected again into systemd below. Supplying it here makes
  # the preflight fail closed even if an old config.yaml omits or misspells the
  # environment field. No service is touched when validation fails.
  ENVIRONMENT=production $GO run ./cmd/config-check --config "$CONFIG"

  echo ">> Verifying required database schema"
  # This is intentionally a check, not an automatic production migration.
  # Apply and complete the two-stage rollout using docs/DATABASE-MIGRATIONS.md.
  # A stale schema stops the deploy before the binary or service is touched.
  ENVIRONMENT=production $GO run ./cmd/device-ssh-schema --action check --config "$CONFIG"
  ENVIRONMENT=production $GO run ./cmd/capacity-schema --action check --config "$CONFIG"
  ENVIRONMENT=production $GO run ./cmd/store-schema --action check --config "$CONFIG"
  ENVIRONMENT=production $GO run ./cmd/auth-session-schema --action check --config "$CONFIG"

  echo ">> Building Go binary"
  # Build to a TEMPORARY file first. Two reasons:
  #   1. A failed build must not touch the binary the service is running.
  #   2. The backup must be taken while `server` is still the OLD binary.
  # This previously read `go build -o server` then `cp server server.prev`,
  # which copied the NEW binary over the backup — so the "previous" file was
  # never the previous anything, and the rollback path it claims to keep did
  # not exist. Verified on the live host: server and server.prev had identical
  # md5sums.
  local tmpserver="server.new.$$"
  if ! $GO build -o "$tmpserver" cmd/server/main.go; then
    echo ">> BUILD FAILED — leaving the running binary untouched" >&2
    rm -f "$tmpserver"
    return 1
  fi
  # `server` is still the old binary here, so this is a REAL rollback point.
  if [ -f server ]; then
    cp -f server server.prev
  fi
  mv -f "$tmpserver" server

  # Persist the same fail-closed environment for the actual service process.
  # A build-time environment variable alone does not reach systemd.
  local dropin_dir="/etc/systemd/system/mega-magic-api.service.d"
  sudo install -d -m 755 "$dropin_dir"
  printf '[Service]\nEnvironment=ENVIRONMENT=production\n' | \
    sudo tee "$dropin_dir/99-production-environment.conf" >/dev/null
  sudo chmod 644 "$dropin_dir/99-production-environment.conf"
  sudo systemctl daemon-reload

  echo ">> Restarting mega-magic-api"
  sudo systemctl restart mega-magic-api
  sleep 3
  systemctl is-active mega-magic-api
  echo ">> API restarted"
}

MODE="${1:-all}"
case "$MODE" in
  web) deploy_web ;;
  api) deploy_api ;;
  all) deploy_api; deploy_web ;;
  *) echo "usage: $0 [web|api|all]"; exit 2 ;;
esac

echo ">> Post-deploy verify"
curl -sk -o /dev/null -w 'landing=%{http_code} ' https://mikromagic.com/
curl -sk -o /dev/null -w 'app=%{http_code} ' https://mikromagic.com/app.html
curl -sk -o /dev/null -w 'health=%{http_code}\n' https://mikromagic.com/health
echo ">> Done"
