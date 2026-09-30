# Database migrations

Production schema changes are explicit operator actions. `deploy.sh api` checks
the schema before replacing the binary and stops safely when the required
device SSH objects are absent; it never mutates the database automatically.

## Bank-transfer grace data

Apply `migrations/20260930190000_portal_grace_data_limit.sql` before deploying
the updated API. It adds a 50 MB default to each business's portal config and
records the allowance on each purchase. Existing grace times are preserved.
Both portal limits are editable; access ends at the first exhausted limit.

Temporary access requires the live per-device applier and a configured Hotspot
server with MAC login enabled. The API checks the client in the router's host
table and verifies the time/data caps after creating the bound user. Missing
connectivity or login support returns an error and does not create a pending
purchase. Repeat requests resume the existing allowance without resetting
counters. Guest-plan approval updates the router's paid allowance, and rejection
removes only that purchase's temporary user. MB means 1,000,000 bytes, counting
uploads plus downloads. A router-local one-shot scheduler disables temporary
access at the wall-clock receipt deadline, even if the API is unavailable.
Connected-uptime caps provide an additional limit. Approval removes the grace
scheduler and enables the purchased allowance.

## Store transaction core

Existing installations must also apply
`migrations/20260930210000_store_runtime_permissions.sql`. It grants the tenant
runtime role SELECT/INSERT/UPDATE on purchases and SELECT/INSERT on the audit
trail. It does not grant DELETE or bypass row-level security. The Store schema
check verifies these permissions and enabled tenant policies before deployment.

Back up the production database, then apply the additive Store tables once:

```sh
ENVIRONMENT=production go run ./cmd/store-schema --action apply --config config.yaml
ENVIRONMENT=production go run ./cmd/store-schema --action check --config config.yaml
```

The API deploy refuses to continue unless this check passes. Store approval and
activation then happen in one transaction: an entitlement or coin-credit error
rolls the decision back to pending.

## Device SSH host-key pinning rollout

Use a database backup and maintenance window. Run commands from the repository
root with the production environment explicitly selected:

```sh
ENVIRONMENT=production go run ./cmd/device-ssh-schema --action apply --config config.yaml
```

`apply` is additive and idempotent. It adds the five SSH columns, the lookup
index, and `NOT VALID` checks. Those checks protect new and updated rows while
allowing legacy credential rows to remain temporarily available for operator
review.

Enroll a verified `SHA256:` host-key fingerprint for every device that retains
SSH credentials. If a legacy credential should not be retained, clear all four
credential fields together. Then complete the tracked second stage:

```sh
ENVIRONMENT=production go run ./cmd/device-ssh-schema --action validate --config config.yaml
ENVIRONMENT=production go run ./cmd/device-ssh-schema --action check --config config.yaml
```

`validate` audits every row first and refuses to alter constraint state while
any incomplete credential record remains. The `check` action is deliberately a
deploy-ready check: it verifies the fingerprint-aware definitions and requires
both constraints to be validated. Only after `validate` and `check` pass should
the API be deployed with `./deploy.sh api`. Re-running `apply` afterward is safe
and preserves the validated state.

## Authentication sessions

Before deploying the authentication update, back up the database and run:

```sh
ENVIRONMENT=production go run ./cmd/auth-session-schema --action apply --config config.yaml
ENVIRONMENT=production go run ./cmd/auth-session-schema --action check --config config.yaml
```

The migration creates an owner-only authentication bootstrap table and revocation
triggers. API startup and `deploy.sh api` refuse to proceed without this schema.
Existing tokens are invalidated: all users must sign in again. Expired/revoked
session rows can be removed during regular database maintenance.

Password changes/resets, role changes, user deactivation, tenant reassignment,
and business suspension revoke sessions permanently. Reactivation does not
restore old sessions. Refresh uses a row lock and replaces the stored token
digest; reusing an old refresh token revokes the entire session. Concurrent
refreshes from different browser tabs may require signing in again.

Browser JSON responses omit reusable tokens. Browser tokens move from localStorage to Secure, HttpOnly, SameSite=Strict
cookies. Production requires HTTPS and a proxy that preserves the request Host.
Cookie-authenticated writes require a matching Origin and X-MM-CSRF: 1.
Non-browser login/register/refresh clients without cookies must send
`X-MM-CSRF: 1`; their JSON token responses remain available for bearer requests.
Only loopback proxies are trusted for client IP headers. If the production proxy
is elsewhere, configure its exact trusted address before deployment. Authentication
attempts are limited to 30 per client IP per minute per API process; logout is
exempt. Multiple API replicas also need a shared proxy rate limit.

Regression tests: `go test ./internal/auth ./internal/handler ./internal/server`.
Set `MM_TEST_DB_URL` to a disposable-test-capable PostgreSQL server to exercise
refresh reuse, account changes, logout, suspension, and schema triggers in a
throwaway database. The tests never apply the migration to production.
