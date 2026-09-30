# SECURITY-CONTRACT.md — Mega Magic non-negotiable guarantees

> **Purpose of this file:** the security contract is the source of truth for what
> the Mega Magic platform must ALWAYS enforce. It is ported from the guarantees
> in the shutdown MikroTik Magic app (React/TanStack/Supabase, ~1,330 test cases)
> and re-expressed for this Go + PostgreSQL + nginx stack.
>
> **Why it exists:** an agent's working memory is finite and can be compressed or
> lost mid-session. This document does not live in memory — it lives in the repo.
> ANY future session must reload this before touching security-sensitive code or
> tests. The guarantees here are negotiable by the user only; never by convenience.
>
> **Coverage key:** ✅ = encoded as a live Go test today · ⛔ = gap, needs a test
> (or is enforced but unproven) · — = intentionally out of scope on this stack.

---

## 1. Tenant isolation (multi-tenant) — EVERY read/write must stay in its tenant

Every `SELECT`/`UPDATE`/`DELETE` through the serving path must be bounded to the
caller's `business_id`. There is no cross-tenant read or write except the granted
DEVELOPER platform surface.

- ✅ `TestDB_RLS_tenantIsolation` — RLS on core tables isolates tenants
- ✅ `TestDB_Team_isolation`, `TestDB_PortalDevice_isolation`,
  `TestDB_DeviceBackup_isolation`, `TestDB_Topology_mapCrossBusinessIsolation`,
  `TestDB_Billing_*`, `TestDB_Alerting_isolation`, `TestDB_Telemetry_isolation`,
  `TestDB_RestrictedConn_isolation`, `TestDB_Webhook_isolation` — per-feature isolation
- ⛔ no single test asserts "a tenant-B token can never read tenant-A's vouchers
  by direct ID" end-to-end over HTTP. Add one.
- ⛔ verify nil business_id (unscoped) is FAIL-CLOSED on serving paths, not a no-op.
  (`TestDB_WithTenant_unscopedIsOff_boundary` covers the helper; confirm each repo
  path uses `WithTenant`, not raw `db`.)

Non-negotiables:
1. Repo methods must take a tenant scope; raw queries must join/window by `business_id`.
2. An unscoped request must 403/404, never return another tenant's rows.
3. New tables get a tenant column + RLS in the SAME migration as creation (see the
   deploy pitfall: generic ALL-TABLES misses tables created later → 500).

## 2. RBAC — role gates are exact, min-tier is explicit, no privilege creep

- ✅ `TestRequireRole_customerOwnerBlocked`, `viewerBlocked`,
  `noRoleBlocked`, `developerAdmitted` — exact-role gate works
- ✅ `TestRequireMinRole`, `RequireMinRole_missingRole` — min-tier gate works
- ✅ `TestIsTenantRole`, `TestHasAtLeastRole`
- ⛔ no HTTP-level test proves a customer-Owner token is rejected on the DEVELOPER
  `/platform` group. `RequireRole(DEVELOPER)` is the gate; prove it over the wire.

Non-negotiables:
1. `RequireRole` = exact match; `RequireMinRole` = tier threshold. Never use min
   where exact is meant (platform is exact DEVELOPER).
2. The `/platform` group must NEVER be tenant-scoped and NEVER min-role.
3. Self-suspend is forbidden: an admin cannot suspend their own business
   (⚠ incident 2026-09-27 — this locked the developer out; fixed, guard + UI).

## 3. Fail-closed behavior — no default-allow

- ✅ `TestDB_WithTenant_failClosed`, `failsClosed*`, `unknownActionFailsClosed`,
  `unknownRequestDenied`
- ✅ voucher/approve "grants once" (idempotency): `TestGuestWifi_approveGrantsExtensionOnce`,
  `TestDB_Billing_approveGrantsOnce`, `rejectGrantsNothing`
- ✅ backup restore only when unambiguous: `verifyFailRollsBack`, `review-none` guard

Non-negotiables:
1. On any error/unknown, deny. Never fallthrough to allow.
2. Idempotency: approving a request twice grants the reward once.
3. Never POST an action for a non-actionable finding ("manual review" must not send a fix).

## 4. Auth session hygiene

- ✅ `TestDB_UserMe_tenantScoped`, `TestDB_PortalDeviceAnchored_isolation`
- Refresh rotation/reuse detection and account-change revocation now have
  `TestSessionLifecycle` (requires `MM_TEST_DB_URL`; skipped without PostgreSQL).
- JWT token-type, algorithm, expiry, issuer/audience rejection and cookie-origin
  checks have unit regression coverage. Browser tokens use HttpOnly cookies.
- ⛔ no test that an inactive user (business suspended) is refused (login path does
  check `users.is_active`; prove it)

## 5. HTML/CSS injection guards (server-rendered surface)

Ported guarantee: escaped in print windows + portal HTML; only `#RRGGBB` colors;
quoted CSS `url()`; no operator HTML in generated hotspot pages.
- ⛔ gap — this Go frontend serves static HTML. Re-verify portal/guest pages escape
  user/company-provided strings (template escaping is default in Go `html/template`).

## 6. Browser security headers

Ported guarantee: CSP, nosniff, frame-ancestors, referrer-policy, no-store on
private pages; `X-Content-Type-Options`; not forcing HTTPS on localhost only.
- ⛔ verify nginx sends these; add a `TestHTTP_headers` against the live origin.

## 7. AI/crawler policy

Ported guarantee: `robots.txt` blocks `/app`, `/auth`, `/dev` and generative-AI bots;
private chrome `noindex`.
- ⛔ verify robots.txt on the live origin.

## 8. Secrets & keys

- ⛔ no scan for accidental keys/secrets in committed code (run a scan).
- Digital secrets must never appear in logs, test reports, or output.

---

## Deployment / process hard rules (from this repo's history — do not regress)

1. Unit + integration suite must pass before deploy (`go test ./...` → 20 packages green).
2. E2E must pass against the live origin (`scripts/e2e-public.sh`).
3. Every deploy happens through `./deploy.sh web|api|all` — never raw `cp` / `nohup`.
4. Binary swap keeps `server.prev` for instant rollback.
5. Verify compiled artifact (strings on the running binary) — never trust build logs alone.
6. Release hygiene: prune all but current + previous release; never accumulate.
7. On discovering a test ONLY because a security property is missing, fix and encode the
   property; do not weaken the test to get green.

---

## Open gaps in priority order (encode as Go tests)

1. HTTP cross-tenant read attempt (tenant-B token → tenant-A voucher) → must 404/403.
2. HTTP customer-role token → DEVELOPER `/platform` → must 403.
3. Suspended/inactive user login → must 401/403.
4. Security response headers on live origin.
5. robots.txt blocks app/auth + AI bots.
6. refresh-token rotation test.

*Maintainer note: update this file whenever a guarantee changes. The contract outranks
any single commit. When context is lost, RE-READ THIS FILE before editing security code.*