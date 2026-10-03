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

deploy_web() {
  echo ">> Syncing frontend web/ -> /opt/mega-magic-web/"
  sudo rsync -a --delete "$ROOT/web/" /opt/mega-magic-web/

  # Cache-busting. Cloudflare serves /assets/* as `public, max-age=604800,
  # immutable` while app.html is DYNAMIC (never cached). So a changed stylesheet
  # would keep its old edge copy for a WEEK while the new HTML went live
  # immediately — the CDN would serve fresh markup against stale CSS/JS.
  # Versioning each asset URL by its content hash makes every change a new URL
  # that the edge has to fetch from origin. HTML is always revalidated. Stamp
  # every page that imports shared assets so marketing, auth, pricing, and the
  # console cannot drift across an immutable-cache deployment.
  local vcss vredesign vapi vapp vtheme vi18n vnetwork stamp_dir page
  vcss=$(md5sum "$ROOT/web/assets/css/styles.css" | cut -c1-8)
  vredesign=$(md5sum "$ROOT/web/assets/css/redesign.css" | cut -c1-8)
  vapi=$(md5sum "$ROOT/web/assets/js/api.js"       | cut -c1-8)
  vapp=$(md5sum "$ROOT/web/assets/js/app.js"       | cut -c1-8)
  vtheme=$(md5sum "$ROOT/web/assets/js/theme.js"   | cut -c1-8)
  vi18n=$(md5sum "$ROOT/web/assets/js/i18n.js"    | cut -c1-8)
  vnetwork=$(md5sum "$ROOT/web/assets/js/network-control.js" | cut -c1-8)
  stamp_dir=$(mktemp -d /tmp/mm-web-stamp.XXXXXX)
  for page in app index signin pricing; do
    sed -e "s#\(/assets/css/styles\.css\)[^\"']*#\1?v=$vcss#" \
        -e "s#\(/assets/css/redesign\.css\)[^\"']*#\1?v=$vredesign#" \
        -e "s#\(/assets/js/api\.js\)[^\"']*#\1?v=$vapi#" \
        -e "s#\(/assets/js/app\.js\)[^\"']*#\1?v=$vapp#" \
        -e "s#\(/assets/js/theme\.js\)[^\"']*#\1?v=$vtheme#" \
        -e "s#\(/assets/js/i18n\.js\)[^\"']*#\1?v=$vi18n#" \
        -e "s#\(/assets/js/network-control\.js\)[^\"']*#\1?v=$vnetwork#" \
        "$ROOT/web/$page.html" > "$stamp_dir/$page.html"
    sudo install -m 644 "$stamp_dir/$page.html" "/opt/mega-magic-web/$page.html"
    rm -f "$stamp_dir/$page.html"
  done
  rmdir "$stamp_dir"

  sudo systemctl reload nginx
  echo ">> Frontend published (assets v: css=$vcss redesign=$vredesign js=$vapi app=$vapp theme=$vtheme i18n=$vi18n) + nginx reloaded"
}

deploy_api() {
  echo ">> Validating production configuration"
  # ENVIRONMENT is injected again into systemd below. Supplying it here makes
  # the preflight fail closed even if an old config.yaml omits or misspells the
  # environment field. No service is touched when validation fails.
  ENVIRONMENT=production /usr/bin/go run ./cmd/config-check --config config.yaml

  echo ">> Verifying required database schema"
  # This is intentionally a check, not an automatic production migration.
  # Apply and complete the two-stage rollout using docs/DATABASE-MIGRATIONS.md.
  # A stale schema stops the deploy before the binary or service is touched.
  ENVIRONMENT=production /usr/bin/go run ./cmd/device-ssh-schema --action check --config config.yaml
  ENVIRONMENT=production /usr/bin/go run ./cmd/capacity-schema --action check --config config.yaml
  ENVIRONMENT=production /usr/bin/go run ./cmd/store-schema --action check --config config.yaml
  ENVIRONMENT=production /usr/bin/go run ./cmd/auth-session-schema --action check --config config.yaml

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
  if ! /usr/bin/go build -o "$tmpserver" cmd/server/main.go; then
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
