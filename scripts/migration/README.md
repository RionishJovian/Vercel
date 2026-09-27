# Mikromagic → Mega Magic data migration

One-way ETL that carries the **old Mikromagic** data into **Mega Magic**'s schema.

The old product is **single-tenant** (the tenant key is the owner's `auth.users.id`); Mega Magic
is **multi-tenant**. Everything else in this folder exists to bridge that one difference.

## Run it

```bash
docker compose up -d saas-db            # target Postgres must be running
./apply_ported_schema.sh                # (only after a schema change) rebuild the ported schema
./run_phase1.sh                         # rebuilds the copy, migrates, then self-verifies
```

Read-only against the source. The target database is **dropped and recreated from the
baseline on every run**, so it is safe to repeat and the baseline is never modified.
The script refuses to run if `TARGET_DB` equals `BASELINE_DB`, and it exits non-zero if the
Phase 3 verification fails, so a broken run can never look like a good one.

Override anything via environment:

| Variable | Default | Meaning |
|---|---|---|
| `SRC_CONTAINER` / `SRC_USER` / `SRC_DB` | `supabase-db` / `postgres` / `postgres` | old Mikromagic database (read-only) |
| `DST_CONTAINER` / `DST_USER` | `mega_magic_db_dev` / `mega_admin` | container holding the new databases |
| `BASELINE_DB` | `mega_magic_dev` | pristine seed, never written to |
| `TARGET_DB` | `mega_merge` | working copy, rebuilt each run |

CSV exports land in `csv-out/` (gitignored). Handy for eyeballing or re-loading without re-exporting.

## The scripts

| Script | Purpose |
|---|---|
| `run_phase1.sh` | the whole ETL: rebuild copy → migrate → clean residue → reconcile → verify |
| `apply_ported_schema.sh` | re-applies the `-- PHASE 2` section of `database/init.sql` to the baseline. **Required after any schema change**, or the copy is rebuilt without the new tables. Drops the ported objects first, because `CREATE POLICY` is not idempotent and `ADD COLUMN IF NOT EXISTS` silently skips the column (leaving it without its foreign key). |
| `verify_phase3.sh` | independent reconciliation — see below. Also runnable on its own. |
| `promote.sh` | builds a production database from `mega_merge` **without** the ETL scaffolding, then verifies it. See "Promotion" below. |

## The one design decision that matters

Old rows everywhere key off the owner's user id — `sites.owner_id`, `managed_devices.owner_id`,
`voucher_codes.owner_id`, `incidents.owner_id`, … Preserving that exact UUID as **both**
`businesses.business_id` and `users.user_id` means:

* every later table maps by a **direct column copy** — no lookup tables to get wrong;
* old foreign keys still resolve (`incidents.acknowledged_by` → a real migrated user);
* preserving old **device ids** keeps `incidents.device_id` valid.

## Steps

| # | Source | Target | Notes |
|---|---|---|---|
| 1.0 | — | — | retire the 4 new-side dev/test tenants |
| 1.1 | `auth.users` + `profiles` + `user_roles` | `businesses`, `users` | one business per owner account; `client`→OWNER, site-owning `agent`→OWNER, non-site `agent`→OPERATOR; bcrypt hashes copied **verbatim** |
| 1.4 | `managed_devices` | `devices` | `category`→`deployment_layer`; vendor/category/transport coerced into the new CHECK enums |
| 1.5a | `voucher_codes` | `vouchers` | `cancelled`→`revoked` (not in the new enum) |
| 1.5b | `portal_settings` + `portal_plans` | `portal_configs` | per-owner plan rows become the `guest_plans` JSONB array |
| 1.6a | `incidents` | `incidents` | old table has no `created_at` → derived from `opened_at` |
| 1.6b | `alert_rules` | `alert_policies` | `router_cpu_high`→`cpu_high`, `router_memory_high`→`memory_high` |
| 2.1–2.3 | `device_health_samples`, 6 audit tables, `service_purchases` | `telemetry_samples`, `audit_events`, `purchase_requests` | consolidated into existing empty tables |
| 2.4 | `magic_coin_*` | `magic_coin_wallets`, `magic_coin_transactions` | immutability trigger carried over |
| 2.5 | 5 unlock-key tables | `unlock_key_activations` | consolidated via a `key_kind` discriminator |
| 2.6 | `sites`, `router_connections`, `site_topology_config` | same + 2 new `devices` columns | Supabase-encrypted secrets renamed `legacy_*` |
| 2.8 | `terminal_templates`, `voucher_resellers`, `voucher_print_layouts`, `pricing_promo`, `ruijie_cloud_connections`, Magic Dude KB | same | |
| 2.9 | — | — | re-homes **platform content** owned by the excluded dev account (the 51-row built-in template library) onto the platform business |
| 2.10 | `i18n_overrides`, `owner_daily_closes`, `device_allowances`, `device_requests`, `notification_prefs`, `platform_announcement`, `platform_admins`, `platform_service_bank_accounts` | same | |
| 2.11 | `syslog_events`, `fleet_scan_runs`, `ai_usage_events`, `admin_notifications` | same | ops history |
| 3.0 | — | — | removes baseline demo/seed residue (source-only enforcement) |

The SQL lives in `sql/`, numbered in execution order, one file per step. Each inserts from a
`_stg_*` text staging table, so the mapping is all in one readable place.

## Data-quality rules applied

* **Orphans are excluded, never invented.** A row whose owner no longer exists in `auth.users`
  cannot belong to a business. This drops 2 of 9 devices and 49 of 72 alert rules.
* **But check what an excluded row IS before dropping it.** A tenant filter that shrinks a row
  count may be discarding *product content* rather than customer data — the excluded `nish@dev`
  account owned the entire 51-row **built-in RouterOS template library**. Those rows are
  re-homed onto the platform business (step 2.9); dropping them would have deleted the library
  from the product.
* **The merged copy is source-only.** It is rebuilt from the baseline each run, and the baseline
  carries the new project's own dev seed, so seed/demo rows survive into the merged dataset unless
  removed. Step 3.0 deletes any `devices` / `portal_configs` row whose key is absent from the
  source. Without it a fixture device (`hardware_name = 'portal-test'`) and a demo portal config
  remained under the platform business.
* **Unknown enum values are coerced**, not passed through — an insert can never violate a CHECK.
* **`'Neon Cafe Wi-Fi'` is treated as a placeholder.** It is the old app's demo default and appears
  for 10 of 13 owners verbatim, so `portal_configs.business_name` (and the print layout) fall back
  to the owning business's real name instead of showing it to guests.
* **Password hashes are copied byte-for-byte** and never printed; integrity is checked by comparing
  `md5()` digests, not by reading the values.
* **`connector_rate_limits` is deliberately NOT migrated.** It is transient rate-limiter state, not
  history (its newest window was 9 days stale). Carrying stale buckets over risks throttling live
  users for zero recoverable information.

## Verification

```bash
./verify_phase3.sh
```

This is deliberately **independent of the pipeline that built the copy** — the runner's own
reconciliation counts what the runner just inserted, so a shared bug looks like success. The
verifier instead:

* compares the **md5 of each table's sorted id set**, not a row count (a count cannot distinguish
  "the same 642 rows" from "642 different rows");
