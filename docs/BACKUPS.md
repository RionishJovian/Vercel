# Backups and recovery

## What exists
| What | Where | When | Checked by |
|---|---|---|---|
| Nightly database dump (`pg_dump -Fc`) | `/var/backups/megamagic/mega_magic-<time>.dump` on the production server, mode 600 | 03:10 UTC, `mm-db-backup.timer` | the script itself: the dump must read back, contain tables and not be less than half the size of the previous one; otherwise it is discarded, the old dump stays and the failure is logged (`journalctl -t mm-db-backup`) |
| Pre-deploy dump (plus compose and config copies) | `/root/mm-rollout-backups/ci-<sha>-<time>/` | every production deploy | the deploy script (restore list must be non-empty) |
| Weekly restore drill | throwaway container, nothing kept | Sundays 04:30 UTC, `mm-restore-drill.timer` | restores the newest dump with `--exit-on-error` and compares structure and every table's row count with production (`journalctl -t mm-restore-drill`) |
| Whole-server snapshot | Vultr backups, if enabled on the instance | per Vultr schedule | confirm in the Vultr dashboard; not verified by the app |

Retention: 14 days of nightly dumps. **All of the above lives on the production server itself** (except an enabled Vultr snapshot). An off-server copy of the dump is still to be set up; until then the server disk is a single point of failure for the dumps.

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

## Open items
Off-server copy of the dump (encrypted, to storage you control) and confirming the Vultr snapshot schedule.
