# MegaMagic continuation: verified local handoff

Date: 2026-10-21 (Asia/Yangon). Scope: local continuation, not deployment.

## Resume point

The user asked to carry on from the review of Hermes's screenshot/report, after
setting up Hermes Desktop's project environment knowledge and persistent memory.
The setup phase writes project guidance and memory, not application fixes.

Setup is complete: Hermes default-profile project `megamagic-local` is registered
with this checkout as its primary folder. Persistent project/user memories were
saved through Hermes's memory tool. A fresh Desktop session opened in this folder,
read the three project documents, and correctly reported the build/source split,
commit dependency gap and Docker limitation. Native context/memory loading checks
all passed. Older notes were preserved with dated scope corrections.

Continue `/Users/aces/123/mega-magic-implementation`, origin
`RionishJovian/Mega`, branch `feature/access-tier-user-role-separation`, observed
HEAD `b5e7c0c7684c9e16f07851567101a85dde32fa90`. Recheck before work.
Read `AGENTS.md` and `docs/HERMES-BUILD-CONTEXT.md`.

## What the review established

- The current mixed working tree passed all 13 critical UI test files.
- `git diff --check` was clean; production `web/app.html` inline JavaScript
  parsed with zero syntax errors in both the working tree and committed HEAD.
- Commit `b5e7c0c` contains exactly `web/app.html`,
  `scripts/test-team-ui.mjs`, and `scripts/test-incidents-ui.mjs`.
- In an isolated export of that commit, `npm run test:critical-ui` failed because
  the command is absent from committed `package.json`.
- Even when supplied the existing local dependencies, BOTH new test files failed:
  incidents exposed a VIEWER action; team still treated OPERATOR as grantable.
- Role pruning in `internal/model/user.go`, matching UI changes and incident
  route permission changes are uncommitted. So are the test command, lockfile,
  `jsdom` declaration and other supporting test files. The three-file commit is
  not independently ready for merge.
- `npm run build` passed in the isolated export using existing local dependencies,
  but builds the separate React/Sites prototype. It does not validate the Go/web
  application and is not a clean dependency installation test.
- At setup start there were 101 status entries, including substantial existing
  workflow-revamp work. This is a snapshot count, not an expected invariant.
- Local Go model/auth tests and the actual `cmd/server` build passed during setup.
  Full Go, database, browser, production and hardware acceptance are not claimed.

## Next application task after setup

1. Recheck source identity, diff and any work done by other sessions.
2. Trace the role/tier change's required dependencies across UI, Go RBAC, incident
   handlers/routes, migration and tests. Check intended policy before changing a test.
3. Separate these dependencies from unrelated workflow-revamp work. Make coherent,
   reviewable commits only after inspecting exact staged hunks; preserve all other
   work. Do not blanket-stage or revert the mixed tree.
4. Verify the resulting committed source in isolation. Include its package scripts,
   dependency declarations/lockfile and required tests. Record actual outputs and
   skips, validate the Go API and production HTML separately from the prototype.
5. Prepare the review/handoff. This local setup does not authorize pushing, merging,
   production migrations, deployment, customer-router changes or external messages.

Older `~/.hermes/Triage/project-state.md` live claims are historical reports.
They do not resolve current release/source identity or supply fresh production proof.

## 2026-10-21 continuation: role-vs-tier change completed and committed

Branch `feature/access-tier-user-role-separation`, observed origin
`RionishJovian/Mega`. New local commits on top of b5e7c0c (not pushed):

- `d153af2` rbac: prune OPERATOR/TECHNICIAN/SUPPORT; complete role-vs-tier
  dependency set. Grantable tenant roles are OWNER/ADMIN/VIEWER (DEVELOPER
  platform-only). Includes model pruning, the seven RoleOperator route
  gates moved to RoleAdmin, incident acknowledge/resolve + topology reboot
  gated at RoleAdmin, the tenant-team platform-account guard, web/app.html
  role hunks, init.sql CHECKs, the prune-roles migration,
  `test:critical-ui` + jsdom (vite 6.4.4 rides in the committed lockfile).
- `2e6b5c4` test: align three stale critical-UI suites with committed
  production content (hub wizard labels stale since `5cd88b6`, deploy.sh
  asset-discovery versioning stale since `d038d3b`, guest-voucher badge
  stub). They failed identically in an isolated export of b5e7c0c, so the
  failures pre-date the role work.