* derives the migrated-owner list **from source `auth.users`**, never from the ETL's own
  `_mig_accounts` staging table.

Tables where every source row belongs to a migrated owner must match the full source (`EXACT`);
tables with rows for owners who never migrated must match source restricted to migrated owners
(`FILTER`).

Expected (exit 0):

```
PHASE 3 OK — 17/17 tables match the source by id-set fingerprint
```

Ad-hoc spot checks:

```bash
N="docker exec mega_magic_db_dev psql -U mega_admin -d mega_merge"
$N -c "select user_role, count(*) from users group by 1"
$N -c "select b.company_name, count(d.device_id) devices, count(v.id) vouchers
       from businesses b left join devices d using(business_id)
       left join vouchers v using(business_id) group by 1 order by 2 desc"
```

Expected: 12 businesses · 13 users · 7 devices · 642 vouchers · 11 portal_configs · 8 incidents ·
21 alert_policies · ~9,900 rows across 50 tables, with **0** orphan references.

## Port completeness audit

The ETL carried every table's data across; the API/SPA surfaces were built feature by feature
afterwards. Audited by grepping `internal/` and `web/` for each of the 50 product tables.

**Precisely what was measured:** whether any Go file names the table. That proves a repository or
handler touches it — not that a screen displays it (several, e.g. `peer_heartbeats`, are internal
plumbing behind a view). The negative result is the reliable one.

**35 of 50 tables are referenced by code. 15 are migrated but referenced by nothing at all,
holding 74 rows.**

