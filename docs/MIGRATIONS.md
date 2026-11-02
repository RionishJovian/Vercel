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
- New tables need row security and policies like the existing ones, and the runtime role needs grants. `up` re-runs `runtime-db-role.sql` afterwards, so grants follow automatically.

## One-time steps

- New empty database: `mm-migrate init` (applies the baseline), then `up`.
- Production, once: `mm-migrate verify-baseline`, then `mm-migrate baseline`. The first builds a scratch database from `baseline.sql` and diffs its schema against production; `baseline` refuses unless they match (`--force` overrides, do not use it casually).

## Tests

`bash deploy/server/test-mm-migrate.sh` with `MM_TEST_DB_URL` pointing at any disposable Postgres 17 (CI provides one). 57 checks cover ordering, rollback, tamper detection and the baseline logic.
