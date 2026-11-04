# Router-local broadband implementation plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task. The owner authorized implementation on 2026-11-04; continue without another planning approval.

**Goal:** Add paid, router-local PPPoE accounts and safe recovery while retaining all existing modules.

**Architecture:** Keep the existing tenant-scoped SSH transport and guarded router context. Store broadband accounts and idempotent operations separately from Hotspot vouchers, media passes, Store purchases and app subscriptions. Router-resident first-use/expiry records remain authoritative during central outages.

**Tech Stack:** Go, PostgreSQL RLS, RouterOS scripts, React/TypeScript.

**Spec:** Owner decisions in this chat: router-local auth, no cryptographic vouchers; new validity starts at first connection, early renewal extends existing expiry, late renewal starts on successful router application, expiry disconnects and preserves account.

## Global constraints
- Preserve unrelated local notes and existing Revenue, Hotspot, Jellyfin, Store, account passes and operational modules.
- Every account, operation and observation is scoped by business and router; no global customer usernames.
- New schema is additive under db/migrations; no live migration/deployment/router action in implementation.
- A repeated/uncertain operation must not extend time or record payment twice.
- Existing unmanaged PPP secrets/profiles are never silently adopted; no shared fallback account, RADIUS server or free-access mode.
- Periods are explicit days (24-hour units), not fixed billing dates. Amount is the owner's recorded payment; authentication never creates a payment.
- Router writes retain existing explicit network-write gate plus intentional confirmation; local enablement trials require no blanket hardware-certification gate.

## Review focus
- Lost response after renewal followed by retry: same expiry and one operation.
- First use and expiry occur entirely during outage: recover original times and expired state.
- Router reboot, clock regression or missing owned records: expose uncertainty, never restart paid validity.
- Foreign tenant/router IDs, identical names and stale UI context: reject cross-context results/writes.
- Existing PPP profile hooks and unowned accounts: refuse destructive adoption.

## Tasks

### Task 1: Domain policy and additive ledger
Files: internal/broadband/{policy.go,policy_test.go}; db/migrations/20261104000000_broadband.sql; internal/database/broadband_repo.go and integration tests.
- [x] Test activation, early/late renewal, retries, stale/invalid observations, and period validation before implementation.
- [x] Add tenant accounts, operations and immutable payment snapshots with composite ownership constraints and forced RLS.
- [x] Use operation IDs, account locking and persisted pending operations; do not hold SQL transactions over SSH.

### Task 2: Router-local enforcement
Files: internal/provisioner/routeros/broadband.go and tests.
- [x] Implement owned per-account PPP profiles/secrets, first-use hook, expiry scheduler, renewal marker and readback.
- [x] Keep source profile addressing/rate behavior without overwriting unrelated scripts or accounts.
- [x] Serialize per-device operations, reuse credentials, reject stale state, and preserve uncertain completion for retry.
- [x] Enforce expiry by disabling the secret and removing active sessions; never delete the account.

### Task 3: API, recovery and revenue
Files: internal/handler/broadband_handler.go; internal/server/server.go and broadband worker; database integration/handler tests.
- [x] Tenant-bound create/renew/suspend/resume, list/sync and report endpoints, with role/body/write gates.
- [x] Encrypted pending provisioning material and retry-safe operation completion.
- [x] Background read-only reconciliation, retained source timestamps and distinct PPP payment report.

### Task 4: Existing-app UI
Files: app/src/pages/network/Broadband.tsx and tests; network/index.tsx; Revenue.tsx and separate broadband report component; i18n strings.
- [x] Existing Customer internet workspace gains paid accounts and renewals, retaining low-level editors.
- [x] Keep router target visible and locked during writes; explicit confirmation and reusable operation IDs.
- [x] Add separate Revenue section; leave Hotspot/media/payment totals unchanged.

### Task 5: Hotspot catch-up and verification
- [x] Regression for first-observed already-expired vouchers; complete activation/expiry in the same reconciliation without shifting first use.
- [x] Run relevant real-Postgres tests, race tests, frontend suite/lint/build and existing regression suites.
- [x] Review the complete diff; record real limitations and release/migration instructions. Do not deploy without a separate instruction.

## Progress
- Source: feature branch from deployed a87397f, two pre-existing untracked notes preserved.
- Ruling: implement natively in this chat; the user's instruction to start implementation supersedes another plan-confirmation pause.

- Completed local implementation: additive tenant ledger; encrypted operation recovery and key rotation; router-local PPPoE validity, first-use and suspension; guarded API/UI; separate payment reporting; observed session maxima; Hotspot catch-up expiry.
- Source review corrections: separate immutable first-use anchor, lock-before-snapshot, operation ordering, stale-sample rejection, managed-profile protections, conditional enabling, enforcement readback and terminal create rejection.
- Verified: full `go test -race ./internal/...` with disposable PostgreSQL; changed handler/router packages rerun after review corrections; command packages; 186 frontend tests; frontend build/lint; 22 critical UI regressions; 57 migration checks; 2 deployment and 8 proxy tests.
- Browser fixture review: desktop and 390px mobile paid-account/revenue screens, populated create form, explicit confirmation details, and no overflow within the new sections. Synthetic data only.
- Existing warnings retained: NetworkEditor hook dependency and large map bundle. No live router execution, database migration or deployment performed.
- Physical RouterOS syntax, permissions, timing and outage behavior remain controlled-trial checks. This is not a blanket deployment/enablement gate or hardware certification claim.
- Accounting boundary: per-session observed maxima/counter-reset evidence, not a durable complete offline traffic journal. No usage-volume billing added. Trusted NTP/clock and an existing usable PPPoE service/plan are prerequisites; this batch does not configure arbitrary factory-default routers.
- Release/rollback details: `docs/ROUTER-LOCAL-BROADBAND.md`. Two pre-existing local notes were preserved. No merge, push or production release included in this implementation.
