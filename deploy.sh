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
  # that the edge has to fetch from origin. app.html is always revalidated, so
  # stamping it here is enough; no Cloudflare API access needed.
  local vcss vapi vapp
  vcss=$(md5sum "$ROOT/web/assets/css/styles.css" | cut -c1-8)
  vapi=$(md5sum "$ROOT/web/assets/js/api.js"       | cut -c1-8)
  vapp=$(md5sum "$ROOT/web/assets/js/app.js"       | cut -c1-8)
  sed -e "s#\(/assets/css/styles\.css\)[^\"']*#\1?v=$vcss#" \
      -e "s#\(/assets/js/api\.js\)[^\"']*#\1?v=$vapi#" \
      -e "s#\(/assets/js/app\.js\)[^\"']*#\1?v=$vapp#" \
      "$ROOT/web/app.html" > /tmp/mm-app.stamped.html
  sudo install -m 644 /tmp/mm-app.stamped.html /opt/mega-magic-web/app.html
  rm -f /tmp/mm-app.stamped.html

  sudo systemctl reload nginx
  echo ">> Frontend published (assets v: css=$vcss js=$vapi app=$vapp) + nginx reloaded"
}

deploy_api() {
  echo ">> Building Go binary"
  /usr/bin/go build -o server cmd/server/main.go
  cp -f server server.prev   # keep previous for instant rollback
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