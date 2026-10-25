# Magic Hub Site Endpoint Fix - Release Manifest

**Release Branch**: `fix/magic-hub-site-endpoint-0908d54`
**Base Commit**: `0908d54` (proven production source — exact binary reproduction confirmed)
**Site-fix commit**: `fea87e1`; follow-up restores the published workflow’s read-only enrollment-capability endpoint. Final source revision is recorded in the deployment log.
**Date**: 2026-10-22

## Problem
The live Magic Hub onboarding flow was blocked because the deployed API was missing the `/api/v1/vps/peers/:deviceId/site` endpoint. The adjacent `/live` route correctly returned 401 (auth required), but `/site` returned plain 404. This prevented site assignment from loading and connection controls remained disabled.

## Root Cause
The site assignment endpoints (`GET/PUT /peers/:deviceId/site`) and the `RequireGatewaySite` guard middleware were implemented on `feature/access-tier-user-role-separation` but were absent from the production baseline (`main` branch at `0908d54`). The deployed API binary SHA256 `3a1604d6808f98ccb2b74b6f0099cab6227162cbb51776cca430a3d7befba17f` corresponds to a build from `0908d54` without these routes.

**Source identity of running API**: ESTABLISHED. Codex independently rebuilt commit `0908d54bfd2ac93190b79628fb727c2424d64026` with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o reproduced ./cmd/server` and obtained EXACT byte-for-byte match with the running API binary (SHA256 `3a1604d6808f98ccb2b74b6f0099cab6227162cbb51776cca430a3d7befba17f`). Production baseline is `0908d54`.

## Solution
Isolated fix containing only the minimal changes needed for the Magic Hub onboarding workflow:
1. Added `internal/handler/hub_site.go` with `GatewaySite` and `RequireGatewaySite` handlers
2. Added `internal/handler/vps_routes.go` with shared route registration helper `RegisterVPSRoutes`
3. Updated `internal/server/server.go` to call `handler.RegisterVPSRoutes` (preserves all other 0908d54 server wiring)
4. Added `internal/handler/hub_site_test.go` with integration tests for site assignment, site guard, and completion endpoints

## Files Changed

| File | Change Type | Purpose |
|------|-------------|---------|
| `internal/handler/hub_site.go` | New file | Site assignment handler (GET/PUT) and site readiness guard |
| `internal/handler/vps_routes.go` | New file | Shared route registration helper `RegisterVPSRoutes` for production and tests |
| `internal/handler/hub_site_test.go` | New file | Integration tests for site assignment, site guard, and completion endpoints |
| `internal/server/server.go` | Modified | Call `handler.RegisterVPSRoutes` instead of inline route registration; preserves all other 0908d54 server wiring (voucher report, accounting worker, etc.) |

## Route Changes

| Method | Path | Handler | Auth | Notes |
|--------|------|---------|------|-------|
| GET | `/api/v1/vps/peers/:deviceId/site` | GatewaySite | Admin | Read site assignment |
| PUT | `/api/v1/vps/peers/:deviceId/site` | GatewaySite | Admin | Assign site to gateway |
| POST | `/api/v1/vps/peers/:deviceId/enroll` | RequireGatewaySite + EnrollGateway | Admin | Guarded enrollment |
| POST | `/api/v1/vps/peers/:deviceId/quick-enroll` | RequireGatewaySite + QuickEnrollGateway | Admin | Guarded quick enrollment |
| POST | `/api/v1/vps/peers/:deviceId/finish` | RequireGatewaySite + FinishGatewayConnection | Admin | Guarded connection completion |
| GET | `/api/v1/vps/peers/:deviceId/bootstrap` | RequireGatewaySite + GatewayBootstrap | Admin | Guarded bootstrap |
| POST | `/api/v1/vps/peers/:deviceId/authenticate` | RequireGatewaySite + AuthenticateGateway | Admin | Guarded management auth |

## Tenant Isolation & Security
- Site assignment validates that the target site belongs to the same business (`business_id` match)
- `RequireGatewaySite` guard enforces:
  - Gateway exists in the tenant's business
  - Gateway has `deployment_layer='MAIN_GATEWAY'`
  - Gateway has a valid site assigned (JOIN with sites table)
  - Returns 409 with `code: "site_required"` if no site assigned
  - Returns 404 for foreign/non-existent gateways
- All endpoints require `RoleAdmin` minimum

## Tests
- `TestVPSHandler_GatewaySite`: 9 subtests — successful readback, assignment, foreign site rejection with DB readback verification, invalid UUID, non-gateway devices, unauthenticated (401), VIEWER role denial (403), VIEWER write denial with DB readback
- `TestVPSHandler_RequireGatewaySite_GuardedRoutes`: 20 subtests — gateway with site (pass), without site (409), non-existent (404), invalid UUID (400), cross-business (404), VIEWER role denial (403), unauthenticated (401) across all 5 guarded endpoints; plus callback non-invocation verification for unauthenticated, VIEWER, missing-site, cross-business, invalid UUID (includes `enrollmentHubCalls`)
- `TestVPSHandler_FinishGatewayConnection`: 3 subtests — site guard runs before handler, 409 without site, 503 without automaticVerify, callback non-invocation
- `TestVPSHandler_AuthenticateGateway`: 5 subtests — site guard runs before handler, 409 without site, 503 without managementAuth, 400 invalid input, VIEWER role denial (403), callback non-invocation
- `TestVPSHandler_GatewayBootstrap`: 3 subtests — site guard runs before handler, 409 without site, 503 without enrollmentHub, callback non-invocation
- `TestVPSHandler_RouteRegistration`: 7 subtests — exercises actual production route registration for all 7 endpoints via shared `RegisterVPSRoutes` helper, asserts not 404
- Tests use integration harness (`MM_TEST_DB_URL`) with real schema (loads `database/init.sql` including `devices.updated_at`), skip gracefully when database unavailable
- **Total: 52 subtests passing, zero skips, zero failures**

## Verification

```
# Build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o /tmp/mega-magic-hub-fix-linux-amd64 ./cmd/server     PASS

