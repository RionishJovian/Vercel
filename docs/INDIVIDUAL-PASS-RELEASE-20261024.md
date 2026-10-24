# Individual client access release — 24 October 2026

## Confirmed model

The platform DEVELOPER manages client accounts. A client has one authorized owner login, one private internal business workspace, one site and one RouterBOARD. Business IDs remain the isolation boundary; client staff creation/invitation redemption is retired. Existing users, routers, Store history and voucher records are preserved. Ambiguous primary logins require explicit operator selection, with expected-identity concurrency protection and an audit record.

An operator approval starts an individual monthly/annual pass at the server's current time. Terms use calendar month/year in Asia/Yangon and clamp month ends. Both plans share features. Expired users can sign in, read their own pass/profile and request renewal, while management requests are denied. Pending renewal retries reuse the request; a request can be approved/rejected once. Neither a renewal request nor a manual pass approval proves bank settlement. Legacy Store purchases never activate the current pass; new legacy checkout/coin writes and old Store decisions are retired in individual mode.

## Enforcement and rollout

`billing.individual_passes_enabled` defaults false and cannot coexist with legacy `subscription_only`. It gates protected management, guest-voucher owner management and media owner routes. Public guest voucher routes, router-local enforcement and accounting workers are not app-pass gates. The legacy business quota-disconnection sweep is disabled in individual mode. Existing guest internet must still be certified on physical RouterOS; passing HTTP tests does not prove it.

One site/router per client workspace is enforced in PostgreSQL across creation paths, including concurrent requests. Existing over-cap records are retained; subsequent creation is denied. Operator workspaces are exempt. Old capacity tiers/token balances remain historical data rather than current subscription allowances. Individual mode bypasses the old Hub/WebFig, site/router and AI feature gates together with the new access middleware. Background AI checks client access before calling its provider and does not debit old app AI tokens. External provider limits, disabled connectors and unsupported router capabilities remain distinct from app usage allowances.

Apply additive migration `20261024040000_individual_pass_rollout.sql`, then execute `deploy/activate-individual-passes.sql` at the deployment boundary. Its preflight rejects active client workspaces without an enabled authorized owner; it arms one shared deployment clock and seeds seven-day (168-hour) deadlines only for pre-existing non-operator accounts. Reruns/restarts do not extend it or grant historical grace to later signups. Explicit revocation overrides transition access. No customer pass/payment is activated by deployment.

Startup checks require the armed rollout, required columns, forced RLS, capacity/primary triggers and restricted tenant mutation privileges. The public pricing response reports current client allowances and no longer advertises old capacity tiers in individual mode; monthly/annual prices remain sourced from existing published pricing.

## Validation

- `MM_TEST_DB_URL=<disposable fixture> go test -race ./internal/...`: passes.
- Production route-chain integration uses the actual `setupRoutes`, real PostgreSQL and session JWTs: calendar activation, expiry recovery, actual password login after expiry, renewal approval, public guest route independence, client staff denial, public policy, single site/router responses and schema mutation rejection.
- Additional real-DB tests cover cross-account renewal isolation, decision retries, explicit primary selection, concurrent resource creation and deployment-clock idempotence/new-signup exclusion.
- `npm run test:critical-ui`: 82 passed, zero skips/failures. Additional navigation and owner-library checks pass.
- Go server build and production inline JavaScript syntax pass. No live payments, client role writes or router configuration are test evidence.

## Release boundary and rollback

Prepared locally; deployment receipt must be appended only after live verification. Preflight observed production API 066f46a and web af4282c with the recorded binary hash. All eight active client workspaces matched canonical enabled owners; none exceeded one site/router. This is a dated observation, not hardware verification.

Before deployment, preserve/validate a fresh DB dump, current Compose/config, binaries, web files and exact image IDs. Build immutable API/web images from the existing runtime bases; change only the API, frontend and individual-pass config. On failure restore the old Compose/config/images/files and set the rollout's enforcement_enabled=false. Keep additive tables/history; do not restore the full database merely to roll back the application. Re-enabling must preserve the original rollout clock.

RouterBOARD/phone internet, first-use, expiry, data-cap and reconnect certification remains pending. Jellyfin connector/key and playback acceptance comes last, after the router path is ready. This release does not complete the overall goal.
