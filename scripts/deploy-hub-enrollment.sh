#!/usr/bin/env bash
# Run on mikromagic-hub after staging and mock-route validation. No peer writes.
set -euo pipefail
stage=${1:?staged directory required}
expected=${2:?expected current service SHA256 required}
test "$stage" = /opt/mikromagic-hub/stage.qoKEO7
service=/opt/mikromagic-hub/service.mjs
test "$(sha256sum "$service" | awk '{print $1}')" = "$expected"
node --check "$stage/service-staged.mjs"
node --check "$stage/hub-enrollment.mjs"
backup=$(mktemp -d /opt/mikromagic-hub/rollback-enrollment.XXXXXX)
chmod 700 "$backup"
cp -p "$service" "$backup/service.mjs"
sudo cp -p /var/lib/mikromagic-hub/peers.json "$backup/peers.json"
sudo chmod 600 "$backup/peers.json"
before=$(sudo wg show wg0 allowed-ips | sha256sum | awk '{print $1}')
store_before=$(sudo sha256sum /var/lib/mikromagic-hub/peers.json | awk '{print $1}')
install -m 644 "$stage/hub-enrollment.mjs" /opt/mikromagic-hub/hub-enrollment.mjs
install -m 644 "$stage/service-staged.mjs" "$service"
sudo install -d -m 755 /etc/systemd/system/mikromagic-hub-peer.service.d
# This non-secret drop-in adds the app return route, not a default/LAN route.
printf '[Service]\nEnvironment=MM_HUB_APP_ROUTE=10.77.0.9/32\n' | sudo tee /etc/systemd/system/mikromagic-hub-peer.service.d/98-enrollment-route.conf >/dev/null
rollback() {
  install -m 644 "$backup/service.mjs" "$service"
  sudo mv /etc/systemd/system/mikromagic-hub-peer.service.d/98-enrollment-route.conf "$backup/98-enrollment-route.disabled"
  sudo systemctl daemon-reload
  sudo systemctl restart mikromagic-hub-peer.service
}
sudo systemctl daemon-reload
if ! sudo systemctl restart mikromagic-hub-peer.service || ! sudo systemctl is-active --quiet mikromagic-hub-peer.service; then rollback; exit 1; fi
after=$(sudo wg show wg0 allowed-ips | sha256sum | awk '{print $1}')
store_after=$(sudo sha256sum /var/lib/mikromagic-hub/peers.json | awk '{print $1}')
if [ "$before" != "$after" ] || [ "$store_before" != "$store_after" ]; then
  echo 'Unexpected peer drift; no peer-store rollback attempted.' >&2; rollback; exit 1
fi
# Firewall changes need separate, explicit approval. This rollout never adds
# forwarding permissions; management verification fails closed when blocked.
printf 'Hub code deployed; existing peers and firewall unchanged. Rollback: %s\n' "$backup"
sha256sum "$service" /opt/mikromagic-hub/hub-enrollment.mjs
