#!/usr/bin/env bash
# Scoped deployment against an already tested production-source staging copy.
set -euo pipefail
stage=${1:?staged directory required}
test "$stage" = /home/ubuntu/mm-hub-stage.EaVGi9
root=/home/ubuntu/mega-magic-project
cd "$root"
test "$(sha256sum internal/server/server.go | awk '{print $1}')" = "${2:?expected server source hash required}"
backup=$(mktemp -d /home/ubuntu/deploy-backups/hub-management.XXXXXX)
chmod 700 "$backup"
files=(internal/server/server.go internal/server/hub_management.go internal/config/config.go
 internal/handler/vps_handler.go internal/handler/hub_enrollment.go internal/handler/hub_enrollment_test.go
 internal/handler/hub_management.go internal/handler/hub_management_test.go
 internal/database/device_credentials.go internal/hub/client.go internal/hub/client_test.go
 internal/hub/router_script.go internal/hub/router_script_test.go)
cp -p server "$backup/server"
for file in "${files[@]}"; do
 mkdir -p "$backup/$(dirname "$file")"
 if [ -f "$file" ]; then cp -p "$file" "$backup/$file"; else printf '%s\n' "$file" >> "$backup/new-files.txt"; fi
 cp -p "$stage/$file" "$file"
done
rollback() {
 for file in "${files[@]}"; do
  if [ -f "$backup/$file" ]; then cp -p "$backup/$file" "$file";
  elif [ -f "$file" ]; then mv "$file" "$backup/$(dirname "$file")/$(basename "$file").disabled"; fi
 done
 cp -p "$backup/server" server
 if [ -f /etc/systemd/system/mega-magic-api.service.d/98-magic-hub-enrollment.conf ]; then
  sudo mv /etc/systemd/system/mega-magic-api.service.d/98-magic-hub-enrollment.conf "$backup/98-enrollment.disabled"
 fi
 sudo systemctl daemon-reload
 sudo systemctl restart mega-magic-api
 bash ./deploy.sh web
}
trap 'rollback' ERR
sudo install -d -m 755 /etc/systemd/system/mega-magic-api.service.d
printf '[Service]\nEnvironment=VPS_HUB_ENROLLMENT_ENABLED=true\n' | sudo tee /etc/systemd/system/mega-magic-api.service.d/98-magic-hub-enrollment.conf >/dev/null
bash ./deploy.sh all
curl --fail --silent https://mikromagic.com/health >/dev/null
sudo systemctl is-active --quiet mega-magic-api
trap - ERR
printf 'Scoped Hub app deployment complete. Rollback: %s\n' "$backup"
sha256sum server internal/server/server.go
