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

## 2026-10-21 Codex continuation: connection and browser reliability completed

The owner asked Codex to continue the interrupted app task after Hermes hit its
provider budget limit. Hermes configuration work was stopped; this continuation
is local application work only.

- Hermes had already committed `8d09874`: lib/pq keyword connection values are
  quoted, including empty passwords and literal whitespace, apostrophes and
  backslashes. The dedicated disposable trust-auth connection regression is
  committed with it.
- Codex completed and committed `32281ad`: the synthetic media editor provides
  synchronous confirmation answers, avoiding the headless Chrome native-dialog
  race. The test asserts the warning text and both outcomes: Cancel sends no
  DELETE and preserves the publication; Confirm issues exactly one DELETE and
  withdraws the page. Unexpected real dialogs, runtime exceptions and CSP errors
  still fail the test. The browser teardown retains Hermes's bounded SIGKILL
  escalation for a child that fails to exit after SIGTERM.

Verification used a clean git archive of `32281ad` and fresh `npm ci`; it did
not reuse the mixed working tree's dependencies or uncommitted source:

- Media browser test: 5/5 consecutive standalone runs passed in the clean
  export, after 5/5 initial working-tree runs.
- Full critical UI suite: 10/10 passed on each of 3 consecutive clean runs.
- Web audit: all 6 HTML surfaces passed. Prototype build and actual Go server
  build passed. The prototype is still separate from the Go/web application.
- Disposable PostgreSQL, passwordless `MM_TEST_DB_URL` and explicit
  `MM_TEST_TRUST_AUTH=true`: internal Go suite 570 top-level PASS / 0 FAIL /
  20 SKIP, plus 444 nested subtest passes. The connection parsing regression
  executed, including empty and injection-shaped password values.
- Correctly seeded disposable `MM_TEST_DSN`: credential/pricing tests 4/4 PASS.
- Disposable `MM_MEDIA_TEST_DSN`: media bridge package 11/11 PASS, including
  its 8 PostgreSQL tests. These runs resolve 12 of the initial 20 skips;
  the remaining 8 are the explicitly gated router lab/CHR/live/hardware tests.
- Actual SCRAM-SHA-256 password authentication, using a synthetic password
  containing whitespace, an apostrophe and a backslash: portal-media upgrade
  and fresh-schema tests plus role migration/import regressions 5/5 PASS,
  with no skips.
- Whitespace checks passed. All unrelated tracked dirty file contents matched
  the hashes captured before the continuation. The private PostgreSQL cluster
  was stopped successfully.

Evidence is under
`/Users/aces/Documents/Codex/2026-10-21/asd/work/app-continuation-32281ad/`.
This is local verification, not production migration, deployment or physical
router acceptance. No push, PR, merge or deployment occurred.

Next resume point: the interrupted two-fix task is complete. Recheck the mixed
tree (82 status entries at this snapshot), then separate the remaining workflow
revamp into coherent dependency-complete commits. Preserve existing work and
the app/data/auth/tenant/Hub boundaries. Remaining groups include device
discovery labels, Hub capability/enrollment, guest sessions, backups UI,
navigation/layout and domain/deployment restoration work; inspect dependencies
before choosing a group. Publishing, production migrations, deployments and
customer-router actions remain outside this local task's scope.

## 2026-10-21 owner priority: router-to-guest internet workflow

Read `docs/INTERNET-ACCESS-PRIORITIES.md` before selecting the next task. The
owner prioritized router add/connection, quick/basic configuration, guest
voucher batches, voucher revenue, then Internet Hotspot captive-portal access.

`0747795` fixes a reproduced one-paste enrollment retry failure. Keys are kept
only in a synchronized, bounded, tenant/device-scoped 30-minute memory cache
before calling Hub. Retries recover the matching one-paste script after uncertain
Hub responses or management-preparation errors. Existing tunnels are never
silently rekeyed. Expiry/restart still requires explicit recovery. Hub responses
with a foreign business/router identity are rejected before management preparation.

The regression tests failed before the fix (lost artifact / replacement key),
then passed with race detection. They cover failures before/after Hub reservation,
management preparation failure, concurrent retries, key/public-key correspondence,
tenant separation, foreign identities, expiry and bounded memory.

A clean export of `0747795` with fresh npm dependencies passed:

- Focused router/Hub race tests: 10 top-level + 10 nested passes, no skips.
- Internal Go suite on disposable PostgreSQL: 576 top-level + 448 nested passes,
  zero failures, 20 skips. Separate credential/pricing tests 4/4, media bridge
  11/11 and SCRAM-authenticated portal/role tests 5/5 passed. The remaining eight
  skips require router lab/live/hardware conditions.
- Critical UI 10/10, six-surface web audit, prototype and Go server builds,
  whitespace checks. Existing unrelated dirty content hashes were preserved.

Evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/router-priority-0747795/`.

The existing signed-in nish@magic app account contains `hAP ax2 lab`, assigned to
its lab site at tunnel address 10.77.0.8. A read-only current Hub check reported
no recent/observed handshake. The owner confirmed the hardware is currently
unavailable. No site assignment, peer, physical router, production database or
deployment was changed. Physical connection, RouterOS version and guest internet
acceptance remain unverified. Continue local work in the newly recorded order.

## Quick Config recovery continuation — 2026-10-21

The next priority exposed a cancellation/rollback defect in
`internal/provisioner/routeros/quick_config.go`. A partial write followed by
request cancellation previously attempted recovery with the cancelled context.
The verification-mismatch error also claimed restoration when rollback failed,
and command success alone was treated as restored without readback.

Recovery now uses an independent 30-second context retaining request values,
reads back the prior identity/DNS values and marks `rolled_back` only after a
match. Both the original failure and any rollback failure remain in the error.
The new regression tests failed before the fix and passed after it. Clean-source
verification (HEAD export plus the two task-owned Go files) passed 119 RouterOS
top-level tests with race detection, zero failures and eight gated hardware/lab
skips; the Go server compiled. Evidence is under
`/Users/aces/Documents/Codex/2026-10-21/asd/work/quick-config-recovery/`.

No production deployment or router operation occurred. Preserve the mixed tree.
Continue with saved-router reuse and basic LAN/DHCP/Hotspot setup, followed by
guest internet voucher batches. Hardware acceptance remains pending while the
owner's lab router is unavailable.

## Basic setup controls continuation — 2026-10-21

Initial LAN/DHCP/Hotspot setup in `web/app.html` now independently restores valid
saved gateway context (or the single gateway), tolerates disabled browser
storage and never guesses port roles. The target and fields lock during apply,
review tokens are consumed once, duplicate submissions are blocked, and changed
inputs/selections invalidate review. Only literal `verified: true` reports
success. Stale inventory and detached-form results are ignored.

Nine new DOM regressions passed, including cancellation, failed/unverified
apply, review races, authorized selection, read-only access and storage failure.
Isolated HEAD plus the task-owned app changes/new test, with fresh npm ci, passed
all 19 critical UI entries and the real Chromium initial-network acceptance
(including synthetic apply and 390px layout). The mixed tree passed 22 entries.
Evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/gateway-setup/`.

Continue with guest internet voucher batch issuance/printing. No deployment or
physical router operation occurred, and physical guest-network acceptance is
still pending. Preserve unrelated mixed-tree work when staging the next group.

## Guest voucher printing continuation — 2026-10-21

The basic setup fix is committed as `eff1cd4`. The next local slice fixes guest
voucher batch printing in `web/app.html`. Issuance retains the exact returned
batch as the default print source, including its response data allowance, and
keeps it available if the inventory refresh fails. The operator can explicitly
switch to loaded inventory. Fresh usage/expiry status updates retained IDs;
older inventory and preview samples do not leak into that batch's printout.

Tiles now print current `price_mmk` values (including free vouchers), the plan
and known guest time/data limits. Preview and print use matching border boxes,
fit readable type or omit details with a notice, reserve A4 cut gaps/padding,
and allow thermal strips to grow for dense rows. A view failure after successful
issuance cannot offer a replacement submit. This is a temporary page selection,
not persistent batch history; inventory rows without data allowance do not
invent it.

Six new DOM regressions passed. Clean HEAD plus only this slice, with fresh npm
ci, passed 25 critical UI entries; the mixed tree passed 28. Six-surface audit
and real Chromium synthetic acceptance passed, including 105 returned codes,
100 + 5 disjoint print selections, price/time/data, A4/thermal fit, and 390px
layout. Evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/voucher-batch/`.
Screenshot: `/Users/aces/Documents/Codex/2026-10-21/asd/outputs/voucher-batch-preview-20261021.png`.

Next: voucher revenue reporting, keeping issued value, usage, confirmed sales
and approved portal payments distinct. No production deployment, production
voucher creation, physical printer test or router write occurred. Physical
guest internet acceptance still awaits the owner's unavailable lab router.

## Voucher activation reporting continuation — 2026-10-21

The owner confirmed the lifecycle: printing leaves codes unused; a guest enters
the purchased code on the portal, and successful activation updates value/counts
automatically. Do not introduce a separate staff "record sale" step. Report
**activated voucher value**, with approved portal payments and bank settlement
kept distinct. First-use reservation already anchors validity and client binding;
this task does not reset that deadline or allowance on failed attempts/reconnect.

`20261021090000_guest_voucher_activation.sql` adds a nullable, guarded
`activated_at`. Verified `GuestVoucherRepository.FinalizeTXN` writes it once in
the same transaction as activation; issuance/reservation and stale finalization
do not. Legacy active rows are not backfilled. The server preflight now requires
the column/guard before enabling guest voucher access. Apply the migration
before a future authorized backend rollout; do not auto-migrate or remove the
column when reverting the binary.

`GET /api/v1/voucher-report` is authenticated and RLS scoped. It aggregates the
full explicit guest-grant inventory, activation dates/value in Yangon time and
per-gateway state/value. Face value, reservations, active grants, missing verified
timestamps and historical activated value are distinct. Business passes/foreign
tenants are excluded; no cash receipt is inferred. The UI keeps voucher and
portal totals independent and exposes source/limited scope in inbox CSV export.

An isolated HEAD export plus only task dependencies, fresh npm ci and private
loopback PostgreSQL passed: 582 internal Go top-level tests, zero failures,
20 gated fixture/hardware skips; critical UI 31/31; six-surface audit; actual Go
server build; Chromium voucher reporting, portal approval/readback and 390px
layout. The mixed tree passed 34 UI entries. All private clusters stopped.
Evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/voucher-report/`.
Screenshot: `/Users/aces/Documents/Codex/2026-10-21/asd/outputs/voucher-revenue-preview-20261021.png`.

No production migration/deployment, actual payment/issuance or router write ran.
Continue with the Internet Hotspot portal flow locally; physical guest access
remains pending while the owner's router is unavailable. Preserve the mixed tree.
