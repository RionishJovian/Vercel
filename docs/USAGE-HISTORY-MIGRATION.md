# Voucher usage history migration

Adds `guest_voucher_usage`: the latest router-observed time and data used per
guest Internet voucher. It is additive, idempotent and operator-run, like the
other files in `migrations/`. **Nothing in this change has been applied to
production.**

## What it changes
- New table `guest_voucher_usage` (tenant row-level security, forced) with a
  composite foreign key to `vouchers(id, business_id)`; counters are
  non-negative and never move backwards (`GREATEST` on upsert).
- No existing table or column is altered. No existing row is touched.

## Release safety
The application checks `to_regclass('public.guest_voucher_usage')` before using
the table. Without the migration, sync behaves exactly as before and the Stock
page shows live usage only. With it, each sync stores the counters (inside a
savepoint, so a storage problem can never block activation or expiry sync) and
the Stock page also shows the last saved reading with its time.

## Apply (operator, with a fresh backup)
1. Back up the database.
2. `psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/20261101120000_guest_voucher_usage.sql`
3. Recommended: rerun `deploy/runtime-db-role.sql` so the runtime role receives
   its service policy on the new table (tenant transactions already work
   without it).
4. Verify: `SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname='guest_voucher_usage';` returns `t | t`, and after a sync with an active voucher the table has a row for it.

## Roll back
`DROP TABLE guest_voucher_usage;` — nothing else references it. The application
falls back to live-only usage automatically.

## Tested
`TestDBGuestVoucherUsageKeepsCountersAndIsolatesTenants` and
`TestGuestVoucherAccountingStoresUsageOnlyWhenMigrationApplied` run against a
real PostgreSQL 17 (apply twice, no regression of counters, tenant isolation,
sync works before and after the table exists).
