# Historical migrations

These files are history, kept for reference and because some Go test harnesses load individual files. Most were written for the retired Supabase setup and do not run on a clean database.

The production schema is `db/baseline.sql`. New changes go in `db/migrations/`. See [docs/MIGRATIONS.md](../docs/MIGRATIONS.md). Do not add files here.
