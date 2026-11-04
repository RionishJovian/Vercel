#!/usr/bin/env bash
# Run as root on the Hub, from a directory holding hub-reconcile.mjs and stage-hub-reconcile.mjs.
# Installs the peer reconciler. It backs up the running files first and rolls back if the
# service does not stay up. No active peer is removed: only peers the store marks removed
# or disabled leave WireGuard.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
hub=/opt/mikromagic-hub
service=$hub/service.mjs
unit=mikromagic-hub-peer.service

node "$here/stage-hub-reconcile.mjs" "$service" "$here/service-staged.mjs"
cp "$here/hub-reconcile.mjs" "$hub/hub-reconcile.mjs.staged"
mv "$hub/hub-reconcile.mjs.staged" "$hub/hub-reconcile.mjs"
node --check "$hub/hub-reconcile.mjs"
cp "$here/service-staged.mjs" "$hub/service-staged.mjs"
node --check "$hub/service-staged.mjs"

backup=$(mktemp -d "$hub/rollback-reconcile.XXXXXX")
chmod 700 "$backup"
cp -p "$service" "$backup/service.mjs"
cp -p /var/lib/mikromagic-hub/peers.json "$backup/peers.json"
cp -p /etc/wireguard/wg0.conf "$backup/wg0.conf"
wg show wg0 allowed-ips >"$backup/live-before.txt"

rollback() {
  cp -p "$backup/service.mjs" "$service"
  systemctl restart "$unit" || true
  echo "Rolled back to the previous Hub service. Backup: $backup" >&2
}

mv "$hub/service-staged.mjs" "$service"
if ! systemctl restart "$unit"; then rollback; exit 1; fi
sleep 8
if ! systemctl is-active --quiet "$unit"; then rollback; exit 1; fi

# Every peer the store marks active must still be live, and the saved file must now hold them.
if ! node --input-type=module -e '
import fs from "node:fs"; import { execFileSync } from "node:child_process";
const { parseLive, parseSaved } = await import(process.argv[1]);
const peers = JSON.parse(fs.readFileSync("/var/lib/mikromagic-hub/peers.json", "utf8")).peers;
const live = parseLive(execFileSync("wg", ["show", "wg0", "allowed-ips"], { encoding: "utf8" }));
const saved = parseSaved(fs.readFileSync("/etc/wireguard/wg0.conf", "utf8"));
const active = peers.filter((p) => p.state === "active" && p.routerPublicKey);
const missing = active.filter((p) => !live.has(p.routerPublicKey) || !saved.has(p.routerPublicKey));
if (missing.length) { console.error(`${missing.length} active peer(s) not live and saved`); process.exit(1); }
console.log(`${active.length} active peers are live and saved; ${live.size} live in total.`);
' "$hub/hub-reconcile.mjs"; then rollback; exit 1; fi

journalctl -u "$unit" -n 5 --no-pager | grep -E "reconcil" || true
echo "Hub reconciler installed. Rollback copy: $backup"
