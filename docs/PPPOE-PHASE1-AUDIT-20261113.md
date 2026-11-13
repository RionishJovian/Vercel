# PPPoE Phase 1 — audit of what exists (2026-11-13)

Read-only audit against `main` at `7f63fbf`. Nothing here was run against a router. `MM_BROADBAND_WRITES_ENABLED` stays off. Labels follow the cycle's evidence vocabulary: **Implemented** = code exists; **Tested locally / CI passed** = automated tests; **Real-device verified** = only with router evidence (none so far).

## 1. What already exists (do not duplicate)

| Layer | Where | State |
|---|---|---|
| Schema | `db/migrations/20261104000000_broadband.sql` | **Deployed** (applied in production; run 672 log). Three tables, all with forced tenant RLS and composite `(business_id, device_id)` foreign keys: `broadband_accounts` (router-local subscriber: `username`, `profile` = RouterOS PPP profile name, `observation`/`session` evidence, `retired`), `broadband_operations` (`create`/`renew`/`suspend`/`resume`, `amount_mmk`, `period_days`, encrypted `payload_ciphertext`, `request_hash`, `pending`→`applied`/`rejected`; one pending per account; immutability trigger), `broadband_session_usage` (per-session byte/uptime maxima). |
| Domain | `internal/broadband/` (`policy.go`, `profile.go`, `transport.go`) | Implemented. First-use anchoring, renew/expiry arithmetic (1–366 days, 24 h days), on-router state encoding, profile-name validation. |
| Repository | `internal/database/broadband_repo.go` | Implemented: list per router, reserve an operation (locks the account first), record observations and sessions, reject a create. |
| RouterOS adapter | `internal/provisioner/routeros/broadband.go`, `broadband_scripts.go` | Implemented. It reuses the SSH transport and host verification, requires NTP to be synchronized, and installs a marked guard script and a 15 s scheduler that enforces expiry on the router. Each subscriber gets its own profile `mm-bb-<id>`. The adapter can settle, verify and read back state. The 2026-11-04 defect (`where name=$name` matching every record) is fixed in source; it is **not real-device verified**. |
| API | `internal/handler/broadband_handler.go`; routes in `internal/server/server.go:641,663-666` | Implemented, Admin role, tenant from the token. `GET /routers/:id/broadband/accounts` (returns `write_enabled`), `POST …/sync`, `POST …/accounts` (create), `POST …/accounts/:accountId/{renew,suspend,resume}`, `POST …/operations/:operationId/retry`, `GET /broadband/report`. Writes return `503 broadband_writes_off` unless `MM_NETWORK_CONTROLS_ENABLED` and `MM_BROADBAND_WRITES_ENABLED` are both `true`. Reads and sync stay available. |
| Background sync | `internal/server/broadband_sync_worker.go` | Implemented; runs only when the applier is enabled. Reconciles evidence; it never records a sale. |
| UI | `app/src/pages/network/Broadband.tsx` (accounts, create/renew/suspend/resume, retry, refresh), `pages/workspaces/Services.tsx` `Isp` (overview; Billing/Automation/RADIUS/FTTx shown as **Planned**), `pages/BroadbandRevenue.tsx` + `Revenue.tsx` (recorded payments, labelled as owner-entered) | **Deployed** with the 2026-11-12 redesign. Connected counts come only from recorded sessions. |
| Tests | 30 Go test functions across `internal/broadband`, `internal/handler/broadband*_test.go` (including a recovery integration test), `internal/provisioner/routeros/broadband*_test.go` (fixture, concurrency); app tests for revenue | CI passed (run 672). Integration tests need `MM_TEST_DB_URL`; without it they skip and are not coverage. |
| Lab evidence | `docs/CHR-LAB.md`, `docs/BROADBAND-PHYSICAL-TRIAL-20261104.md` | Software path passed on a disposable CHR with real PPPoE sessions (2026-11-05, RouterOS 7.24.6). **Every physical-router row is still pending.** |

## 2. Gaps against the Phase 4 brief

