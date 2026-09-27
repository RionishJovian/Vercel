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
  sudo systemctl reload nginx
  echo ">> Frontend published + nginx reloaded"
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