- `f074134` test: audit-web-ui scans scripts referenced by the HTML
  surfaces, so committed media.html button hooks resolve.

Validated in an isolated export of `f074134` with the existing local
node_modules (not a clean npm install): `test:critical-ui` 10/10 pass,
including b5e7c0c's team/incidents suites; `audit-web-ui` passes all 6
surfaces including inline-script syntax; `go test -count=1` ok for
internal/model, internal/auth, internal/handler, internal/server,
internal/database (56 database tests SKIP without MM_TEST_DB_URL/MM_TEST_DSN
— no live integration coverage claimed); `go build ./cmd/server` OK;
`npm run build` produced dist/client/index.html, dist/server/index.js and
dist/.openai/hosting.json; `git diff --check` clean.

Uncommitted workflow-revamp work is preserved untouched (87 status entries
remain): hub enrollment-capability, guest sessions, discovery labels,
topology site filter, backups UI, megamagic.tech domain migration, deploy
restore files and revamp test/tool versions. The committed package.json
intentionally omits `scripts/test-current-hub-contract.mjs` and the `.venv`
test:api change; those belong to the revamp commits.

Next: review these three commits; then separate the workflow-revamp work
into coherent commits of its own. No push, PR, merge, production migration,
deployment or customer-router action is authorized by this local session.

## 2026-10-21 continuation: role-consolidation database fixes (Codex review)

Commit `792ab4b` (on top of a3b2e87) fixes the two committed-HEAD review
findings plus a third instance found by sweeping the documented import
pipeline:

- `migrations/20261018180000_prune_roles_oper_tech_supp.sql` now reassigns
  existing OPERATOR/TECHNICIAN/SUPPORT rows to ADMIN (the documented
  consolidation target) across users, business_memberships,
  business_invitations and both role-default tables BEFORE enforcing the
  stricter CHECK constraints, touches only the role column (tenant
  associations preserved), and wraps the whole script in BEGIN/COMMIT so
  the documented `psql -v ON_ERROR_STOP=1` path — and a plain autocommit
  run, and any single-connection runner — rolls back completely on failure.
- `scripts/migration/sql/120_entitlements_features.sql` emitted OPERATOR
  into both role-default tables; now emits ADMIN.
- `scripts/migration/run_phase1.sh` computed `new_role='OPERATOR'` for
  site-less agents; now ADMIN. A repo-wide sweep found no other producer
  of removed roles.
- New `internal/database/role_prune_migration_integration_test.go` covers
  the old-role upgrade (mapping, tenant boundaries, supported roles stay
  valid, removed roles rejected afterwards), full rollback of a failed
  migration, and the legacy entitlements import against the pruned schema.

Verification actually run (disposable local PostgreSQL 17.11 on
127.0.0.1:5544 — Homebrew binaries, trust auth + fixture password;
production databases were never contacted):

- New focused tests via the Go harness: 3/3 PASS.
- psql repro of the reported failure (sabotaged final ADD CONSTRAINT, plain
  autocommit, no ON_ERROR_STOP): migration fails, prints ROLLBACK, and the
  OPERATOR row + old users constraint are byte-identical afterwards — the
  preceding DROP CONSTRAINT no longer leaks. Success path with
  ON_ERROR_STOP=1: OPERATOR/TECHNICIAN/SUPPORT users and invitations become
  ADMIN, VIEWER untouched, a fresh OPERATOR insert is rejected.
- Working tree, harness active: `go test -count=1 ./internal/...`
  580 PASS / 0 FAIL / 17 SKIP; `npm run test:critical-ui` 13/13;
  `audit-web-ui` pass; `npm run build` artifacts present; `go build
  ./cmd/server` OK; `git diff --check` clean.
- CLEAN EXPORT of `792ab4b` with FRESH `npm ci` (no reused node_modules):
  `test:critical-ui` 10/10; `audit-web-ui` pass; `npm run build` artifacts
  present; `git diff --check` clean; `go build` OK;
  `go test -count=1 ./internal/...` 564 PASS / 3 FAIL / 20 SKIP.

