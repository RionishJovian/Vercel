---
name: megamagic-backend
description: ALWAYS invoke when writing or modifying Mega Magic backend code (Go in cmd/ and internal/), database migrations, MikroTik RouterOS scripts, Magic Hub WireGuard enrollment, vouchers, payments, Telegram alerts or approvals.
---

# Mega Magic backend

`AGENTS.md` is the source of truth for layout, commands, deploys and owner decisions. Read the section you need there; this file only lists the rules that are easy to get wrong.

- Production backend is Go (`cmd/`, `internal/`, module `mega-magic`). `backend/` (Python), `src/` and `worker/` are historical prototypes.
- Migrations: add `db/migrations/<14-digit timestamp>_<name>.sql` only. Never edit an applied file. No BEGIN/COMMIT. New tables need row security, policies and grants. Add the new file to the test-schema file lists in `internal/handler/guest_wifi_handler_test.go` (and `internal/server/individual_pass_http_test.go` if used), or CI test databases will miss the column.
- Tenant isolation is enforced in code, not by the database alone. Business-scoped queries run in `runInTenant` / `database.WithTenant` (role `authenticated`, policy on `app.current_business_id`). The app's runtime role also has a `USING (true)` service policy on every table (checked on production, 2026-11-17), so a query outside a tenant transaction sees every tenant: put `business_id` in its WHERE, and keep those queries to platform-only or deliberate owner reads (for example `GetByIDOwnerTXN`, the retry loops).
- RouterOS: one uniform CLI; rely on documented board strings and runtime capability checks. Router scripts keep WireGuard private keys on the router. Writes are fail-closed behind the gates in `AGENTS.md` section 5; a PR that adds a gate defaults it off.
- Notifications: never drop a send error (about 30 `_ =` swallowed errors remain in `internal/handler`; fix them when you touch the code). Log it, mark delivery only after success, and retry undelivered alerts (see `internal/handler/telegram_renewals.go`).
- Vouchers: internet guest codes are 8 characters from `ABCDEFGHJKMNPQRSTVWXYZ` (`GuestCodeAlphabet`); other codes use the 30-character `CodeAlphabet` with digits 2-9 (`internal/voucher/voucher.go`). Show a code once, store hashes.
- Verify with: `go test -race ./internal/...`, `go build ./...`, and for UI changes `npm run test:critical-ui` and the `app/` tests. Database tests need `MM_TEST_DB_URL` (a disposable database, never production); skipped tests are not coverage.
- Every push to `main` deploys. Check write gates and migrations before merging.