Referenced (35): `businesses` `users` `devices` `vouchers` `portal_configs` `portal_sessions`
`incidents` `alert_policies` `telemetry_samples` `purchase_requests` `guest_purchase_requests`
`device_config_backups` `device_change_events` `peer_heartbeats` `business_metered_quotas`
`business_telegram_bots` `nats_subjects` `webhook_endpoints` `webhook_deliveries` `audit_events`
plus this session's fifteen: `magic_coin_wallets` `magic_coin_transactions` `sites`
`router_connections` `device_allowances` `device_requests` `syslog_events` `terminal_templates`
`voucher_resellers` `voucher_print_layouts` `fleet_scan_runs` `ai_usage_events`
`terminal_history` `unlock_key_activations` `i18n_overrides`.

Unreferenced (15), by real `COUNT(*)` — the genuine gaps:

| table | rows | note |
|---|---|---|
| `owner_daily_closes` | 30 | daily reconciliation records |
| `operator_feature_role_defaults` | 20 | role→feature default matrix |
| `portal_mode_role_defaults` | 4 | portal role defaults |
| `admin_notifications` | 4 | admin notices |
| `account_entitlements` | 4 | feature entitlements |
| `site_topology_config` | 2 | per-site topology settings |
| `platform_service_bank_accounts` | 2 | payment details |
| `operator_feature_grants` | 2 | per-operator grants |
| `ruijie_cloud_connections` | 1 | Ruijie cloud integration |
| `platform_announcement` | 1 | announcement banner |
| `platform_admins` | 1 | kept as-is by design (no role change) |
| `magic_dude_solved_cases` | 1 | AI assistant knowledge |
| `magic_dude_knowledge_notes` | 1 | AI assistant knowledge |
| `pricing_promo` | 1 | blocked — see Open finding 2 |
| `notification_prefs` | 0 | empty; nothing to show |

Some are plumbing that arguably needs no screen (`operator_feature_*` and `portal_mode_role_defaults`
are config matrices, `platform_admins` is deliberately untouched), but `owner_daily_closes` is the
only remaining user-facing data with no way to see it. (`ai_usage_events`, `terminal_history`,
`unlock_key_activations` and `i18n_overrides` were built after this audit was first written.)

## Open findings — data carried over but not acted on

Recorded here because each one is a question about **intent**, not something the pipeline can
decide. Nothing below has been "fixed"; every affected row is present and unaltered.

### 1. `fleet_scan_runs` — 1,102 of 1,156 rows (95%) are the old app's crash ✅ DECIDED (data kept, labelled)

Every one contains a JavaScript runtime error persisted as if it were a scan result:

```
"subtitle": "KoZay · Cannot read properties of undefined (reading 'rest')"
```

The same string also appears in `payload->'snapshot'[].error`, under the insight title
`Router unreachable` and `max_severity = 'critical'`. The routers evidently replied — the old code
read their interface lists before throwing — so these are **not** fleet outages. For Kyaw Coffee
the 332 `critical` scans decompose into 318 crash artefacts and ~14 genuine findings.

Consequence: a raw count of `max_severity = 'critical'` massively overstates real fleet problems.
The API therefore labels rather than filters — `runtime_error` per finding and
`crash_artifact_scans` per response, drawn from `model.RuntimeErrorMarkers()` — and the console
renders them as `scan error` with a leading notice.

The genuine signal is the `Interface down` family: `CCR2004-16G-2S+ · ether9 (ether) carrier down`.

Open question: leave as labelled history (default), or archive the crash rows to a sibling table so
the scans view shows only signal.

**Decision: keep every row exactly where it is. Nothing is deleted or rewritten.**

