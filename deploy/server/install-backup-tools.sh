#!/usr/bin/env bash
# Installs the backup tools on the production server (run as root from a checkout of this repository):
#   sudo bash deploy/server/install-backup-tools.sh
# Idempotent. Every file it replaces is first copied to /root/mm-rollout-backups/backup-tools-<time>/.
set -Eeuo pipefail
here=$(cd "$(dirname "$0")" && pwd)
bk=/root/mm-rollout-backups/backup-tools-$(date -u +%Y%m%dT%H%M%SZ); mkdir -p "$bk"
put() { # source, destination, mode
  if [ -f "$2" ]; then cp -p "$2" "$bk/$(basename "$2")"; fi
  install -m "$3" "$here/$1" "$2"
}
bash -n "$here/mm-db-backup"; bash -n "$here/mm-restore-drill"
put mm-db-backup /usr/local/bin/mm-db-backup 750
put mm-restore-drill /usr/local/bin/mm-restore-drill 750
for u in mm-db-backup.service mm-db-backup.timer mm-restore-drill.service mm-restore-drill.timer; do put "$u" "/etc/systemd/system/$u" 644; done
systemctl daemon-reload
systemctl enable --now mm-db-backup.timer mm-restore-drill.timer
echo "installed; previous files in $bk"
systemctl list-timers --no-pager | grep -E "mm-db-backup|mm-restore-drill" || true
