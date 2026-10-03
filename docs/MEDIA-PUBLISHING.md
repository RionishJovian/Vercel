# Magic Media guest-page publishing

2026-10-03 continuation: [paid-access work](MEDIA-PAID-ACCESS.md) records the
tested purchase/expiry foundations and the owner-PC architecture. It does not
replace the publishing-only production boundary below.

This slice replaces browser-only saving with actual hosted publication.
An administrator edits Guest appearance, chooses **Publish guest page**, and
receives a `/media.html?id=...` link only after the database write commits.
Design and planned pass settings survive browser/device changes. Existing local
drafts may seed the first unpublished design; published server settings take
precedence. Reset affects only the editor until explicitly published.

## Boundaries

This is not a router deployment or a Jellyfin entitlement. Guests cannot buy or
play media yet. Public pages label pricing as planned and exclude sample movies.
The combined target remains Jellyfin identity/playback plus Magic Media purchase
approval, timed access and expiry/session revocation. Those are not implemented
by a design-publication success. Publishing does not change routers, Jellyfin
accounts/libraries, firewalls, purchases or entitlements. Do not enable sales
solely because a computer is paired. Timed grants must expire locally even if the
cloud is unavailable before payments can be enabled.

## API and storage

- Protected GET/PUT/DELETE `/api/v1/media/publication`: active-tenant, admin-role,
  subscription and cookie-origin guards. PUT/DELETE require `?revision=N`;
  zero creates, positive revisions compare-and-swap. Ambiguous writes require
  reload before retry; conflicts never silently overwrite another operator.
- Random 256-bit sharing IDs, no business-ID enumeration. Public GET
  `/api/v1/media-guest/:id` exposes design and reports purchases/playback false.
  Public fetch omits staff cookies. Suspended businesses and withdrawn links
  return 404. Republished pages receive new sharing IDs.
- Dedicated table with forced RLS, restricted transaction-local `authenticated`
  role and tenant context. Unique business/sharing keys provide indexed lookups.
  Public ID resolution alone uses the owner pool, then reads under tenant RLS
  and checks that the ID remains current.
- Bounded typed fields, unknown fields rejected, integer price/duration, validated
  palettes/colours, no arbitrary URLs/credentials. PNG/JPEG headers and dimensions
  checked (maximum 1200 pixels); browser uploads are resized/re-encoded, including
  WebP input. Maximum request is 3 MiB.
- Escaped markup in a scriptless sandboxed iframe; raster data images only.
  Uploaded artwork becomes public with the design: do not upload private images.
- Missing migration returns 503 for publication, leaving existing connector
  routes operational. No false successful publishing response.

## Rollout and verification

Production migration/deployment explicitly approved on 2026-10-03. Protected
pre-release database/schema and binary/config/web backups:
`/home/ubuntu/deploy-backups/media-publishing-20261003.ZNggnO/`.

Apply once with ON_ERROR_STOP after schema-only production rehearsal.
The migration is additive and transactional, not rerunnable. Verify forced RLS,
tenant policy and authenticated grants before deploying API/web. Never use
fixtures on production. Retain the additive table on app rollback; restore
binary/web backups rather than overwriting live database contents.

Go race tests and browser/static checks passed. Browser acceptance covers publish,
server reload, revision conflicts, public CSP, cookie isolation and withdrawn
links with synthetic data. Isolated PostgreSQL tests passed three consecutive
race-tested runs: persisted updates, tenant isolation, stale writes, suspension
and withdrawn/reissued links. Production status is recorded in the current handoff
after rollout. The migration and API/web deployment completed at `d51e212`.
Landing/app/health returned 200, and the signed-in live Guest design loaded
the new enabled Publish control and unpublished status. This was a read-only
live check; no customer design, Jellyfin account or paid access was changed.

Commands: `go test -race ./internal/media/bridge ./internal/handler ./internal/server`,
`node scripts/test-media-appearance.mjs`, `node scripts/test-media-dashboard.mjs`,
`node scripts/test-media-publishing-ui.mjs`. PostgreSQL tests require explicit
`MM_MEDIA_TEST_DSN` for disposable loopback `mm_media_test`; otherwise they skip.