The scans really did run, and the crash message is what the old app *recorded* — deleting it would
destroy the evidence of a real defect, and "cleaning" migrated data is indistinguishable from losing
it. The honest fix is the one already in place: the Fleet scans view leads with an amber notice
("97% of the scans below carry a crash message from the old app — the scan itself still ran; this
label is historical, not a live failure") and marks each affected row, so 1,102 bad rows are
truthfully labelled rather than silently absent.

If these rows ever become a problem in reports, the right move is a **filter** over the labelled
flag — not a delete. Any archive would be a copy, with the originals left intact.

### 2. `pricing_promo` — the promo prices are higher than the standard prices ✅ DECIDED (data left as-is)

The single row (`id boolean CHECK (id)`) describes "Grand opening gift — 30% off" yet reads:

| monthly promo | monthly standard | annual promo | annual standard |
|---|---|---|---|
| 66,500 | 35,000 | 731,500 | 350,000 |

A "30% off" promo priced ~1.9× the standard price. The annual figures are consistent multiples of
their own monthly counterparts (66,500 × 11, 35,000 × 10), so this is not a units mix-up across
rows — it looks like the two columns were **transposed** during extraction. The promo is also
expired (`ends_on` 2026-09-24) and `active = false`.

Not corrected, because guessing either way writes a wrong price.

**Decision taken:** leave the data exactly as the old system stored it. Migrating faithfully means
carrying a suspicious value, not inventing a corrected one — and inventing one in a billing table
is the more dangerous of the two. No screen was built for it either: it is one platform-wide row,
inactive and expired, so nothing a tenant can reach depends on it. Still worth a human answer
(transpose, or is "promo" something other than a discount?) before it is ever activated; a promo
priced above the standard rate would overcharge.

### 3. `voucher_resellers` — no unique index on `(business_id, code_prefix)` ✅ RESOLVED

**Resolved.** `database/init.sql` now creates
`voucher_resellers_business_prefix_unique ON (business_id, upper(code_prefix))`. The ported data
was verified to satisfy it (no duplicates) *before* the index was added, and the index is
per-business by design — the same prefix in a different business is still allowed (proven by
inserting one).

The ported table has no uniqueness constraint on the prefix, but the prefix is what keeps one
shop's voucher codes distinguishable from another's. The API enforces uniqueness per business in
the repository (`ErrResellerPrefixInUse` → 409, case-insensitive), and an UPDATE that keeps its own
prefix is deliberately allowed.

Because application-level guards can be bypassed by any other writer, a real index is recommended:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS voucher_resellers_business_prefix_key
    ON voucher_resellers (business_id, upper(code_prefix));
```

Not applied here: it is a schema change to a ported table, and the call belongs to whoever owns
`database/init.sql`.

### 4. `mega_merge` carries 48 ETL staging tables holding 10,583 rows of raw source data ✅ RESOLVED (by `promote.sh`)

`run_phase1.sh` stages the extracted CSVs into `_stg_*` text tables (and `_mig_accounts` for
identity mapping) so the transformations are readable in one place. It creates them with
`DROP TABLE IF EXISTS … CREATE TABLE …` at the start of every run and **never drops them at the
end**, so they persist in `mega_merge` afterwards:

```
staging_rows = 10,583 across 48 tables
_stg_audit_routerops = 3,410   _stg_health = 2,715   _stg_scan_runs = 1,214
_stg_syslog = 1,177            _stg_vouchers = 642   _stg_audit_routersave = 445
```

More rows than the 9,900 in the 50 product tables — this is a second, un-transformed copy of the
old system's data. `_mig_accounts` additionally holds `password_hash` with **11 populated
bcrypt hashes** for the migrated accounts.

Why it matters: the documented promotion route is
`CREATE DATABASE mega_magic TEMPLATE mega_merge`, which would copy all 48 staging tables — and that
credential column — into production.

Recommended before promoting (safe, because the runner recreates every staging table on its next
run):

```sql
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables
             WHERE schemaname = 'public' AND tablename LIKE '\_%'
    LOOP EXECUTE format('DROP TABLE IF EXISTS %I', t); END LOOP;
END $$;
```

Verify afterwards that only the 50 product tables remain, then promote. Do not run this while an ETL
run is in flight.

**Dry-run result.** The promotion route was exercised end to end against a throwaway copy
(`CREATE DATABASE mega_promotion_dryrun TEMPLATE mega_merge`, then dropped):

* the copy is **98 tables — 50 product + all 48 staging**, carrying the full 10,583 staging rows
  and the 11 `password_hash` values;
* the API was started against it (`DATABASE_NAME=mega_promotion_dryrun`) and served every ported
  feature correctly — fleet scans 354, event log 546, sites, connections, templates, resellers,
  allowances — all HTTP 200.

So **go-live works, but it would ship the staging tables and the credential column with it.**

**Addressed by `promote.sh`** (see below): it performs that promotion and then refuses to declare
success unless the result is clean. The raw SQL above is what it runs internally.

### 5. `i18n_overrides` is a GLOBAL table — and nine tables have no `business_id` at all ✅ RESOLVED

`i18n_overrides` holds UI translations. It has **no `business_id` column**, and unlike every
tenant table RLS is **disabled** on it (verified: `relrowsecurity = false`, 0 policies), so all
businesses read — and would write — the same rows.

**Nine** product tables have no `business_id`: `admin_notifications`, `i18n_overrides`,
`magic_dude_knowledge_notes`, `operator_feature_role_defaults`, `platform_admins`,
`platform_announcement`, `platform_service_bank_accounts`, `portal_mode_role_defaults`,
`pricing_promo`.

> An earlier revision of this finding listed `site_topology_config` as a tenth and stated that these
> tables have RLS disabled. **That was wrong on both counts.** `site_topology_config` has
> `relrowsecurity = true` and a policy that scopes it *transitively* — it has no `business_id`, but
> it has `site_id`, and `sites` carries the tenant:
>
> ```sql
> site_topology_config_tenant_isolation:
>   site_id IN (SELECT s.site_id FROM sites s
>               WHERE s.business_id = (current_setting('app.current_business_id'))::uuid)
> ```
>
> Verified by reading it the way the application does (`SET LOCAL ROLE authenticated` plus the
> tenant setting — note that testing as the superuser `mega_admin` proves nothing, because
> superusers bypass RLS entirely): the owning tenant sees its 1 row, a tenant owning none sees
> **0**, and a session with no tenant set **errors** rather than returning every row. That last
> point matters — the failure mode is closed, not open.

Why it matters for the port: every other ported feature follows the same shape (rows belong to a
business, reads and writes go through `runInTenant`). Copying that shape onto `i18n_overrides`
would have produced an endpoint where one supplier's OWNER could rewrite the interface strings
for every other supplier — a cross-tenant write with no error to signal it.

What was done instead: the read route is open (browsing translations is harmless), and the three
write routes carry the **exact DEVELOPER gate** (`auth.RequireRole`, the same gate as the
`/platform` group) rather than `RequireMinRole(ADMIN)`. Verified with real tokens: OWNER, ADMIN,
OPERATOR and VIEWER all receive **403** on POST/PUT/DELETE, DEVELOPER receives 201/200/200, and a
direct POST that bypasses the UI returns `403 insufficient permissions`.

**Decision: the nine are global by design, and no `business_id` should be added.** They are
platform-staff, platform-config and platform-broadcast data — `platform_admins` (who runs the
platform), `platform_service_bank_accounts` (where the platform is paid), `platform_announcement`
and `admin_notifications` (broadcasts), `i18n_overrides` (interface strings),
`magic_dude_knowledge_notes` (the AI's knowledge base), `pricing_promo` (platform price list).

The decisive evidence is `operator_feature_role_defaults`: it looks tenant-ish, but it is the
*defaults* half of a defaults-and-overrides pair, and the overrides half is properly tenant-scoped —
`operator_feature_grants` carries `business_id`, has RLS **enabled**, and holds the per-operator
grants. Platform-wide defaults with per-tenant overrides is the correct shape; giving the defaults
table a `business_id` would fork one platform policy into twelve divergent copies. Verified:

```
operator_feature_role_defaults   rls=false  20 rows   (platform defaults)
operator_feature_grants          rls=true    2 rows   (tenant-scoped overrides)
```

Adding a tenant column here would not merely be unnecessary — it would change behaviour for
existing rows, since the current data is single-tenant and every business would inherit a copy.

### 6. `owner_daily_closes` — no unique constraint on `(business_id, business_day)` ✅ RESOLVED

**Resolved.** `database/init.sql` now creates
`owner_daily_closes_business_day_unique ON (business_id, business_day)`. Verified by attempting a
duplicate close, which the database rejects with
`duplicate key value violates unique constraint "owner_daily_closes_business_day_unique"`.

The table is a cash-reconciliation ledger: one row should exist per business per day. The migrated
data satisfies that (30 rows, 30 distinct `(business_id, business_day)` pairs, spread over 5
businesses × 6 days from 2026-09-13 to 2026-09-18), but **nothing in the schema enforces it** —
there is no unique constraint or unique index on those two columns. A second close for the same day
would be accepted, and for a money ledger that means the day could be counted twice.

Same class as finding 3 (`voucher_resellers`): a rule the application must currently uphold itself.

```sql
-- what should exist
CREATE UNIQUE INDEX owner_daily_closes_business_day_key
    ON owner_daily_closes (business_id, business_day);
```

Not applied here, for the same reason as finding 3 — it is a schema change to a ported table.

**Also worth knowing: `counted_cash_mmk` is `0` in all 30 rows.** That is not corruption. The
auto-close note reads *"Counted cash = recorded voucher sales for the day"*, and no voucher was
redeemed in that window (`vouchers.first_seen_at` has zero rows between 2026-09-13 and 2026-09-18;
only one voucher was even created). So zero is the correct answer for every one of those days, and
the ledger simply has nothing to show yet. Any screen over this table will look empty until real
redemptions happen.


## Promotion

`promote.sh` builds a production database from `mega_merge` without the ETL scaffolding and
verifies the result before reporting success.

```bash
./promote.sh                                    # target: mega_magic
TARGET_DB=mega_promotion_check ./promote.sh     # any other target
```

It refuses to write to `mega_merge` or the baseline, refuses to overwrite an existing target unless
`FORCE=1`, and refuses to start while anything is connected to the source. It then:

1. creates the target from `mega_merge`;
2. drops every `_`-prefixed table;
3. verifies — and **fails loudly** rather than reporting success — unless the target has the same
   product-table count as the source, zero staging tables, **no `password_hash` column anywhere
   except the product `users` table**, and every product table row-for-row identical.

Validated end to end against a throwaway target: 50 product tables, 0 staging tables, no stray
credentials, every table identical — and the API started against the result
(`DATABASE_NAME=<target>`) served every ported feature, confirmed by reading `pg_stat_activity`
to check which database the server had actually attached to.

**Switch the API with `DATABASE_NAME=<target>`.** That works because `config.Load` sets
`viper.SetEnvKeyReplacer(".", "_")`, so `database.name` is overridable as `DATABASE_NAME`.

### Promotion has been executed

`mega_magic` now exists and `config.yaml` points at it (`database.name: "mega_magic"`), so the
application serves the merged data. Before this, the app pointed at `mega_magic_dev` — the empty
baseline (5 businesses, 0 vouchers, 0 syslog, 0 scanned devices) — which meant every merged row was
invisible to the running product no matter how many features were ported.

Verified against the running app: signed in as **Kyaw Coffee**, the Event log reads
"Showing 1–50 of **546** event(s)", Fleet scans totals **354**, AI usage **82**, vouchers **81** —
each equal to `mega_magic`'s own count for that tenant and 0 in the baseline. `pg_stat_activity`
confirmed the server had attached to `mega_magic`.

> Note when reading counts: these endpoints are **tenant-scoped**. They total 0 for a tenant with no
> telemetry (e.g. the seed business "Magic Development") — that is correct isolation, not missing
> data. Only `i18n_overrides` is global and returns the same 8 rows to everyone.

**To revert:** set `database.name` back to `"mega_magic_dev"` in `config.yaml` — one line, nothing
else is touched.

`config.yaml` is **gitignored** (it holds the database password), so changing it alone would have
promoted the merged data on one machine only — and a fresh deploy would have gone on serving the
empty baseline, silently losing every migrated row. The default was therefore changed in the two
places that actually ship:

* `internal/config/config.go` — `SetDefault("database.name", "mega_magic")`
* `config.yaml.example` — the tracked template

Verified by starting the API with a config containing **no** `database.name` at all: it attached to
`mega_magic` (confirmed via `pg_stat_activity`), so the merged data is the default for every
deployment rather than an accident of this working copy.

**After any later ETL change**, `mega_magic` is a snapshot and will not follow `mega_merge`
automatically. Re-promote with `FORCE=1` to rebuild it from the fresh merge:

```bash
./run_phase1.sh && FORCE=1 ./promote.sh
```

**Known, not changed:** the pre-existing `/api/v1/vouchers` list endpoint reads no `limit`
parameter and returns every voucher for the tenant (81 here). It is out of scope for the merge and
was left alone deliberately — the existing voucher screen may filter client-side, so bounding the
response could break a working screen. Flagged for a deliberate decision rather than changed
silently.

Two bugs found while building it, both caught by the script's own verification rather than by
reading it:

* `docker exec` needs `-i` to feed a heredoc to `psql`; without it psql sees EOF immediately and
  executes **none** of the script — the drop silently did nothing, and the verification is what
  noticed;
* the connection guard originally queried the source database, so the check's own connection
  counted itself and it refused to run at all. It now queries `postgres`.
