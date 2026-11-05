# Database migrations

The old `migrations/` folder (190 files, 132 of which only work on the retired Supabase setup) is history. The working chain is:

- `db/baseline.sql`: the production schema as of the version on its first line (`-- mm:baseline-version 20261102000000`).
- `db/migrations/<14-digit timestamp>_<name>.sql`: one file per change after the baseline.
- `deploy/server/mm-migrate`: the runner. It records every applied file and its checksum in `schema_migrations`.

## Everyday use

```bash
mm-migrate status     # applied, pending, changed, missing, out of order (exit 0 up to date, 2 pending, 3 problem)
mm-migrate up         # apply pending files, each in its own transaction, then re-run deploy/runtime-db-role.sql
mm-migrate grants     # re-run the runtime-role setup alone
```

Connection: `MM_DB_URL=postgres://user:pass@host/db`, or `MM_PSQL_BASE="docker exec -i <container> psql -U <user>"` with `MM_DB_NAME=<db>`.

## Rules for a migration file

- Name: `YYYYMMDDHHMMSS_short_name.sql`, lower case, newer than the baseline.
- No `BEGIN`/`COMMIT`: the runner wraps the file and its bookkeeping row in one transaction, so a failure leaves nothing half applied.
- Statements that cannot run in a transaction (`CREATE INDEX CONCURRENTLY`) put `-- mm:no-transaction` in the first lines.
- Never edit an applied file. Its checksum is stored; `status` reports `CHANGED` and `up` refuses. Fix forward with a new file.
- New tables need row security, policies like the existing ones, and explicit `GRANT`s to `authenticated`, because `baseline.sql` carries no grants and only migrations give a new database access. The runtime role needs grants too. `up` re-runs `runtime-db-role.sql` afterwards, so grants follow automatically.

## One-time steps

- New empty database: `mm-migrate init` (applies the baseline), then `up`.
- Production, once: `mm-migrate verify-baseline`, then `mm-migrate baseline`. The first builds a scratch database from `baseline.sql` and diffs its schema against production; `baseline` refuses unless they match (`--force` overrides, do not use it casually).

## Tests

`bash deploy/server/test-mm-migrate.sh` with `MM_TEST_DB_URL` pointing at any disposable Postgres 17 (CI provides one). 63 checks cover ordering, rollback, tamper detection and the baseline logic.

## Deploys

`deploy/deploy-vps.sh` ships `db/`, `runtime-db-role.sql` and `mm-migrate` with the release. After the pre-deploy backup and before the new API starts it runs `mm-migrate status` (a changed, missing or out-of-order file, or a missing baseline, stops the deploy before anything is touched) and then `mm-migrate up`. If anything later fails, the old API and web images come back; the schema stays at the new version. So a migration must keep the previous release working: add columns and tables in one release, remove or rename them in a later one.

The deploy path itself was changed without a production deploy to test it; the first deploy after this change is its first real run (the backup and automatic rollback still apply).
