# Mega Magic — verified status (audited 2026-11-14)

This file replaces the build-20 status of 2026-10-07, which described only the Python prototype and listed
features that were not wired or tests that could not run. Product rules and the current production picture live in
`AGENTS.md`; read that first. Statements here were checked on the date above unless marked **not verified**.

## Which code is which

| Code | Role | Evidence |
|---|---|---|
| `cmd/`, `internal/` (Go, PostgreSQL) | **Production API** | `AGENTS.md` sections 2 and 3 |
| `app/`, `web/` | Production console and portal pages | `AGENTS.md` section 2 |
| `backend/` (Python, FastAPI, SQLite or PostgreSQL via Alembic) | Prototype of the account and payment-review API. Not in the production deploy path. About 2,400 lines, last meaningful change 2026-10-07 | `git log -- backend/` |
| `server.py` at the repository root | **Absent.** A stdlib demo server that once held `/api/hooks/payment-decision` and `/api/demo-session`. `.gitignore` (`/server.*`) hid it, so it was never committed | See "Payment webhook" below |

## Tests (run 2026-11-14)

- Python: `python -m unittest discover -s tests -p 'test_*.py' -v`, on Python 3.12 (the CI runner's version) with the
  pinned `requirements.txt`: 119 run, all pass, 1 skipped. The skip is `tests/test_api.py`, which now **skips with a
  stated reason** because `server.py` is not in the repository (before this change it failed at import). On macOS
  with Python 3.15 the pinned `pydantic_core` wheel does not load (`dlopen` error); use 3.12 locally.
- Node: `node --test tests/account-client.test.mjs` passes 4 of 4.
- Go: `go build ./...` builds. `go test -short` passes for `internal/hub`, `internal/voucher`, `internal/auth`,
  `internal/secretbox`, `internal/security`. Database-backed Go tests need a disposable `MM_TEST_DB_URL` and are
  skipped without it; a skipped test is not coverage. The full Go suite and `-race` were **not run** in this audit.
- CI: the `python-api` job in `.github/workflows/deploy.yml` now runs the Python and account-client tests on every
  push and pull request. It does not yet gate the `build` job. Make it required in the repository settings once it
  has been green for a while.

## Payment webhook

The contract in `README.md` (`POST /api/hooks/payment-decision`, `X-Timestamp` within 300 seconds, `X-Nonce` of at
least 16 characters, lowercase hex `HMAC-SHA256` over `timestamp + "." + nonce + "." + raw_body`, nonce replay
rejected) has **no implementation in this repository**: not in `backend/`, not in Go. The 7 workflow tests in
`tests/test_api.py` that exercised it cannot run, so the earlier "7/7 API workflow tests pass" claim is unverifiable.

An archived copy of the old demo server exists outside the repository (a handoff zip from 2026-10-13). It is a
reference for the webhook design only. It must not be restored as-is: its `/api/demo-session` issues a token for any
role without authentication. Decide whether to port the webhook (to Go, behind an off-by-default gate) or retire the
README section.

## Python prototype: what exists and what is only planned

- Exists and tested: accounts and sessions, orders and manual payment review, receipts, vouchers (hashed at rest,
  single redemption), voucher sales, append-only financial events and revenue reports, portals, sites, appearance,
  media titles, knowledge cards, staff grants, subscriptions, MFA, review-API credentials and event delivery.
- Planning only: Magic Hub plans (`backend/hubs.py`) record the `/32` address and public keys. `dispatch` is
  `not_implemented`; nothing contacts a router. Private WireGuard keys are never accepted.
- Removed on 2026-11-14: `backend/radius.py` and the `mock_routeros_checks` / `onboarding_workflow` functions. No
  route used them, and the mock reported checks as "passed", which could have been mistaken for real verification.
- Migrations 001 to 019. 003 to 019 refuse to downgrade when they hold data. **001 and 002 do not** (their
  downgrade drops accounts, orders, audit events, tenants and portal settings). Not yet fixed.

## Open items from the 2026-11-14 backend audit (not fixed by this change)

1. Go guest voucher codes are stored in plaintext in `vouchers.code`.
2. The one-paste WireGuard flow generates the router private key on the server and keeps it, encrypted, for up to 30
   days. The two-step bootstrap flow keeps the key on the router.
3. `DecideGuest` calls the router inside an open database transaction with no timeout.
4. Rate limits key on the client IP; whether the production proxy chain presents real client addresses was **not
   verified** on the live server.
5. The public Go portal config returns `business_id`, `media_host` and `updated_at` to anonymous callers.
6. Whether the production server has a credential encryption key set was **not verified**.

## Not verified

Anything about the live server, a physical router, Telegram approver checks, the Hub service's own address
allocator, and the Python modules not read in the audit (knowledge, deliveries, review API, staff, subscriptions).
