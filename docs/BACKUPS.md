# Backups and recovery

## What exists
| What | Where | When | Checked by |
|---|---|---|---|
| Nightly database dump (`pg_dump -Fc`) | `/var/backups/megamagic/mega_magic-<time>.dump` on the production server, mode 600 | 03:10 UTC, `mm-db-backup.timer` | the script itself: the dump must read back, contain tables and not be less than half the size of the previous one; otherwise it is discarded, the old dump stays and the failure is logged (`journalctl -t mm-db-backup`) |
| Pre-deploy dump (plus compose and config copies) | `/root/mm-rollout-backups/ci-<sha>-<time>/` | every production deploy | the deploy script (restore list must be non-empty) |
| Weekly restore drill | throwaway container, nothing kept | Sundays 04:30 UTC, `mm-restore-drill.timer` | restores the newest dump with `--exit-on-error` and compares structure and every table's row count with production (`journalctl -t mm-restore-drill`) |
| Encrypted off-server copy (Cloudflare R2) | bucket `megamagic-db-backups`, folder `db/`, files `mega_magic-<time>.dump.age` | 03:40 UTC nightly, `mm-offsite-copy.timer` (after you run `mm-offsite-setup` once) | the script: dump must be under 30 hours old, upload size must match, failure logged and pinged; remote copies older than 30 days are pruned |
| Whole-server snapshot | Vultr backups, if enabled on the instance | per Vultr schedule | confirm in the Vultr dashboard; not verified by the app |

Retention: 14 days of nightly dumps on the server, 30 days off the server. Without the off-server copy configured, every dump lives on the production server itself, so its disk would be a single point of failure.

## Alerts
Without anything else, a failed backup or drill is only visible in the system log. To get an email when one fails **or does not run at all**, create two free checks at healthchecks.io (backup: period 1 day, grace 3 hours; drill: period 7 days, grace 1 day), copy `deploy/server/mm-monitor.env.example` to `/etc/mm-monitor.env` (mode 600) and paste the two ping URLs. The scripts ping on success and `<url>/fail` on failure.

## Install or update the tools on the server
Copy `deploy/server/` to the server and run `sudo bash deploy/server/install-backup-tools.sh`. It is safe to run again, and copies any file it replaces to `/root/mm-rollout-backups/backup-tools-<time>/`. Test the script logic anywhere with `bash deploy/server/test-mm-db-backup.sh` (also run in CI).

## Prove a backup works (any time)
`sudo mm-restore-drill` restores the newest dump into a throwaway database and prints `RESULT: PASS` only if it restored with no errors and every table matches. First drill on 2026-11-02: restore in 2 s, 78 tables, 172 indexes, 398 constraints, 122 policies, 60 functions, 21 triggers and all 374 rows identical to production.

## Recover production from a dump (runbook)
Use this if the database is lost or damaged. Read it through once before starting; the steps keep the broken database until the restored one is proven.

1. **Choose the dump.** `ls -lt /var/backups/megamagic/` (newest first). Anything after that dump's time is lost; guest vouchers and approvals made since then must be re-checked against the routers.
2. **Stop the application** so nothing writes: `cd /opt/mega-magic-go-restored-20261018T100554Z && docker compose -f deploy/compose.go-restore.yaml stop api web`.
3. **Restore into a new database next to the old one:**
   ```
   DUMP=/var/backups/megamagic/mega_magic-<time>.dump
   docker exec mega-magic-go-restored-db-1 psql -U mega_magic -d postgres -c 'create database mega_magic_restore'
   docker exec -i mega-magic-go-restored-db-1 pg_restore -U mega_magic -d mega_magic_restore --exit-on-error < "$DUMP"
   ```
   The roles `authenticated` and `mega_magic_runtime` must exist (they do on the live server; on a new server create them first, see `deploy/runtime-db-role.sql`).
4. **Check it.** Count rows in a few tables (`businesses`, `users`, `devices`) and compare with what you expect, for example `docker exec mega-magic-go-restored-db-1 psql -U mega_magic -d mega_magic_restore -c 'select count(*) from businesses'`.
5. **Swap the databases** (nothing is connected, the application is stopped):
   ```
   docker exec mega-magic-go-restored-db-1 psql -U mega_magic -d postgres -c 'alter database mega_magic rename to mega_magic_broken' -c 'alter database mega_magic_restore rename to mega_magic'
   ```
6. **Start the application and verify:** `docker compose -f deploy/compose.go-restore.yaml start api web`, then `curl -fsS https://megamagic.tech/ready` must say `ready`, and sign in.
7. **Keep `mega_magic_broken` for a few days** in case something in it is needed, then `drop database mega_magic_broken`.

To rebuild on a **new server** instead: install Docker and the compose stack, create the database user and roles, restore the dump into an empty `mega_magic` the same way, then deploy. The schema is part of the dump; the migration files are not needed (and most of them cannot run on an empty database, see the audit).

## Off-server copy: one-time setup
The dumps are encrypted with **age** before they leave the server. The **private key is never on the server**: only you hold it, so neither Cloudflare nor a leaked bucket nor a stolen copy of the server's files can read the dumps. **Without the private key the off-server copies cannot be opened, so keep the key file safe in at least two places (password manager and a USB stick or printout).**

1. **Make the key** (once, on your own computer, not on the server): `brew install age`, then `age-keygen -o ~/MegaMagic-Backup-Key.txt` and `chmod 600 ~/MegaMagic-Backup-Key.txt`. The file contains the private key and, on a comment line, the public key (`age-keygen -y ~/MegaMagic-Backup-Key.txt` prints it, starts with `age1`). Only the public key goes to the server.
2. **Create the bucket** in the Cloudflare dashboard: R2 Object Storage (Cloudflare may ask for a payment method even for the free allowance of 10 GB; this backup uses well under 1 GB) → Create bucket → name `megamagic-db-backups`, leave it private. Optional safety net: bucket Settings → Object lifecycle rules → delete objects after 35 days.
3. **Create a token limited to that bucket:** R2 Object Storage → Manage API tokens → Create API token → permission **Object Read & Write**, Specify bucket → `megamagic-db-backups` → Create. Copy the **Access Key ID** and **Secret Access Key** (shown only once) and note your **Account ID** (R2 overview page). Do not paste them into chat.
4. **On the server:** `ssh -i ~/.ssh/vultr-relay-20261102 root@207.148.76.172`, then `sudo mm-offsite-setup`. It asks for the account ID, bucket, key ID, secret (typed hidden) and the public key, saves `/etc/mm-offsite.env` (mode 600), proves the connection with a tiny encrypted test file, and enables the nightly copy. Run the first copy at once with `sudo systemctl start mm-offsite-copy.service`, then check `journalctl -t mm-offsite-copy -n 3` and that the file appears in the bucket.

## Recover from an off-server copy (the server is gone)
1. Download the newest `db/mega_magic-<time>.dump.age` from the bucket (dashboard, or `rclone`).
2. Decrypt on your computer with the private key: `age -d -i ~/MegaMagic-Backup-Key.txt -o mega_magic.dump mega_magic-<time>.dump.age`.
3. Copy `mega_magic.dump` to the new server and follow "Recover production from a dump" (step 3 onward), or "To rebuild on a new server" above.
Test this once now, not during an emergency: download one `.age` file, decrypt it, and run `pg_restore -l mega_magic.dump | head` to see the table list.

## Open items
Confirming the Vultr snapshot schedule, and the two healthchecks.io checks (backup, drill) plus a third for the off-server copy (`HEALTHCHECK_OFFSITE_URL` in `/etc/mm-offsite.env`).