| Requirement (4A/4B/4C) | Today | Gap |
|---|---|---|
| Subscriber profile and contact | Only `username` on a router | No subscriber entity independent of the router (name, phone, address, notes). Needs a `pppoe_subscribers` table linked 1:N to `broadband_accounts`. Treat the contact fields as personal data: minimal fields and a retention rule. |
| Plans and versioned plan changes | A plan is an existing RouterOS PPP profile name chosen from inventory | No tenant plan record (rates with units, period, price, currency, archived) and no versioning. Needs `pppoe_plans` + `pppoe_plan_versions`. The RouterOS profile stays the enforcement target; changing a plan produces a draft and never touches RouterOS directly. |
| Site and concentrator assignment | Implicit: the account's `device_id` | Explicit site lookup through `devices`. Concentrator = registered MikroTik router. Mostly a read-model/API change. |
| Intended service state | Derived from the last applied operation | Needs an explicit column or view (`pending` / `authorized` / `suspended`), updated only by an applied operation. |
| RouterOS provisioning state | Operation `pending`/`applied`/`rejected` | Maps cleanly. Add a `recovery_required` outcome for verify failures after apply (today the operation stays `pending` with "Retry saved operation"). |
| Observed session state + freshness | `observation`, `session`, `*_observed_at`, usage table | Exists. Needs the UI states connected / failed / stale / unverified computed from timestamps with an explicit freshness threshold. |
| Secure credential handling | The new-account password is encrypted in the operation payload, excluded from the hash and from responses, and included in key rotation | Exists. Keep it; add a test that no API or log line contains the secret. |
| Payments recorded manually | `amount_mmk` lives **inside** create/renew operations | Payment is coupled to provisioning. The brief needs payments recordable separately from router writes (e.g. cash received for an existing renewal) and clearly labelled "recorded by owner". Proposal: a `pppoe_payments` table referencing the subscriber and optionally an operation. Revenue keeps counting existing applied operations so history does not change. |
| Renewal records | Operations of kind `renew` | Exists. |
| Audit history | Immutable operations | Partial: there is no actor (`user_id`) on operations and no audit for non-router edits. Add an actor and a `pppoe_audit_events` table for subscriber/plan edits. |
| Pagination, search, filters | `GET accounts` returns every account on one router | Needs a tenant-wide paginated list with site/plan/state filters and a search. |
| Draft → review → approval → backup → apply → verify | The confirm checkbox goes straight to apply | No draft object, no human-readable diff, no backup step. 4B adds a `pppoe_change_requests` table (draft/approved/applied/rolled_back/recovery_required) reusing the existing Safe Change pattern used by network controls. Apply stays behind `MM_BROADBAND_WRITES_ENABLED`. |
| Owner-only approval | Admin role | The account model has one login per client (AGENTS §4), so approval means an explicit second confirmation by the same owner, with the diff shown. No new roles. |

## 3. Proposed PR sequence (each a draft PR; none merges without owner approval)

1. **4A-1 Schema (additive only).** Add `pppoe_subscribers`, `pppoe_plans`, `pppoe_plan_versions`, `pppoe_payments` and `pppoe_audit_events`, plus nullable `subscriber_id` and `actor_id` columns on the existing tables. Forced RLS mirrors the broadband policies. No backfill that guesses: existing accounts stay unlinked until the owner links them. Migration tests run against a disposable database (`MM_TEST_DB_URL`). Rollback = a down script that drops only the new objects.
2. **4A-2 Read APIs.** Tenant-wide subscriber list (keyset pagination, search, filters), subscriber detail joining accounts, sessions and operations, plan list. Tests cover authorization, cross-tenant 404 and validation.
3. **4A-3 Non-router writes.** Create and edit subscribers and plans (versioned), record a payment manually (idempotency key, `source = 'owner_recorded'`) and write audit events. None of these touch a router, so they are **not** behind the broadband gate. They are still additive, and the UI labels them.
4. **4B Change requests.** Draft, diff, approve, then apply through the existing adapter, then verify, ending in success, rollback or recovery required. Before apply, export the router's PPP section (`/ppp export`) to a server-side encrypted snapshot. The apply path stays behind `MM_BROADBAND_WRITES_ENABLED`, which defaults off. Retries must stay idempotent through the existing operation identity.
5. **4C UI.** Connect the ISP workspace's Overview, Subscribers, Detail, Plans, Sessions, Renewal and Payment screens to the endpoints above, showing the pending / connected / failed / stale / unverified states and evidence timestamps. Billing automation, RADIUS, Network automation and FTTx stay Planned.
6. **4D Real-router acceptance.** Follow `docs/BROADBAND-PHYSICAL-TRIAL-20261104.md` plus the 15 steps in the brief, on a dedicated router, with owner approval of the window. Prepare a separate proposal to enable the gate only after this passes.

## 4. Decisions needed from the owner before 4A-1 is finalized

- **Contact data:** which subscriber fields to keep (name and phone are proposed; address optional) and how long to retain them after a subscriber leaves.
- **Payments for existing accounts:** confirm that the payments already recorded inside create/renew operations stay as they are (proposed), and that new standalone payments are added beside them rather than migrated.
- **Currency:** MMK only for now (proposed), with a currency column for later.
- **Approval:** approval is the same owner confirming a shown diff (proposed, matching the one-login model), rather than a second person.
