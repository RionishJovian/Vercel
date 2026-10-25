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

Deployed and verified; the receipt below supersedes the preparation state. Preflight observed production API 066f46a and web af4282c with the recorded binary hash. All eight active client workspaces matched canonical enabled owners; none exceeded one site/router. This is a dated observation, not hardware verification.

Before deployment, preserve/validate a fresh DB dump, current Compose/config, binaries, web files and exact image IDs. Build immutable API/web images from the existing runtime bases; change only the API, frontend and individual-pass config. On failure restore the old Compose/config/images/files and set the rollout's enforcement_enabled=false. Keep additive tables/history; do not restore the full database merely to roll back the application. Re-enabling must preserve the original rollout clock.

RouterBOARD/phone internet, first-use, expiry, data-cap and reconnect certification remains pending. Jellyfin connector/key and playback acceptance comes last, after the router path is ready. This release does not complete the overall goal.


## Verified production receipt

API source: 0291edca2dec66b8e176441985b92d4643ef54d4 (includes fbb8394). Final frontend source: 0d085ea. VPS compilation independently matched local Linux AMD64 SHA256 `ae4c24a835fac5d05be0dc81c46b1293401fd359f1de7e11d18ae2e6283a175c`.

API image: `mega-magic-api:0291edc`, ID `sha256:f660dd9df4055e4eb068c190514eb15a4af502959a0c481ab38cb96b706d60d8`.
Web image: `mega-magic-web:0d085ea`, ID `sha256:7be3b88e2136ca8eae471a4ef0a04b8eccc0bb7148b85517ae41cc6e50f8c4a6`.
Final public app.html SHA256: `5b4c6515b31b0d5c1c242fc227e97393f12aba23c27ce64b28db44e4524d7851`. Both changed public pages matched source; readiness reports database/NATS/Redis OK. All five application containers are running; web/database health checks pass. No current-container fatal/panic log entries were observed after rollout.

The once-only deployment clock started 2026-10-24 04:05:07.09118 UTC. Twelve pre-existing non-operator rows have the same deadline, 2026-10-31 04:05:07.09118 UTC (**31 October, 10:35 AM Yangon**). Disabled accounts remain disabled, and secondary/legacy logins do not gain management authorization merely from a transition row. Eight active client workspaces matched enabled authorized owners; none exceeded the new site/router capacity. Operator nish@magic is exempt, with no pass required.

Production readback: zero app passes, zero renewal requests. No customer pass/payment was approved and no client role, credential or physical-router setting was changed. Signed-in dashboard displays current 1/1 client capacities, the transition deadline, administrator exemption, renewal inbox and explicit historical quota labels. The administrator's own pass page displays exemption and no renewal requests.

Two initial application switches automatically rolled back because atomic config replacement preserved mode 0600 but changed owner from the API user (UID/GID 100:100) to root. The failed-container log established `open config.yaml: permission denied`. The corrected script preserves UID/GID as well as mode, verifies those metadata values and tests readability as the API image's user before changing running services. No application-code relaxation was made. The final rollout succeeded. The original transition clock was preserved through retries.

Validated rollout backup: `/root/mm-rollout-backups/individual-0291edc-20261024T041829Z` (fresh DB dump/list/SHA, original Compose/config, binaries, assets and image IDs). Final wording-only frontend rollback: `/root/mm-rollout-backups/frontend-0d085ea-20261024T042620Z`. Earlier failed-attempt backups remain intact. Source/build/verification logs: `/root/mm-individual-0291edc/`. The exact corrected operational script is retained as `deploy/releases/individual-0291edc.sh`; it is a version-specific guarded runbook, not a generic redeploy command.

Read-only Hub verification: WireGuard wg0 listens on UDP 51820, with three peers and one historically observed handshake. The live app reads hAP ax2 lab's saved peer (10.77.0.8/32), reports its last handshake as 21 October 2026, 2:45 PM Yangon, and correctly leaves current reachability unverified. No reenrollment, key change, relay or router write was performed. Physical RouterBOARD/phone certification remains pending; Jellyfin connector/key/playback remains last priority. The overall goal is still active.

Local evidence directory: `/Users/aces/Documents/Codex/2026-10-21/asd/work/restructure-20261024/`; screenshots `individual-live-pass.png` and `individual-live-platform.png`. Unrelated WORKFLOW_CONTRACT.md, config.test.yaml and design-alternatives/ remain untouched and uncommitted.
