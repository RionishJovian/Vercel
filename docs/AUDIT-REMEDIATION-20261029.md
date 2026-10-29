# Audit remediation — 29 October 2026

The subsequent [frontend synchronization](FRONTEND-SYNC-20261029.md) updates the
served frontend identity recorded below. The API hardening and data boundary
remain unchanged.

The five findings from the 28 October VPS/workspace audit are remediated on
the production VPS. Local source is on `codex/normalize-audit-findings-20261028`.
The work preserves the existing application, tenant boundaries, account passes,
voucher recovery/printing guards, router-local portal and router write gates.

## Verified results

1. **SSH:** root accepts the existing authorized application key; password and
   keyboard-interactive SSH authentication are disabled. The older SSH configuration
   did not include its drop-in directory, so an explicit first-line include now
   activates the key-only policy. A fresh connection passed. The local saved SSH
   alias now selects that existing key; no new key was authorized.
2. **Database login:** the API now connects as `mega_magic_runtime`, with
   `NOSUPERUSER`, `NOBYPASSRLS`, `NOCREATEDB`, `NOCREATEROLE`, `NOREPLICATION` and
   `NOINHERIT`. It has explicit service DML grants/policies for the cross-tenant
   authentication, platform and worker operations the existing app requires.
   Tenant transactions still switch to `authenticated`; read-only production
   probes and disposable database tests verify tenant isolation. The database
   owner is retained for administration and recovery, outside serving credentials.
   Production startup now rejects administrative or schema-creation credentials.
3. **Legacy telemetry submission:** authenticated client submissions to the old
   telemetry ingestion handler return `410 telemetry_ingest_retired`, without
   storing a sample, updating the cache or publishing an incident input. The
   authenticated read endpoints and server-side RouterOS collector are preserved.
4. **Browser headers:** the live HTTPS proxy sends HSTS (one year, without
   subdomain expansion), content-type protection, referrer policy, same-origin
   framing protection and a compatible CSP. Both router and hosted guest previews
   rendered with no CSP console errors. Inline scripts/styles remain allowed for
   the existing app and print windows; this is a compatibility policy, not a
   claim of a fully nonce-based CSP. Local container/Vercel configuration contains
   the same policy for future releases; no separate Vercel deployment was made.
5. **Host patches:** all eleven pending packages were upgraded, including host
   Caddy to 2.11.7. A fresh package check reports zero pending upgrades. Affected
   host services were refreshed; the final restart scan reports no pending
   services and the running kernel matches the expected kernel. No VPS reboot
   or data-container restart was required.

## Release identity and preservation

- API image: `mega-magic-api:audit-fixes-20261029`.
- API image ID:
  `sha256:c078966d7dbe63ed6ffe2f9a1e32cc70b961f5354663e3f561e2eba70107f9e8`.
- Running binary SHA-256:
  `c75c3c5b453f08ca50343e99dad7e7d978dc5de48b1ba9fbd6a2758158d51056`.
- Served owner HTML remains the October 28 release:
  `8798bbe2c2f088d202978e3390e5fb9b3da90e99d14b160f571dcc8db1164a0f`.
- Web, PostgreSQL, Redis and NATS container identities were preserved. Only the
  API was recreated. The API readiness endpoint reports database/Redis/NATS OK;
  final API restart count is zero. Persistent Compose, serving credentials and
  the staged recovery binary identify the new release.

The API was built additively against the verified VPS release source, after
checking the original telemetry and voucher-route source hashes. The consolidated
local frontend, including its additional topology change, was not substituted
for the live frontend.

## Verification and operating boundary

- Full local Go suite passed. Database-backed race checks passed for database,
  handlers and server, including runtime-role isolation and voucher compatibility.
- All 107 critical UI tests, eight Vercel proxy tests, two deployment transform
  tests, production frontend build and six-surface static UI audit passed.
- A separately compiled preflight ran the actual production schema checks
  read-only inside the intended container environment before the final switch.
- Fresh signed-in browser reads loaded the overview and voucher stock; both
  router and hosted guest previews rendered without browser console errors.
- Anonymous management access remains refused. No guest voucher was issued,
  retried, synchronized, printed or activated; no payment decision or intentional
  router configuration operation was performed for verification. Physical router
  connectivity/enforcement remains a separate acceptance requirement.

## Voucher policy compatibility and recovery

The first switch exposed an interaction missed by the initial tests: the prior
voucher startup check required exactly one policy per protected table, so the
new service policy prevented startup. Restoring the old image alone did not
restore readiness. Removing the staged service policy from the four protected
voucher/recovery tables restored the previous API.

The final source still requires exactly one tenant policy and all existing
triggers, columns, forced RLS and tenant grants. It permits only one additional
named service policy, with exact role/expression checks and safe role attributes;
the tenant role must not be a member of that service role. Regressions reject
tenant-visible policies, extra policies, changed expressions, inherited authority
and RLS-bypass privileges. The final rollout preflight passed before restarting
the API. Rollback now also restores the old voucher-policy shape before starting
the old release.

Protected recovery material is retained at `/root/mm-audit-fixes-20261028`,
including a validated database dump, previous image/config/environment/binary,
SSH/Caddy backups, package records and the verified release manifest. Private
local evidence is under `.local/audit-fixes-20261028` and remains excluded from
Git and frontend output. Do not publish those directories or replay an old
release override over the persistent current Compose configuration.

`deploy/runtime-db-role.sql` is an explicit operator-run provisioning step,
not an automatic application migration. Future schema changes must provision
service grants/policies for new tables before their corresponding release.
Credentials are generated and stored through protected operator I/O, never in
the SQL file. Rollback to the older superuser-based release is an emergency
recovery action that reopens the retired findings until a corrected release is
restored.