# Tests (with MM_TEST_DB_URL set)
go test -v ./internal/handler -run TestVPSHandler                                        PASS (52/52 subtests)
go test ./internal/handler/...                                                           PASS
go test ./internal/server/...                                                            PASS
go test ./internal/...                                                                   PASS (all tests pass on corrected base)
```

### Build Artifacts

**Linux AMD64 (deployment artifact)**:
- Path: `/tmp/mega-magic-hub-fix-linux-amd64`
- Format: ELF 64-bit LSB executable, x86-64, statically linked, Go BuildID=hJs4-8BCCtX3q8E5wqg9...
- Go version: go1.26.5, GOOS=linux, GOARCH=amd64, CGO_ENABLED=0
- **SHA256**: `b09b8e2febc04e1d07dfa90e74f145d1197db4b0a8100281f33ea5af3525112e`

## Production Features Preserved (verified present on 0908d54 base)
- Router-local voucher login (`internal/provisioner/routeros/voucher_login.go`, `voucher_login_sync.go`)
- Voucher provisioning during issuance (`internal/handler/guest_voucher_handler.go`)
- Manual and periodic router-accounting synchronization (`internal/server/guest_voucher_sync_worker.go`, `guest_voucher_routes.go`)
- Voucher reporting (`internal/handler/voucher_report.go`)
- Site portal configuration (`internal/database/site_portal_repo.go`)
- Memberships/invitations and existing migrations
- Existing authentication and subscription enforcement
- Router-local portal behavior with `megamagic.tech` origin (`internal/provisioner/routeros/portal_deploy.go`)
- Production database DSN handling with `quoteConnectionValue` (`internal/database/db.go`)

## Mutation Tests (regression detection verified)

| Mutation | Test That Failed |
|----------|------------------|
| Remove `/enroll` route from `RegisterVPSRoutes` | `TestVPSHandler_RouteRegistration/POST_/api/v1/vps/peers/test-device/enroll` → 404 |
| Remove `RequireGatewaySite` from enroll route | Missing-site/foreign/nonexistent gateway and denied-callback tests fail |
| Remove Admin middleware from GET site | `TestVPSHandler_GatewaySite/VIEWER_role_-_returns_403` fails |

All mutations detected by focused tests in isolated compiler overlays. Restored normal implementation confirms all focused tests pass against disposable PostgreSQL with zero unexpected skips.

## Release Status
**RELEASE PREPARED FOR CODEX DEPLOYMENT REVIEW** — Not deployed. Codex to review, deploy via existing Docker Compose layout, and rerun physical Magic Hub connection process on RouterBOARD.

## Deployment Notes (for Codex)
- Deployment root: `/opt/mega-magic-go-restored-20261018T100554Z`
- Compose file: `deploy/compose.go-restore.yaml`
- Compose project: `mega-magic-go-restored`
- Executable: `/app/mega-magic`
- API config mount: `deployment-root/config.go-production.yaml` → `/app/config.yaml` (read-only)
- App data volume: `mega-magic-go-staging_app_data` → `/app/data`
- Public domain: `megamagic.tech`

## Codex follow-up: published workflow availability

Authenticated post-deployment verification of fea87e1 confirmed site readback succeeds, but the published frontend also requests GET /api/v1/vps/enrollment-capability. That route was missing (404), leaving Connect Magic Hub disabled.

The follow-up restores the existing workflow implementation as a read-only handler and registers it through the shared production routing helper with Admin authorization. It returns deployment flags and plan availability only, never keys or router identities, and does not enroll or modify routers. Tests cover disabled/partial/ready deployment states plus anonymous, VIEWER and Admin HTTP authorization.

The fea87e1 artifact hash above identifies that exact release, not this follow-up. The final clean-export build hash and exact deployment/rollback state are recorded in the Codex deployment evidence. Existing production behavior and infrastructure are preserved.