The 3 export failures are PRE-EXISTING, proven by running them identically
at the parent `a3b2e87`: `TestDB_PortalDevice_isolation`,
`TestDB_PortalDeviceAnchored_isolation` and
`TestGuestWifi_approveGrantsExtensionOnce` all fail with
`column "media_host" does not exist` — committed tests reading
`portal_configs.media_host`, a column that exists only in the UNCOMMITTED
workflow-revamp init.sql hunks. They are the revamp workstream's to resolve
(commit the init.sql media columns together with those tests). The same
export run also exposed a committed-tree DSN quirk: `database.New` builds
an unquoted keyword DSN, so an empty password makes lib/pq drop the dbname
and the connection fails; the UNCOMMITTED `quoteConnectionValue` fix in
`internal/database/db.go` addresses it. Fixtures avoid it by using a
passworded URL (as production DSNs have).

Next resume point: review `792ab4b`; then land the workflow-revamp commits
which must include the init.sql portal-media columns and the db.go DSN
quoting fix so the three pre-existing failures reconcile. No push, PR,
merge, production migration, deployment or customer-router action is
authorized by this local session.

## 2026-10-21 continuation: portal-media baseline fix

Commit `0748ecd` resolves the three database-enabled failures Codex
reproduced at a3b2e87/03d5aa4 (`TestDB_PortalDevice_isolation`,
`TestDB_PortalDeviceAnchored_isolation`,
`TestGuestWifi_approveGrantsExtensionOnce`).

Root cause: the committed PortalConfigRepository/portal handlers already
read/write `portal_configs.media_host`/`media_port`, and the tracked
`migrations/20261004130000_portal_media_server.sql` already upgrades
existing databases — but the fresh `database/init.sql` baseline never
received the columns. The migration half existed; only the fresh-schema
half was missing, so no new migration was needed.

- init.sql: the two media columns and their CHECK constraints, byte-
  identical to the migration's contract (private IPv4 only, numeric port
  1-65535, default 8096). Only this hunk is committed; the uncommitted
  device_discovery_labels hunk stays with the revamp work.
- New `portal_media_migration_integration_test.go`: an existing-schema
  upgrade fixture (pre-media baseline rebuilt from init.sql, seeded
  portal settings, tracked migration applied) proving defaults land,
  every pre-existing setting is preserved, the CHECKs match, the
  migration is idempotent and the real repository round-trips media
  values; plus a fresh-schema fixture carrying the same guards.

Clean-source verification (fresh `npm ci` export of `0748ecd`, disposable
PostgreSQL 17.11; production databases never contacted):

- `go test -count=1 ./internal/...` with the MM_TEST_DB_URL harness:
  569 PASS / 0 FAIL / 20 SKIP. The three target tests, the role-prune
  and legacy-import regressions and tenant isolation all PASS.
- The 20 skips were then resolved or explained: the 4 MM_TEST_DSN tests
  (device-credential source, pricing promo shape, migrated-promo guard)
  PASS 4/4 against a correctly initialized disposable fixture (schema +
  seeded pricing_promo + unconfigured device); all 11
  internal/media/bridge tests (including the 8 Postgres ones) PASS with
  MM_MEDIA_TEST_DSN pointing at a disposable loopback `mm_media_test`
  database (the URL needs `?sslmode=disable` — lib/pq defaults to SSL).
  The remaining 8 skips are the router lab/live/hardware suites that
  require a physical disposable router; no disposable database can run
  them.
- `test:critical-ui` 10/10; `audit-web-ui` pass; `npm run build` produced
  dist/client/index.html, dist/server/index.js, dist/.openai/hosting.json;
  `go build ./cmd/server` OK; `git diff --check` clean.
- Working tree equivalents: Go 582 PASS / 0 FAIL / 17 SKIP (the 3 target
  tests included), `test:critical-ui` 13/13, audit pass, build OK.

db.go DSN quoting: inspected per the task. It does NOT block these
fixtures (they use passworded URLs; only passwordless URLs misparse), so
per instruction it is documented rather than fixed here: the committed
`database.New` builds an unquoted keyword DSN, an empty password makes
lib/pq drop the dbname, and the harness then silently skips. The
uncommitted `quoteConnectionValue` fix plus its
`connection_integration_test.go` (gated on MM_TEST_TRUST_AUTH) remain in
the working tree for the revamp commits.

Next resume point: review `0748ecd`; the committed tree is now green
against disposable fixtures with only live-router suites skipped. The
remaining work is the workflow-revamp commits (84 uncommitted entries,
including the db.go DSN fix, init.sql device_discovery_labels hunk, hub
capability, guest sessions, backups UI and the domain migration). No
push, PR, merge, production migration, deployment or customer-router
action is authorized by this local session.
